package routes

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/connections"
	"github.com/cwchen-twn/rigel-ledger/internal/sealing"
)

// The whole P4c-1 loop, the way a runner and a person drive it: a runner
// token, keys and connectors; a connection with sealed credentials the app
// cannot read; a claim, a challenge answered in the app, a batch into the
// book's queue; a failure, a key rotation, and a delete.
func TestConnectionsAndTheRunnerProtocol(t *testing.T) {
	f := newAPI(t)
	alice, bob := f.browser("alice"), f.browser("bob")
	var me UserDTO
	alice.json("GET", "/api/me", nil, 200, &me)

	// Alice links her runner (Settings -> Sync runner).
	var made TokenCreatedDTO
	alice.json("POST", "/api/me/runner/token", map[string]any{"label": "fake runner"}, 201, &made)
	runner := &client{f: f, bearer: made.Token}

	// Its scope is /api/runner/* and nothing else; nobody else reaches that.
	for _, p := range []string{"/api/me", "/api/books", "/api/connectors"} {
		if res, b := runner.do("GET", p, nil); res.StatusCode != 403 || errorCode(t, b) != "token_scope" {
			t.Errorf("runner GET %s = %d %s", p, res.StatusCode, b)
		}
	}
	if res, _ := alice.do("POST", "/api/runner/jobs/claim", map[string]any{}); res.StatusCode != 404 {
		t.Fatalf("a person claimed jobs: %d", res.StatusCode)
	}

	// No runner key yet: nothing can be sealed.
	var cat CatalogDTO
	alice.json("GET", "/api/connectors", nil, 200, &cat)
	if cat.Key != nil || len(cat.Connectors) != 0 {
		t.Fatalf("catalog before the runner = %+v", cat)
	}
	priv, pub, _ := sealing.NewKey()
	runner.json("POST", "/api/runner/keys", map[string]any{"public_keys": [][]byte{pub}}, 200, nil)
	runner.json("PUT", "/api/runner/connectors", map[string]any{"connectors": []map[string]any{{
		"id": "fake", "name": "Fake Bank", "country": "zz",
		"fields": []map[string]any{{"name": "username", "label": "Username", "kind": "text"}, {"name": "password", "label": "Password", "kind": "secret"}},
	}}}, 204, nil)
	alice.json("GET", "/api/connectors", nil, 200, &cat)
	if cat.Key == nil || !bytes.Equal(cat.Key.PublicKey, pub) || len(cat.Connectors) != 1 || cat.Connectors[0].Country != "ZZ" {
		t.Fatalf("catalog = %+v", cat)
	}

	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "Family", "base_currency": "TWD"}, 201, &book)
	creds := []byte(`{"username":"alice","password":"otp"}`)
	blob, _ := sealing.Seal(pub, creds, connections.CredentialsAAD(me.ID, "fake"))
	in := map[string]any{"book_id": book.ID, "connector": "fake", "label": "My fake bank", "key_id": cat.Key.ID, "sealed": blob}

	// Only an editor of the book may point a connection at it.
	if res, b := bob.do("POST", "/api/me/connections", in); res.StatusCode != 422 {
		t.Fatalf("non-member connection = %d %s", res.StatusCode, b)
	}
	alice.json("POST", fmt.Sprintf("/api/books/%d/members", book.ID), map[string]string{"username": "bob", "role": "viewer"}, 204, nil)
	if res, b := bob.do("POST", "/api/me/connections", in); res.StatusCode != 422 {
		t.Fatalf("viewer connection = %d %s", res.StatusCode, b)
	}
	if res, _ := alice.do("POST", "/api/me/connections", map[string]any{"book_id": book.ID, "connector": "fake", "key_id": cat.Key.ID, "sealed": []byte("plain text")}); res.StatusCode != 422 {
		t.Fatalf("an unsealed blob was accepted: %d", res.StatusCode)
	}
	var created CreatedDTO
	alice.json("POST", "/api/me/connections", in, 201, &created)

	// The blob never comes back to a person, and the audit copy leaves it out.
	res, body := alice.do("GET", "/api/me/connections", nil)
	if res.StatusCode != 200 || bytes.Contains(body, []byte(base64.StdEncoding.EncodeToString(blob)[:40])) || bytes.Contains(body, []byte("sealed")) {
		t.Fatalf("connections list leaks the blob: %s", body)
	}
	var audited int
	if err := f.store.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE table_name = 'connections' AND (new_values ? 'sealed' OR old_values ? 'sealed')`).Scan(&audited); err != nil || audited != 0 {
		t.Fatalf("audit rows with the blob: %d %v", audited, err)
	}

	// The runner claims it once, and opens exactly what the browser sealed.
	var jobs []JobDTO
	runner.json("POST", "/api/runner/jobs/claim", map[string]any{"limit": 5}, 200, &jobs)
	if len(jobs) != 1 || jobs[0].ID != created.ID || !bytes.Equal(jobs[0].Sealed, blob) {
		t.Fatalf("jobs = %+v", jobs)
	}
	got, err := sealing.Open(priv, jobs[0].Sealed, connections.CredentialsAAD(jobs[0].UserID, jobs[0].Connector))
	if err != nil || !bytes.Equal(got, creds) {
		t.Fatalf("runner opened %q %v", got, err)
	}
	runner.json("POST", "/api/runner/jobs/claim", map[string]any{}, 200, &jobs)
	if len(jobs) != 0 {
		t.Fatalf("claimed twice: %+v", jobs)
	}
	cpath := fmt.Sprintf("/api/runner/connections/%d", created.ID)

	// A challenge nobody answers expires; one answered in the app reaches the
	// runner once, sealed.
	var ch ChallengeCreatedDTO
	runner.json("POST", cpath+"/challenges", map[string]any{"kind": "otp", "prompt": "code?", "ttl_seconds": 1}, 201, &ch)
	time.Sleep(1100 * time.Millisecond)
	var ans ChallengeAnswerDTO
	runner.json("GET", fmt.Sprintf("%s/challenges/%d", cpath, ch.ID), nil, 200, &ans)
	if !ans.Expired || ans.Answered {
		t.Fatalf("expired challenge = %+v", ans)
	}
	runner.json("POST", cpath+"/challenges", map[string]any{"kind": "otp", "prompt": "Enter the SMS code", "ttl_seconds": 300}, 201, &ch)
	var list []ConnectionDTO
	alice.json("GET", "/api/me/connections", nil, 200, &list)
	if len(list) != 1 || list[0].Status != "needs_user_action" || list[0].Challenge == nil || list[0].Challenge.ID != ch.ID {
		t.Fatalf("waiting connection = %+v", list)
	}
	answer, _ := sealing.Seal(pub, []byte("123456"), connections.AnswerAAD(ch.ID))
	apath := fmt.Sprintf("/api/me/connections/%d/challenges/%d", created.ID, ch.ID)
	if res, _ := bob.do("POST", apath, map[string]any{"sealed": answer}); res.StatusCode != 409 {
		t.Fatalf("bob answered alice's challenge: %d", res.StatusCode)
	}
	alice.json("POST", apath, map[string]any{"sealed": answer}, 204, nil)
	if res, _ := alice.do("POST", apath, map[string]any{"sealed": answer}); res.StatusCode != 409 {
		t.Fatalf("answered twice: %d", res.StatusCode)
	}
	runner.json("GET", fmt.Sprintf("%s/challenges/%d", cpath, ch.ID), nil, 200, &ans)
	code, err := sealing.Open(priv, ans.Sealed, connections.AnswerAAD(ch.ID))
	if !ans.Answered || err != nil || string(code) != "123456" {
		t.Fatalf("answer = %+v %q %v", ans, code, err)
	}
	ans = ChallengeAnswerDTO{}
	runner.json("GET", fmt.Sprintf("%s/challenges/%d", cpath, ch.ID), nil, 200, &ans)
	if ans.Sealed != nil || !ans.Answered {
		t.Fatal("an answer was handed out twice")
	}

	// Rows land in the connection's book, under its connector.
	var imp ImportResultDTO
	runner.json("POST", cpath+"/imports", map[string]any{"connector": "something-else", "label": "sync",
		"accounts": []map[string]any{{"id": "chk-1", "label": "Checking", "currency": "TWD"}},
		"rows":     []map[string]any{{"kind": "transaction", "account": "chk-1", "id": "r1", "date": "2026-09-24", "amount": "-120", "description": "7-ELEVEN"}},
	}, 201, &imp)
	var queue []ImportRowDTO
	alice.json("GET", fmt.Sprintf("/api/books/%d/imports/queue", book.ID), nil, 200, &queue)
	if imp.Staged != 1 || len(queue) != 1 || queue[0].ExternalID != "fake:r1" {
		t.Fatalf("staged %+v, queue %+v", imp, queue)
	}
	runner.json("POST", cpath+"/finish", map[string]any{"status": "ok"}, 204, nil)
	if res, _ := runner.do("POST", cpath+"/imports", map[string]any{"connector": "fake", "rows": []any{}}); res.StatusCode != 409 {
		t.Fatalf("import after finish = %d", res.StatusCode)
	}

	// Not due again until asked; a failure is a code, shown to the owner.
	runner.json("POST", "/api/runner/jobs/claim", map[string]any{}, 200, &jobs)
	if len(jobs) != 0 {
		t.Fatalf("claimed a job that just ran: %+v", jobs)
	}
	alice.json("POST", fmt.Sprintf("/api/me/connections/%d/sync", created.ID), nil, 204, nil)
	runner.json("POST", "/api/runner/jobs/claim", map[string]any{}, 200, &jobs)
	if len(jobs) != 1 {
		t.Fatalf("sync now was not claimed: %+v", jobs)
	}
	if res, _ := runner.do("POST", cpath+"/finish", map[string]any{"status": "failed", "error": "Your password is wrong!"}); res.StatusCode != 422 {
		t.Fatalf("free-text failure accepted: %d", res.StatusCode)
	}
	runner.json("POST", cpath+"/finish", map[string]any{"status": "failed", "error": "bad_credentials"}, 204, nil)
	alice.json("GET", "/api/me/connections", nil, 200, &list)
	if list[0].Status != "failed" || list[0].LastError != "bad_credentials" || list[0].LastRunAt == nil || list[0].Challenge != nil {
		t.Fatalf("after failure = %+v", list[0])
	}

	// The runner rotates its key: the old blob is stale, and the owner must
	// enter the credentials again, sealed to the new key.
	_, pub2, _ := sealing.NewKey()
	runner.json("POST", "/api/runner/keys", map[string]any{"public_keys": [][]byte{pub2}}, 200, nil)
	alice.json("GET", "/api/me/connections", nil, 200, &list)
	if !list[0].NeedsReentry {
		t.Fatal("connection sealed to a retired key is not flagged")
	}
	alice.json("POST", fmt.Sprintf("/api/me/connections/%d/sync", created.ID), nil, 204, nil)
	runner.json("POST", "/api/runner/jobs/claim", map[string]any{}, 200, &jobs)
	if len(jobs) != 0 {
		t.Fatalf("claimed a connection sealed to a retired key: %+v", jobs)
	}
	cpathMe := fmt.Sprintf("/api/me/connections/%d", created.ID)
	if res, b := alice.do("PUT", cpathMe+"/credentials", map[string]any{"key_id": cat.Key.ID, "sealed": blob}); res.StatusCode != 422 {
		t.Fatalf("sealed to the old key = %d %s", res.StatusCode, b)
	}
	alice.json("GET", "/api/connectors", nil, 200, &cat)
	blob2, _ := sealing.Seal(pub2, creds, connections.CredentialsAAD(me.ID, "fake"))
	alice.json("PUT", cpathMe+"/credentials", map[string]any{"key_id": cat.Key.ID, "sealed": blob2}, 204, nil)
	alice.json("PATCH", cpathMe, map[string]any{"enabled": false, "label": "Paused"}, 204, nil)
	if res, _ := alice.do("POST", cpathMe+"/sync", nil); res.StatusCode != 422 {
		t.Fatalf("sync of a disabled connection = %d", res.StatusCode)
	}
	if res, _ := bob.do("DELETE", cpathMe, nil); res.StatusCode != 404 {
		t.Fatalf("bob deleted alice's connection: %d", res.StatusCode)
	}
	alice.json("DELETE", cpathMe, nil, 204, nil)
	alice.json("GET", "/api/me/connections", nil, 200, &list)
	if len(list) != 0 {
		t.Fatalf("after delete = %+v", list)
	}

	// Unlinked, the runner token is gone.
	var st RunnerStatusDTO
	alice.json("GET", "/api/me/runner", nil, 200, &st)
	if len(st.Tokens) != 1 || len(st.Keys) != 2 || st.Keys[1].RetiredAt == nil {
		t.Fatalf("runner status = %+v", st)
	}
	if res, _ := bob.do("DELETE", fmt.Sprintf("/api/me/runner/tokens/%d", st.Tokens[0].ID), nil); res.StatusCode != 404 {
		t.Fatalf("bob unlinked alice's runner: %d", res.StatusCode)
	}
	alice.json("DELETE", fmt.Sprintf("/api/me/runner/tokens/%d", st.Tokens[0].ID), nil, 204, nil)
	if res, _ := runner.do("POST", "/api/runner/jobs/claim", map[string]any{}); res.StatusCode != 401 {
		t.Fatalf("revoked runner token = %d", res.StatusCode)
	}
}

// Every person's runner is their own: each sees, claims and works on only its
// owner's connections, cannot change what another offers, and linking a new
// runner unlinks the last.
func TestRunnersStayWithTheirOwner(t *testing.T) {
	f := newAPI(t)
	alice, bob := f.browser("alice"), f.browser("bob")
	var am, bm UserDTO
	alice.json("GET", "/api/me", nil, 200, &am)
	bob.json("GET", "/api/me", nil, 200, &bm)
	fields := []map[string]any{{"name": "username", "label": "Username", "kind": "text"}, {"name": "password", "label": "Password", "kind": "secret"}}

	// Admin or not, each links their own; the admin pages offer none.
	if res, _ := alice.do("POST", "/api/admin/runner/tokens", map[string]any{"label": "server"}); res.StatusCode != 404 && res.StatusCode != 405 {
		t.Fatalf("the admin runner token still exists: %d", res.StatusCode)
	}
	var at, bt TokenCreatedDTO
	alice.json("POST", "/api/me/runner/token", map[string]any{"label": "next to the app"}, 201, &at)
	bob.json("POST", "/api/me/runner/token", map[string]any{"label": "bob's laptop"}, 201, &bt)
	ar, br := &client{f: f, bearer: at.Token}, &client{f: f, bearer: bt.Token}
	for _, c := range []*client{ar, br} {
		if res, b := c.do("GET", "/api/me", nil); res.StatusCode != 403 || errorCode(t, b) != "token_scope" {
			t.Fatalf("runner token reached /api/me: %d %s", res.StatusCode, b)
		}
	}

	_, apub, _ := sealing.NewKey()
	_, bpub, _ := sealing.NewKey()
	ar.json("POST", "/api/runner/keys", map[string]any{"public_keys": [][]byte{apub}}, 200, nil)
	ar.json("PUT", "/api/runner/connectors", map[string]any{"connectors": []map[string]any{{"id": "fake", "name": "Fake Bank", "fields": fields}}}, 204, nil)
	if res, _ := br.do("POST", "/api/runner/keys", map[string]any{"public_keys": [][]byte{apub}}); res.StatusCode != 422 {
		t.Fatalf("bob's runner took over alice's key: %d", res.StatusCode)
	}
	br.json("POST", "/api/runner/keys", map[string]any{"public_keys": [][]byte{bpub}}, 200, nil)
	br.json("PUT", "/api/runner/connectors", map[string]any{"connectors": []map[string]any{
		{"id": "fake", "name": "Fake Bank (bob's copy)", "fields": fields}, {"id": "evil", "name": "Evil", "fields": fields}}}, 204, nil)

	// Each person sees their own runner's key and connectors.
	var ac, bc CatalogDTO
	alice.json("GET", "/api/connectors", nil, 200, &ac)
	bob.json("GET", "/api/connectors", nil, 200, &bc)
	if ac.Key == nil || !bytes.Equal(ac.Key.PublicKey, apub) || len(ac.Connectors) != 1 || ac.Connectors[0].Name != "Fake Bank" {
		t.Fatalf("alice catalog = %+v", ac)
	}
	if bc.Key == nil || !bytes.Equal(bc.Key.PublicKey, bpub) || len(bc.Connectors) != 2 {
		t.Fatalf("bob catalog = %+v", bc)
	}
	var bs RunnerStatusDTO
	bob.json("GET", "/api/me/runner", nil, 200, &bs)
	if len(bs.Tokens) != 1 || bs.Tokens[0].LastUsedAt == nil || len(bs.Keys) != 1 || len(bs.Connectors) != 2 {
		t.Fatalf("bob's runner = %+v", bs) // used at once: Settings shows it linked
	}

	connect := func(c *client, userID int64, cat CatalogDTO, pub []byte) int64 {
		t.Helper()
		var b BookDTO
		c.json("POST", "/api/books", map[string]string{"name": "Home", "base_currency": "TWD"}, 201, &b)
		blob, _ := sealing.Seal(pub, []byte(`{"username":"u","password":"p"}`), connections.CredentialsAAD(userID, "fake"))
		var cr CreatedDTO
		c.json("POST", "/api/me/connections", map[string]any{"book_id": b.ID, "connector": "fake", "key_id": cat.Key.ID, "sealed": blob}, 201, &cr)
		return cr.ID
	}
	// Sealed to someone else's runner is refused.
	var bb BookDTO
	bob.json("POST", "/api/books", map[string]string{"name": "Scratch", "base_currency": "TWD"}, 201, &bb)
	wrong, _ := sealing.Seal(apub, []byte(`{}`), connections.CredentialsAAD(bm.ID, "fake"))
	if res, _ := bob.do("POST", "/api/me/connections", map[string]any{"book_id": bb.ID, "connector": "fake", "key_id": ac.Key.ID, "sealed": wrong}); res.StatusCode != 422 {
		t.Fatalf("bob sealed to alice's runner: %d", res.StatusCode)
	}
	aliceConn := connect(alice, am.ID, ac, apub)
	bobConn := connect(bob, bm.ID, bc, bpub)

	var aj, bj []JobDTO
	ar.json("POST", "/api/runner/jobs/claim", map[string]any{}, 200, &aj)
	br.json("POST", "/api/runner/jobs/claim", map[string]any{}, 200, &bj)
	if len(aj) != 1 || aj[0].ID != aliceConn || len(bj) != 1 || bj[0].ID != bobConn {
		t.Fatalf("alice's runner claimed %+v, bob's claimed %+v", aj, bj)
	}
	// Neither can work on the other's claimed connection.
	if res, _ := br.do("POST", fmt.Sprintf("/api/runner/connections/%d/finish", aliceConn), map[string]any{"status": "ok"}); res.StatusCode != 409 {
		t.Fatalf("bob's runner finished alice's connection: %d", res.StatusCode)
	}
	if res, _ := ar.do("POST", fmt.Sprintf("/api/runner/connections/%d/imports", bobConn), map[string]any{"connector": "fake", "rows": []any{}}); res.StatusCode != 409 {
		t.Fatalf("alice's runner imported into bob's connection: %d", res.StatusCode)
	}
	ar.json("POST", fmt.Sprintf("/api/runner/connections/%d/finish", aliceConn), map[string]any{"status": "ok"}, 204, nil)
	br.json("POST", fmt.Sprintf("/api/runner/connections/%d/finish", bobConn), map[string]any{"status": "ok"}, 204, nil)

	// Bob links his runner again (same volume, new token): the old token
	// stops working, the key and his connection carry on.
	var bt2 TokenCreatedDTO
	bob.json("POST", "/api/me/runner/token", map[string]any{"label": "bob's laptop"}, 201, &bt2)
	if res, _ := br.do("POST", "/api/runner/jobs/claim", map[string]any{}); res.StatusCode != 401 {
		t.Fatalf("the replaced token = %d", res.StatusCode)
	}
	br2 := &client{f: f, bearer: bt2.Token}
	bob.json("GET", "/api/me/runner", nil, 200, &bs)
	if len(bs.Tokens) != 1 || bs.Tokens[0].ID != bt2.Session.ID || bs.Tokens[0].LastUsedAt != nil {
		t.Fatalf("bob's tokens after relinking = %+v", bs.Tokens)
	}
	bob.json("POST", fmt.Sprintf("/api/me/connections/%d/sync", bobConn), nil, 204, nil)
	br2.json("POST", "/api/runner/jobs/claim", map[string]any{}, 200, &bj)
	if len(bj) != 1 || bj[0].ID != bobConn {
		t.Fatalf("after relinking, bob's runner claimed %+v", bj)
	}
	// Alice's runner is untouched by any of it.
	var as RunnerStatusDTO
	alice.json("GET", "/api/me/runner", nil, 200, &as)
	if len(as.Tokens) != 1 || len(as.Keys) != 1 {
		t.Fatalf("alice's runner = %+v", as)
	}
}

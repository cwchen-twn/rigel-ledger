package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/dbtest"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
	"github.com/cwchen-twn/rigel-ledger/web"
)

type apiFixture struct {
	t   *testing.T
	srv *httptest.Server
	svc *ledger.Service
}

func newAPI(t *testing.T) *apiFixture {
	t.Helper()
	store := dbtest.New(t)
	svc := ledger.NewService(store)
	h := New(Deps{
		Service:     svc,
		Auth:        auth.NewManager(store, time.Hour, false),
		Templates:   response.NewTemplateEngine("test", web.TemplateFiles, true),
		StaticFiles: web.StaticFiles,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	for _, name := range []string{"alice", "bob", "carol"} {
		if _, err := svc.CreateUser(context.Background(), ledger.NewUser{Username: name, Email: name + "@example.com", Password: "correct horse"}); err != nil {
			t.Fatal(err)
		}
	}
	return &apiFixture{t: t, srv: srv, svc: svc}
}

// client is one signed-in browser (cookie) or script (bearer).
type client struct {
	f      *apiFixture
	cookie *http.Cookie
	bearer string
	csrf   bool
}

func (f *apiFixture) browser(user string) *client {
	f.t.Helper()
	c := &client{f: f, csrf: true}
	res, body := c.do("POST", "/api/auth/login", map[string]string{"username": user, "password": "correct horse"})
	if res.StatusCode != http.StatusOK {
		f.t.Fatalf("login %s: %d %s", user, res.StatusCode, body)
	}
	for _, ck := range res.Cookies() {
		if ck.Name == auth.CookieName {
			c.cookie = ck
		}
	}
	if c.cookie == nil || !c.cookie.HttpOnly {
		f.t.Fatal("login did not set an HttpOnly session cookie")
	}
	return c
}

func (c *client) do(method, path string, body any) (*http.Response, []byte) {
	c.f.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.f.srv.URL+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	if c.csrf {
		req.Header.Set(auth.ClientHeader, "web")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.f.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

func (c *client) json(method, path string, body any, want int, out any) {
	c.f.t.Helper()
	res, b := c.do(method, path, body)
	if res.StatusCode != want {
		c.f.t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, want, b)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			c.f.t.Fatalf("%s %s: decode %s: %v", method, path, b, err)
		}
	}
}

func errorCode(t *testing.T, b []byte) string {
	t.Helper()
	var e response.ErrorBody
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatalf("not an error body: %s", b)
	}
	return e.Error.Code
}

func accountIDs(t *testing.T, c *client, bookID int64) map[string]int64 {
	t.Helper()
	var accts []AccountDTO
	c.json("GET", fmt.Sprintf("/api/books/%d/accounts", bookID), nil, 200, &accts)
	m := map[string]int64{}
	for _, a := range accts {
		if a.TemplateKey != nil {
			m[*a.TemplateKey] = a.ID
		}
	}
	return m
}

func TestLoginSessionAndLogout(t *testing.T) {
	f := newAPI(t)
	anon := &client{f: f}
	res, b := anon.do("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "wrong"})
	if res.StatusCode != 401 || errorCode(t, b) != "invalid_credentials" {
		t.Fatalf("wrong password: %d %s", res.StatusCode, b)
	}
	if res, _ := anon.do("GET", "/api/me", nil); res.StatusCode != 401 {
		t.Fatalf("anonymous /api/me = %d, want 401", res.StatusCode)
	}

	alice := f.browser("alice")
	var me UserDTO
	alice.json("GET", "/api/me", nil, 200, &me)
	if me.Username != "alice" || me.Language != "en" {
		t.Fatalf("me = %+v", me)
	}

	alice.json("PATCH", "/api/me/settings", map[string]any{
		"display_name": "Alice", "language": "zh", "display_currency": "twd",
		"timezone": "Asia/Taipei", "date_format": "YYYY/MM/DD", "theme": "dark", "default_book_id": nil,
	}, 200, &me)
	if me.Language != "zh" || me.DisplayCurrency != "TWD" || me.Theme != "dark" {
		t.Fatalf("settings not applied: %+v", me)
	}

	alice.json("POST", "/api/auth/logout", nil, 204, nil)
	if res, _ := alice.do("GET", "/api/me", nil); res.StatusCode != 401 {
		t.Fatalf("/api/me after logout = %d, want 401", res.StatusCode)
	}
}

func TestCSRFHeaderAndBearerTokens(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")
	alice.csrf = false
	res, b := alice.do("POST", "/api/books", map[string]string{"name": "B", "base_currency": "TWD"})
	if res.StatusCode != 403 || errorCode(t, b) != "csrf" {
		t.Fatalf("cookie POST without %s = %d %s", auth.ClientHeader, res.StatusCode, b)
	}

	var login loginResponse
	(&client{f: f}).json("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "correct horse", "client": "api"}, 200, &login)
	if login.Token == "" {
		t.Fatal("api login returned no token")
	}
	script := &client{f: f, bearer: login.Token}
	script.json("POST", "/api/books", map[string]string{"name": "B", "base_currency": "TWD"}, 201, nil)
}

func TestBookTransactionRoundTrip(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")

	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "Family", "base_currency": "twd"}, 201, &book)
	if book.BaseCurrency != "TWD" || book.Role != "owner" {
		t.Fatalf("book = %+v", book)
	}
	ids := accountIDs(t, alice, book.ID)
	base := fmt.Sprintf("/api/books/%d", book.ID)

	var usd AccountDTO
	alice.json("POST", base+"/accounts", map[string]any{
		"class": "asset", "name": "Chase", "commodity": "USD", "is_cash": true,
		"opening_balance": map[string]any{"amount": "100.00", "base_amount": "3210", "date": "2026-01-01"},
	}, 201, &usd)

	alice.json("POST", base+"/prices", map[string]any{"commodity": "USD", "quote": "TWD", "date": "2026-09-01", "rate": "32.1"}, 201, nil)
	var rate RateDTO
	alice.json("GET", base+"/rate?from=usd&to=twd&date=2026-09-15", nil, 200, &rate)
	if rate.Rate == nil || rate.Rate.String() != "32.1" {
		t.Fatalf("rate = %+v", rate)
	}

	var txn TransactionDTO
	alice.json("POST", base+"/transactions", map[string]any{
		"date": "2026-09-15", "payee": "Delta", "tags": []string{"trip"},
		"lines": []map[string]any{
			{"account_id": ids["travel"], "commodity": "USD", "amount": "10.5"},
			{"account_id": usd.ID, "amount": "-10.5"},
		},
	}, 201, &txn)
	if txn.Postings[0].BaseAmount.String() != "337.05" || txn.Date.Format(time.DateOnly) != "2026-09-15" {
		t.Fatalf("txn = %+v", txn)
	}

	res, b := alice.do("POST", base+"/transactions", map[string]any{
		"date": "2026-09-15",
		"lines": []map[string]any{
			{"account_id": ids["groceries"], "amount": "100"},
			{"account_id": ids["cash"], "amount": "-90"},
		},
	})
	if res.StatusCode != 422 || errorCode(t, b) != "unbalanced" {
		t.Fatalf("unbalanced = %d %s", res.StatusCode, b)
	}

	var page TransactionPageDTO
	alice.json("GET", base+"/transactions?tag=trip", nil, 200, &page)
	if len(page.Transactions) != 1 || page.Transactions[0].ID != txn.ID {
		t.Fatalf("page = %+v", page)
	}

	var bal BalancesDTO
	alice.json("GET", base+"/balances?as_of=2026-09-30", nil, 200, &bal)
	if !bal.Check.IsZero() {
		t.Fatalf("check = %s", bal.Check)
	}
	for _, a := range bal.Accounts {
		if a.AccountID == usd.ID && a.TotalAmounts[0].Amount.String() != "89.5" {
			t.Fatalf("Chase = %+v, want 89.5 USD", a)
		}
	}

	// Money goes over the wire as strings.
	_, raw := alice.do("GET", base+"/transactions/"+fmt.Sprint(txn.ID), nil)
	if !strings.Contains(string(raw), `"amount":"10.5"`) {
		t.Fatalf("amount not a JSON string: %s", raw)
	}
}

func TestBookAccessIsMembershipScoped(t *testing.T) {
	f := newAPI(t)
	alice, bob, carol := f.browser("alice"), f.browser("bob"), f.browser("carol")
	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "Family", "base_currency": "TWD"}, 201, &book)
	base := fmt.Sprintf("/api/books/%d", book.ID)
	ids := accountIDs(t, alice, book.ID)

	if res, b := carol.do("GET", base+"/accounts", nil); res.StatusCode != 404 || errorCode(t, b) != "not_found" {
		t.Fatalf("non-member = %d %s, want 404", res.StatusCode, b)
	}

	alice.json("POST", base+"/members", map[string]string{"username": "bob", "role": "viewer"}, 204, nil)
	bob.json("GET", base+"/accounts", nil, 200, nil)
	res, b := bob.do("POST", base+"/transactions", map[string]any{
		"date":  "2026-09-01",
		"lines": []map[string]any{{"account_id": ids["rent"], "amount": "1"}, {"account_id": ids["cash"], "amount": "-1"}},
	})
	if res.StatusCode != 403 || errorCode(t, b) != "role" {
		t.Fatalf("viewer write = %d %s, want 403", res.StatusCode, b)
	}

	var books []BookDTO
	bob.json("GET", "/api/books", nil, 200, &books)
	if len(books) != 1 || books[0].Role != "viewer" {
		t.Fatalf("bob's books = %+v", books)
	}
}

func TestShellAndUnknownRoutes(t *testing.T) {
	f := newAPI(t)
	res, err := http.Get(f.srv.URL + "/b/1/transactions")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	csp := res.Header.Get("Content-Security-Policy")
	if res.StatusCode != 200 || !strings.Contains(string(body), `<div id="app">`) || !strings.Contains(csp, "nonce-") {
		t.Fatalf("shell = %d, csp %q", res.StatusCode, csp)
	}

	res, err = http.Get(f.srv.URL + "/api/nope")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 404 || errorCode(t, b) != "not_found" {
		t.Fatalf("unknown api route = %d %s", res.StatusCode, b)
	}
}

func TestIdentityAndCommoditiesAPI(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")

	res, b := alice.do("PATCH", "/api/me/account", map[string]string{"username": "alicia", "email": "a@example.com", "current_password": "nope"})
	if res.StatusCode != 422 || errorCode(t, b) != "invalid_input" {
		t.Fatalf("wrong password = %d %s", res.StatusCode, b)
	}
	var me UserDTO
	alice.json("PATCH", "/api/me/account", map[string]string{"username": "alicia", "email": "a@example.com", "current_password": "correct horse"}, 200, &me)
	if me.Username != "alicia" {
		t.Fatalf("me = %+v", me)
	}
	// The session survives the rename.
	alice.json("GET", "/api/me", nil, 200, &me)

	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "B", "base_currency": "TWD"}, 201, &book)
	var c CommodityDTO
	alice.json("POST", fmt.Sprintf("/api/books/%d/commodities", book.ID),
		map[string]any{"code": "MILES:EVA", "kind": "points", "name": "EVA miles"}, 201, &c)
	if c.Kind != "points" || c.Decimals != 0 || c.QuoteCurrency != nil {
		t.Fatalf("commodity = %+v", c)
	}
	var all []CommodityDTO
	alice.json("GET", "/api/commodities", nil, 200, &all)
	if all[0].Kind != "currency" || all[len(all)-1].Code != "MILES:EVA" {
		t.Fatalf("commodities order: first %+v last %+v", all[0], all[len(all)-1])
	}

	var acct AccountDTO
	alice.json("POST", fmt.Sprintf("/api/books/%d/accounts", book.ID),
		map[string]any{"class": "asset", "name": "EVA miles", "commodity": "MILES:EVA",
			"opening_balance": map[string]any{"amount": "5000", "base_amount": "3000", "date": "2026-01-01"}}, 201, &acct)
	var cb CostBasisDTO
	alice.json("GET", fmt.Sprintf("/api/books/%d/accounts/%d/cost?as_of=2026-02-01", book.ID, acct.ID), nil, 200, &cb)
	if cb.Quantity.String() != "5000" || cb.UnitCost.String() != "0.6" {
		t.Fatalf("cost basis = %+v", cb)
	}
}

func TestProbes(t *testing.T) {
	f := newAPI(t)
	for _, path := range []string{"/livez", "/readyz"} {
		res, err := http.Get(f.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("%s = %d, want 200", path, res.StatusCode)
		}
	}

	// Readiness follows the database; liveness does not.
	h := New(Deps{
		Service: f.svc, Auth: auth.NewManager(nil, time.Hour, false),
		Templates: response.NewTemplateEngine("test", web.TemplateFiles, true),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Ready:     func(context.Context) error { return fmt.Errorf("down") },
	})
	srv := httptest.NewServer(h)
	defer srv.Close()
	for path, want := range map[string]int{"/livez": 200, "/readyz": 503} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("down db: %s = %d, want %d", path, res.StatusCode, want)
		}
	}
}

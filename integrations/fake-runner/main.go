// fake-runner is a stand-in sync runner for development and end-to-end
// tests: it speaks the whole runner protocol (keys, connectors, claims,
// sealed credentials, challenges, batches) against one pretend institution,
// "fake", so the app side can be exercised without a real bank. The real
// runners (integrations/tw-sync, P4c-2) follow the same steps.
//
//	RUNNER_TOKEN=... go run ./integrations/fake-runner -url http://localhost:8080 -key /tmp/fake-runner.key
//
// The pretend institution: any username; password "wrong" fails with
// bad_credentials; password "otp" asks for a one-time code, and the code is
// 123456. It sends one checking account with three transactions and a
// balance, the same ids every day, so a re-sync stages nothing new.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/connections"
	"github.com/cwchen-twn/rigel-ledger/internal/sealing"
)

type runner struct {
	url, token string
	priv       []byte
	http       *http.Client
	log        *slog.Logger
}

func (r *runner) call(ctx context.Context, method, path string, in, out any) error {
	var body bytes.Buffer
	if in != nil {
		if err := json.NewEncoder(&body).Encode(in); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, r.url+path, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		var e struct {
			Error struct{ Code, Message string }
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		return fmt.Errorf("%s %s: %d %s %s", method, path, res.StatusCode, e.Error.Code, e.Error.Message)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

func loadKey(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		return base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	}
	priv, _, err := sealing.NewKey()
	if err != nil {
		return nil, err
	}
	return priv, os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0o600)
}

type job struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Connector string `json:"connector"`
	Sealed    []byte `json:"sealed"`
}

func (r *runner) setup(ctx context.Context) error {
	pub, err := sealing.PublicKey(r.priv)
	if err != nil {
		return err
	}
	if err := r.call(ctx, "POST", "/api/runner/keys", map[string]any{"public_keys": [][]byte{pub}}, nil); err != nil {
		return err
	}
	return r.call(ctx, "PUT", "/api/runner/connectors", map[string]any{"connectors": []map[string]any{{
		"id": "fake", "name": "Fake Bank", "country": "ZZ",
		"fields": []map[string]any{
			{"name": "username", "label": "Username", "kind": "text"},
			{"name": "password", "label": "Password", "kind": "secret"},
		},
	}}}, nil)
}

func (r *runner) finish(ctx context.Context, id int64, status, code string) {
	if err := r.call(ctx, "POST", fmt.Sprintf("/api/runner/connections/%d/finish", id), map[string]string{"status": status, "error": code}, nil); err != nil {
		r.log.Error("finish", "connection", id, "error", err)
	}
}

// otp asks the owner for a code and waits for it.
func (r *runner) otp(ctx context.Context, id int64) (string, error) {
	var ch struct {
		ID int64 `json:"id"`
	}
	if err := r.call(ctx, "POST", fmt.Sprintf("/api/runner/connections/%d/challenges", id),
		map[string]any{"kind": "otp", "prompt": "Fake Bank sent a code by SMS (it is 123456)", "ttl_seconds": 300}, &ch); err != nil {
		return "", err
	}
	for {
		var a struct {
			Answered bool   `json:"answered"`
			Expired  bool   `json:"expired"`
			Sealed   []byte `json:"sealed"`
		}
		if err := r.call(ctx, "GET", fmt.Sprintf("/api/runner/connections/%d/challenges/%d", id, ch.ID), nil, &a); err != nil {
			return "", err
		}
		if a.Expired {
			return "", errExpired
		}
		if a.Answered && a.Sealed != nil {
			code, err := sealing.Open(r.priv, a.Sealed, connections.AnswerAAD(ch.ID))
			return string(code), err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

var errExpired = fmt.Errorf("challenge expired")

func (r *runner) run(ctx context.Context, j job) {
	log := r.log.With("connection", j.ID, "connector", j.Connector)
	plain, err := sealing.Open(r.priv, j.Sealed, connections.CredentialsAAD(j.UserID, j.Connector))
	if err != nil {
		log.Warn("credentials do not open", "error", err)
		r.finish(ctx, j.ID, "failed", "credentials_unreadable")
		return
	}
	var creds map[string]string
	if err := json.Unmarshal(plain, &creds); err != nil || creds["username"] == "" {
		r.finish(ctx, j.ID, "failed", "bad_credentials")
		return
	}
	switch creds["password"] {
	case "wrong":
		r.finish(ctx, j.ID, "failed", "bad_credentials")
		return
	case "otp":
		code, err := r.otp(ctx, j.ID)
		if err == errExpired {
			r.finish(ctx, j.ID, "failed", "challenge_expired")
			return
		}
		if err != nil || code != "123456" {
			r.finish(ctx, j.ID, "failed", "bad_otp")
			return
		}
	}
	day := time.Now().UTC().Format("2006-01-02")
	batch := map[string]any{
		"connector": j.Connector, "label": "Fake Bank " + day,
		"accounts": []map[string]string{{"id": "fake-chk-001", "label": "Fake Bank checking ...001", "currency": "TWD"}},
		"rows": []map[string]any{
			{"kind": "transaction", "account": "fake-chk-001", "id": day + "-1", "date": day, "amount": "-120", "description": "7-ELEVEN 0931"},
			{"kind": "transaction", "account": "fake-chk-001", "id": day + "-2", "date": day, "amount": "-1580", "description": "PX MART"},
			{"kind": "transaction", "account": "fake-chk-001", "id": day + "-3", "date": day, "amount": "42000", "description": "SALARY ACME"},
			{"kind": "balance", "account": "fake-chk-001", "id": day + "-bal", "date": day, "amount": "40300"},
		},
	}
	var res struct {
		Staged     int `json:"staged"`
		Duplicates int `json:"duplicates"`
	}
	if err := r.call(ctx, "POST", fmt.Sprintf("/api/runner/connections/%d/imports", j.ID), batch, &res); err != nil {
		log.Error("import", "error", err)
		r.finish(ctx, j.ID, "failed", "import_rejected")
		return
	}
	log.Info("synced", "staged", res.Staged, "duplicates", res.Duplicates)
	r.finish(ctx, j.ID, "ok", "")
}

func main() {
	url := flag.String("url", "http://localhost:8080", "the app")
	keyPath := flag.String("key", "fake-runner.key", "private key file (created when missing)")
	every := flag.Duration("every", 3*time.Second, "poll interval")
	once := flag.Bool("once", false, "claim and run once, then exit")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	token := os.Getenv("RUNNER_TOKEN")
	if token == "" {
		log.Error("RUNNER_TOKEN is not set (rigel-ledger-cli create-runner-token, or Administration -> Sync runner)")
		os.Exit(2)
	}
	priv, err := loadKey(*keyPath)
	if err != nil {
		log.Error("key", "error", err)
		os.Exit(1)
	}
	r := &runner{url: strings.TrimRight(*url, "/"), token: token, priv: priv, http: &http.Client{Timeout: 30 * time.Second}, log: log}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := r.setup(ctx); err != nil {
		log.Error("setup", "error", err)
		os.Exit(1)
	}
	log.Info("fake runner ready", "url", r.url)
	for {
		var jobs []job
		if err := r.call(ctx, "POST", "/api/runner/jobs/claim", map[string]int{"limit": 5}, &jobs); err != nil {
			log.Error("claim", "error", err)
		}
		for _, j := range jobs {
			if j.Connector != "fake" {
				r.finish(ctx, j.ID, "failed", "connector_unavailable")
				continue
			}
			r.run(ctx, j)
		}
		if *once {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(*every):
		}
	}
}

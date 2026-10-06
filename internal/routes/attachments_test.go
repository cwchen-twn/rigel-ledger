package routes

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
)

// upload sends one file as the multipart field "file", as the browser does.
func (c *client) upload(path, filename string, content []byte) (*http.Response, []byte) {
	c.f.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", filename)
	_, _ = part.Write(content)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", c.f.srv.URL+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
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

func TestAttachmentEndpoints(t *testing.T) {
	f := newAPI(t)
	alice, bob, carol := f.browser("alice"), f.browser("bob"), f.browser("carol")
	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "Family", "base_currency": "TWD"}, 201, &book)
	base := fmt.Sprintf("/api/books/%d", book.ID)
	ids := accountIDs(t, alice, book.ID)
	var txn TransactionDTO
	alice.json("POST", base+"/transactions", map[string]any{
		"date": "2026-09-15", "payee": "PX Mart",
		"lines": []map[string]any{{"account_id": ids["groceries"], "amount": "320"}, {"account_id": ids["cash"], "amount": "-320"}},
	}, 201, &txn)
	files := fmt.Sprintf("%s/transactions/%d/attachments", base, txn.ID)
	jpeg := append([]byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"), bytes.Repeat([]byte{0x42}, 100)...)

	res, b := alice.upload(files, "收據.jpg", jpeg)
	if res.StatusCode != 201 {
		t.Fatalf("upload = %d %s", res.StatusCode, b)
	}
	if !strings.Contains(string(b), `"mime":"image/jpeg"`) || !strings.Contains(string(b), `"filename":"收據.jpg"`) {
		t.Fatalf("attachment = %s", b)
	}
	var got TransactionDTO
	alice.json("GET", fmt.Sprintf("%s/transactions/%d", base, txn.ID), nil, 200, &got)
	if len(got.Attachments) != 1 {
		t.Fatalf("transaction lists %+v", got.Attachments)
	}
	at := got.Attachments[0]
	var page TransactionPageDTO
	alice.json("GET", base+"/transactions", nil, 200, &page)
	if len(page.Transactions) != 1 || len(page.Transactions[0].Attachments) != 1 {
		t.Fatalf("the list does not show the attachment: %+v", page.Transactions)
	}

	// Served with its stored type, never sniffed, and sandboxed.
	fileURL := fmt.Sprintf("%s/attachments/%d", base, at.ID)
	res, b = alice.do("GET", fileURL, nil)
	if res.StatusCode != 200 || !bytes.Equal(b, jpeg) || res.Header.Get("Content-Type") != "image/jpeg" ||
		res.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(res.Header.Get("Content-Security-Policy"), "sandbox") ||
		!strings.HasPrefix(res.Header.Get("Content-Disposition"), "inline") {
		t.Fatalf("file = %d %v", res.StatusCode, res.Header)
	}
	if res, _ = alice.do("GET", fileURL+"?download=1", nil); !strings.HasPrefix(res.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("download disposition = %q", res.Header.Get("Content-Disposition"))
	}

	// HTML named .png is still HTML: refused.
	res, b = alice.upload(files, "x.png", []byte("<html><script>alert(1)</script></html>"))
	if res.StatusCode != 422 || !strings.Contains(string(b), `"file":"unsupported_type"`) {
		t.Fatalf("html upload = %d %s", res.StatusCode, b)
	}
	res, b = alice.do("POST", files, nil)
	if res.StatusCode != 422 || !strings.Contains(string(b), `"file":"required"`) {
		t.Fatalf("no file = %d %s", res.StatusCode, b)
	}
	res, b = alice.upload(files, "big.pdf", append([]byte("%PDF-"), make([]byte, ledger.MaxAttachmentBytes+128<<10)...))
	if res.StatusCode != 422 || !strings.Contains(string(b), `"file":"too_large"`) {
		t.Fatalf("too large = %d %.200s", res.StatusCode, b)
	}

	// Someone else's book: not found. A viewer reads, and cannot write.
	if res, _ = carol.do("GET", fileURL, nil); res.StatusCode != 404 {
		t.Fatalf("non-member file = %d", res.StatusCode)
	}
	alice.json("POST", base+"/members", map[string]string{"username": "bob", "role": "viewer"}, 204, nil)
	if res, _ = bob.do("GET", fileURL, nil); res.StatusCode != 200 {
		t.Fatalf("viewer file = %d", res.StatusCode)
	}
	if res, b = bob.upload(files, "x.jpg", jpeg); res.StatusCode != 403 {
		t.Fatalf("viewer upload = %d %s", res.StatusCode, b)
	}
	// An import token reaches none of it.
	var made TokenCreatedDTO
	alice.json("POST", "/api/me/tokens", map[string]any{"label": "script", "days": 1}, 201, &made)
	script := &client{f: f, bearer: made.Token}
	if res, b = script.upload(files, "x.jpg", jpeg); res.StatusCode != 403 || errorCode(t, b) != "token_scope" {
		t.Fatalf("token upload = %d %s", res.StatusCode, b)
	}
	if res, b = script.do("GET", fileURL, nil); res.StatusCode != 403 {
		t.Fatalf("token download = %d %s", res.StatusCode, b)
	}

	alice.json("DELETE", fmt.Sprintf("%s/%d", files, at.ID), nil, 204, nil)
	if res, _ = alice.do("GET", fileURL, nil); res.StatusCode != 404 {
		t.Fatalf("the file outlived its last link: %d", res.StatusCode)
	}
}

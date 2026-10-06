package ledger

import (
	"testing"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// Just enough of each format for content sniffing.
var (
	pngFile = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR receipt")
	pdfFile = []byte("%PDF-1.7\n1 0 obj << >> endobj\n%%EOF\n")
	svgFile = []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
)

func (f *fixture) lunch(t *testing.T) int64 {
	t.Helper()
	v, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date: day("2026-09-01"), Payee: "Lunch",
		Lines: []LineInput{{AccountID: f.keys["groceries"], Amount: d("180")}, {AccountID: f.keys["cash"], Amount: d("-180")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return v.ID
}

func (f *fixture) files(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.store.Pool.QueryRow(f.ctx, `SELECT count(*) FROM attachments WHERE book_id = $1`, f.acc.Book.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAttachmentsOnATransaction(t *testing.T) {
	f := setup(t)
	lunch, dinner := f.lunch(t), f.lunch(t)

	at, err := f.svc.AttachFile(f.ctx, f.acc, lunch, FileInput{Filename: `C:\Users\me\IMG_0042.HEIC`, Bytes: pngFile})
	if err != nil {
		t.Fatal(err)
	}
	// The type is the content's, and the name follows it.
	if at.Mime != "image/png" || at.Filename != "IMG_0042.png" || at.Size != int32(len(pngFile)) {
		t.Fatalf("attached = %+v", at)
	}
	// The same receipt on another transaction is one file with two links.
	if _, err := f.svc.AttachFile(f.ctx, f.acc, dinner, FileInput{Filename: "again.png", Bytes: pngFile}); err != nil {
		t.Fatal(err)
	}
	if n := f.files(t); n != 1 {
		t.Fatalf("files = %d, want 1 (one copy per content)", n)
	}
	v, err := f.svc.GetTransaction(f.ctx, f.acc, lunch)
	if err != nil || len(v.Attachments) != 1 || v.Attachments[0].ID != at.ID {
		t.Fatalf("lunch lists %+v (%v)", v.Attachments, err)
	}
	file, err := f.svc.File(f.ctx, f.acc, at.ID)
	if err != nil || string(file.Bytes) != string(pngFile) || file.Mime != "image/png" {
		t.Fatalf("file = %+v (%v)", file.Filename, err)
	}

	// Not an image or a PDF, empty, or too big: refused with the field named.
	_, err = f.svc.AttachFile(f.ctx, f.acc, lunch, FileInput{Filename: "x.png", Bytes: svgFile})
	if le := wantCode(t, err, "invalid_input"); le.Fields["file"] != "unsupported_type" {
		t.Fatalf("svg: %+v", le.Fields)
	}
	_, err = f.svc.AttachFile(f.ctx, f.acc, lunch, FileInput{Filename: "x.pdf", Bytes: append([]byte("%PDF-"), make([]byte, MaxAttachmentBytes)...)})
	if le := wantCode(t, err, "invalid_input"); le.Fields["file"] != "too_large" {
		t.Fatalf("too large: %+v", le.Fields)
	}
	_, err = f.svc.AttachFile(f.ctx, f.acc, 999999, FileInput{Filename: "x.pdf", Bytes: pdfFile})
	wantCode(t, err, "not_found")

	// A viewer reads the file, but cannot attach or detach.
	bob := newUser(t, f, "bob")
	if err := f.svc.AddMember(f.ctx, f.acc, "bob", db.MemberRoleViewer); err != nil {
		t.Fatal(err)
	}
	bobAcc, err := f.svc.ResolveAccess(f.ctx, bob.ID, f.acc.Book.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.File(f.ctx, bobAcc, at.ID); err != nil {
		t.Fatalf("a viewer could not open a receipt: %v", err)
	}
	_, err = f.svc.AttachFile(f.ctx, bobAcc, lunch, FileInput{Filename: "x.pdf", Bytes: pdfFile})
	wantCode(t, err, "role")
	wantCode(t, f.svc.DetachFile(f.ctx, bobAcc, lunch, at.ID), "role")

	// The file stays while dinner still holds it, and goes with the last link.
	if err := f.svc.DetachFile(f.ctx, f.acc, lunch, at.ID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.svc.DetachFile(f.ctx, f.acc, lunch, at.ID), "not_found")
	if n := f.files(t); n != 1 {
		t.Fatalf("files = %d after one of two links went", n)
	}
	if err := f.svc.DeleteTransaction(f.ctx, f.acc, dinner); err != nil {
		t.Fatal(err)
	}
	if n := f.files(t); n != 0 {
		t.Fatalf("files = %d after the last link went with its transaction", n)
	}
}

func TestCleanFilename(t *testing.T) {
	for in, want := range map[string]string{
		"../../etc/passwd":      "passwd.pdf",
		"statement.PDF":         "statement.PDF",
		"photo.jpeg":            "photo.jpeg",
		"":                      "file.pdf",
		"bad\x00name\n.pdf":     "badname.pdf",
		`C:\Users\me\收據 9月.pdf`: "收據 9月.pdf",
	} {
		mime := "application/pdf"
		if want == "photo.jpeg" {
			mime = "image/jpeg"
		}
		if got := cleanFilename(in, mime); got != want {
			t.Errorf("cleanFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

// A source's evidence waits with its row and lands on the transaction the
// row becomes; a resent batch stores nothing new.
func TestImportEvidenceFollowsTheRow(t *testing.T) {
	f := setup(t)
	in := bankBatch(
		ImportRowInput{Kind: "transaction", Account: "card", ID: "o-1", Date: day("2026-09-03"), Amount: d("-890"), Description: "momo order 12345", File: "mail-1"},
		ImportRowInput{Kind: "transaction", Account: "card", ID: "o-2", Date: day("2026-09-04"), Amount: d("-120"), Description: "no evidence"},
	)
	in.Files = []ImportFile{{Ref: "mail-1", FileInput: FileInput{Filename: "order.pdf", Bytes: pdfFile}}}

	bad := in
	bad.Rows = []ImportRowInput{{Kind: "transaction", Account: "card", ID: "x", Date: day("2026-09-03"), Amount: d("-1"), File: "nope"}}
	_, err := f.svc.Import(f.ctx, f.acc, bad)
	if le := wantCode(t, err, "invalid_input"); le.Fields["rows[0].file"] != "unknown" {
		t.Fatalf("unknown ref: %+v", le.Fields)
	}

	if _, err := f.svc.Import(f.ctx, f.acc, in); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Import(f.ctx, f.acc, in); err != nil {
		t.Fatal(err)
	}
	if n := f.files(t); n != 1 {
		t.Fatalf("files = %d after a resend, want 1", n)
	}
	f.mapSource(t, "card", f.keys["credit_card"])
	q := f.queue(t)
	if q["testbank:o-1"].AttachmentID == nil || q["testbank:o-2"].AttachmentID != nil {
		t.Fatalf("queue evidence = %v, %v", q["testbank:o-1"].AttachmentID, q["testbank:o-2"].AttachmentID)
	}
	groceries := f.keys["groceries"]
	txn, err := f.svc.AcceptRow(f.ctx, f.acc, q["testbank:o-1"].ID, &groceries)
	if err != nil {
		t.Fatal(err)
	}
	v, err := f.svc.GetTransaction(f.ctx, f.acc, txn)
	if err != nil || len(v.Attachments) != 1 || v.Attachments[0].Filename != "order.pdf" {
		t.Fatalf("accepted transaction carries %+v (%v)", v.Attachments, err)
	}
}

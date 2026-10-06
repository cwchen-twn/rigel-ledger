package ledger

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// Attachments (#36, migration 000009): files on transactions -- the receipt a
// person photographs, and the evidence a source sends with a row. A book
// keeps one copy per content (sha256); a file goes when nothing points at it.
// The lock date does not stop them: a receipt changes no figure.

// MaxAttachmentBytes caps one file. Images are downscaled in the browser
// first, so this is room for a statement PDF.
const MaxAttachmentBytes = 10 << 20

// attachmentTypes are what may be stored, decided from the bytes (never the
// client's word): images and PDFs. HTML and SVG would run script when
// opened from the app's origin.
var attachmentTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"application/pdf": ".pdf",
}

// FileInput is a file as it arrives: its name as the person or source gave
// it, and its bytes.
type FileInput struct {
	Filename string
	Bytes    []byte
}

// Attachment is a file's metadata, as a transaction lists it.
type Attachment struct {
	ID        int64
	Filename  string
	Mime      string
	Size      int32
	CreatedAt time.Time
}

// AttachmentFile is a file with its bytes, for download.
type AttachmentFile = db.GetAttachmentFileRow

// sniffType names a file's type from its content.
func sniffType(b []byte) (string, bool) {
	mime, _, _ := strings.Cut(http.DetectContentType(b), ";")
	_, ok := attachmentTypes[mime]
	return mime, ok
}

// cleanFilename keeps a display name: no directories, no control
// characters, at most 200 characters, and the extension of its real type.
func cleanFilename(name, mime string) string {
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == "" {
		name = "file"
	}
	ext := attachmentTypes[mime]
	if strings.ToLower(path.Ext(name)) != ext && !(ext == ".jpg" && strings.EqualFold(path.Ext(name), ".jpeg")) {
		name = strings.TrimSuffix(name, path.Ext(name)) + ext
	}
	if r := []rune(name); len(r) > 200 {
		name = string(r[:200-len(ext)]) + ext
	}
	return name
}

// storeFile keeps a file in the book, or finds the copy it already holds.
// field names the input in errors.
func (s *Service) storeFile(ctx context.Context, q *db.Queries, a Access, f FileInput, field string) (int64, error) {
	if len(f.Bytes) == 0 {
		return 0, fieldError(field, "required", "the file is empty")
	}
	if len(f.Bytes) > MaxAttachmentBytes {
		return 0, fieldError(field, "too_large", "a file is at most %d MiB", MaxAttachmentBytes>>20)
	}
	mime, ok := sniffType(f.Bytes)
	if !ok {
		return 0, fieldError(field, "unsupported_type", "only images (JPEG, PNG, WebP) and PDFs can be attached")
	}
	sum := sha256.Sum256(f.Bytes)
	id, err := q.InsertAttachment(ctx, db.InsertAttachmentParams{
		BookID: a.Book.ID, Sha256: sum[:], Filename: cleanFilename(f.Filename, mime), Mime: mime,
		Size: int32(len(f.Bytes)), Bytes: f.Bytes, CreatedBy: &a.UserID,
	})
	if errors.Is(err, pgx.ErrNoRows) { // the book holds this content already
		return q.FindAttachment(ctx, db.FindAttachmentParams{BookID: a.Book.ID, Sha256: sum[:]})
	}
	return id, err
}

// AttachFile keeps a file on a transaction.
func (s *Service) AttachFile(ctx context.Context, a Access, txnID int64, f FileInput) (Attachment, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return Attachment{}, err
	}
	var id int64
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		if _, err := q.GetTransaction(ctx, db.GetTransactionParams{BookID: a.Book.ID, ID: txnID}); err != nil {
			return translate(err, "transaction")
		}
		var err error
		if id, err = s.storeFile(ctx, q, a, f, "file"); err != nil {
			return err
		}
		return q.LinkAttachment(ctx, db.LinkAttachmentParams{BookID: a.Book.ID, TransactionID: txnID, AttachmentID: id, CreatedBy: &a.UserID})
	})
	if err != nil {
		return Attachment{}, translate(err, "attachment")
	}
	list, err := s.attachments(ctx, []int64{txnID})
	if err != nil {
		return Attachment{}, err
	}
	for _, at := range list[txnID] {
		if at.ID == id {
			return at, nil
		}
	}
	return Attachment{}, notFound("attachment")
}

// DetachFile takes a file off a transaction; the file goes with its last link.
func (s *Service) DetachFile(ctx context.Context, a Access, txnID, attachmentID int64) error {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return err
	}
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		n, err := q.UnlinkAttachment(ctx, db.UnlinkAttachmentParams{BookID: a.Book.ID, TransactionID: txnID, AttachmentID: attachmentID})
		if err == nil && n == 0 {
			return notFound("attachment")
		}
		return err
	})
	return translate(err, "attachment")
}

// File reads a file of the book, for anyone who can read the book.
func (s *Service) File(ctx context.Context, a Access, id int64) (AttachmentFile, error) {
	f, err := s.store.GetAttachmentFile(ctx, db.GetAttachmentFileParams{BookID: a.Book.ID, ID: id})
	if err != nil {
		return AttachmentFile{}, translate(err, "attachment")
	}
	return f, nil
}

// attachments lists the files of some transactions, by transaction.
func (s *Service) attachments(ctx context.Context, txnIDs []int64) (map[int64][]Attachment, error) {
	rows, err := s.store.ListTransactionAttachments(ctx, txnIDs)
	if err != nil {
		return nil, err
	}
	out := map[int64][]Attachment{}
	for _, r := range rows {
		out[r.TransactionID] = append(out[r.TransactionID], Attachment{ID: r.ID, Filename: r.Filename, Mime: r.Mime, Size: r.Size, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

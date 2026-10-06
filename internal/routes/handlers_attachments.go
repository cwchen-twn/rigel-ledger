package routes

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

// Attachments (#36): files on transactions. Uploads are one multipart file
// each; the type is decided from the bytes (ledger.AttachFile), never from
// the request.

type AttachmentDTO struct {
	ID        int64     `json:"id"`
	Filename  string    `json:"filename"`
	Mime      string    `json:"mime" enums:"image/jpeg,image/png,image/webp,application/pdf"`
	Size      int32     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func attachmentDTO(a ledger.Attachment) AttachmentDTO {
	return AttachmentDTO{ID: a.ID, Filename: a.Filename, Mime: a.Mime, Size: a.Size, CreatedAt: a.CreatedAt}
}

func attachmentDTOs(as []ledger.Attachment) []AttachmentDTO {
	out := make([]AttachmentDTO, len(as))
	for i, a := range as {
		out[i] = attachmentDTO(a)
	}
	return out
}

// attachFile
//
//	@Summary		Attach a file to a transaction
//	@Description	A receipt photo or a PDF, up to 10 MiB, as the multipart field "file". Images, JPEG, PNG or WebP, are best downscaled first. The same content twice is kept once per book.
//	@Tags			transactions
//	@Accept			mpfd
//	@Produce		json
//	@Param			bookID			path		int		true	"book id"
//	@Param			transactionID	path		int		true	"transaction id"
//	@Param			file			formData	file	true	"the file"
//	@Success		201				{object}	AttachmentDTO
//	@Failure		422				{object}	response.ErrorBody
//	@Router			/api/books/{bookID}/transactions/{transactionID}/attachments [post]
func (h *handlers) attachFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "transactionID")
	if !ok {
		badParam(w, "transactionID", "invalid")
		return
	}
	// The file plus the multipart framing; anything larger is too large.
	r.Body = http.MaxBytesReader(w, r.Body, ledger.MaxAttachmentBytes+64<<10)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		var tooBig *http.MaxBytesError
		code := "required"
		if errors.As(err, &tooBig) {
			code = "too_large"
		}
		response.Error(w, http.StatusUnprocessableEntity, "invalid_input", "send one file as the multipart field file", map[string]string{"file": code})
		return
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, ledger.MaxAttachmentBytes+1))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	a, err := h.svc.AttachFile(r.Context(), access(r), id, ledger.FileInput{Filename: hdr.Filename, Bytes: b})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, attachmentDTO(a))
}

// detachFile
//
//	@Summary		Take a file off a transaction
//	@Description	The file itself is deleted once nothing else points at it.
//	@Tags			transactions
//	@Param			bookID			path	int	true	"book id"
//	@Param			transactionID	path	int	true	"transaction id"
//	@Param			attachmentID	path	int	true	"attachment id"
//	@Success		204
//	@Router			/api/books/{bookID}/transactions/{transactionID}/attachments/{attachmentID} [delete]
func (h *handlers) detachFile(w http.ResponseWriter, r *http.Request) {
	txn, ok1 := pathID(r, "transactionID")
	at, ok2 := pathID(r, "attachmentID")
	if !ok1 || !ok2 {
		badParam(w, "attachmentID", "invalid")
		return
	}
	if err := h.svc.DetachFile(r.Context(), access(r), txn, at); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// getFile
//
//	@Summary		A file of the book
//	@Description	Shown inline (an image, a PDF) unless download=1. Served with its stored type, nosniff, and a sandbox for images.
//	@Tags			transactions
//	@Produce		image/jpeg,image/png,image/webp,application/pdf
//	@Param			bookID			path	int		true	"book id"
//	@Param			attachmentID	path	int		true	"attachment id"
//	@Param			download		query	bool	false	"as a download"
//	@Success		200
//	@Router			/api/books/{bookID}/attachments/{attachmentID} [get]
func (h *handlers) getFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "attachmentID")
	if !ok {
		badParam(w, "attachmentID", "invalid")
		return
	}
	f, err := h.svc.File(r.Context(), access(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	hd := w.Header()
	hd.Set("Content-Type", f.Mime)
	hd.Set("Content-Length", strconv.Itoa(len(f.Bytes)))
	hd.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.Filename}))
	hd.Set("X-Content-Type-Options", "nosniff")
	// The bytes of a file never change (a new upload is a new id), but they
	// are a person's receipts: cached by their browser only.
	hd.Set("Cache-Control", "private, max-age=86400")
	if f.Mime != "application/pdf" { // the browser's PDF viewer does not run sandboxed
		hd.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(f.Bytes)
}

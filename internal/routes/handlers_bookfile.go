package routes

import (
	"encoding/json"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

// A book file of years of history is a few MB (six years and 15,000
// transactions: about 8 MB); this leaves room for a much larger one.
const maxBookFileBytes = 256 << 20

// Importing that many transactions in one database transaction takes longer
// than the server's 10 s read and 30 s write timeouts allow, so the import
// handler extends both for itself.
const bookFileDeadline = 10 * time.Minute

// exportBook
//
//	@Summary		The whole book as one file, to import elsewhere
//	@Description	Owner only. The format is ledger.BookFile ("rigelledger-book", version 1).
//	@Tags			books
//	@Produce		json
//	@Param			bookID	path		int	true	"book id"
//	@Success		200		{object}	ledger.BookFile
//	@Router			/api/books/{bookID}/export [get]
func (h *handlers) exportBook(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(bookFileDeadline))
	f, err := h.svc.ExportBook(r.Context(), access(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	name := bookFileName(f.Book.Name, f.ExportedAt)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	_ = enc.Encode(f)
}

// bookFileName is "<book>-<date>.rigelledger.json" with only safe characters.
func bookFileName(book string, at time.Time) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, strings.TrimSpace(book))
	clean = strings.Trim(clean, "-")
	if clean == "" {
		clean = "book"
	}
	return clean + "-" + at.Format("2006-01-02") + ".rigelledger.json"
}

type BookImportDTO struct {
	Book          BookDTO `json:"book"`
	Accounts      int     `json:"accounts"`
	Transactions  int     `json:"transactions"`
	Assertions    int     `json:"assertions"`
	PricesAdded   int64   `json:"prices_added"`
	PricesSkipped int     `json:"prices_skipped"`
}

// importBook
//
//	@Summary		Create a new book from a book file
//	@Description	The whole file is imported in one database transaction: on any error nothing is created. Prices only fill days with no rate.
//	@Tags			books
//	@Accept			json
//	@Produce		json
//	@Param			name	query		string			false	"name for the new book (default: the file's)"
//	@Param			body	body		ledger.BookFile	true	"book file"
//	@Success		201		{object}	BookImportDTO
//	@Failure		422		{object}	response.ErrorBody
//	@Router			/api/books/import [post]
func (h *handlers) importBook(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(bookFileDeadline))
	_ = rc.SetWriteDeadline(time.Now().Add(bookFileDeadline))
	var f ledger.BookFile
	if err := response.DecodeLimit(w, r, &f, maxBookFileBytes); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	res, err := h.svc.ImportBook(r.Context(), id.User.ID, f, r.URL.Query().Get("name"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, BookImportDTO{
		Book: bookDTO(res.Book, db.MemberRoleOwner), Accounts: res.Accounts, Transactions: res.Transactions,
		Assertions: res.Assertions, PricesAdded: res.PricesAdded, PricesSkipped: res.PricesSkipped,
	})
}

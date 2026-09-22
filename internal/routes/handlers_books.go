package routes

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

type accessKey struct{}

// bookAccess resolves {bookID} to the caller's membership once per request.
// Non-members get the same 404 as a book that does not exist.
func (h *handlers) bookAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bookID, err := strconv.ParseInt(chi.URLParam(r, "bookID"), 10, 64)
		if err != nil {
			response.Error(w, http.StatusNotFound, "not_found", "book not found", nil)
			return
		}
		id, _ := auth.FromContext(r.Context())
		a, err := h.svc.ResolveAccess(r.Context(), id.User.ID, bookID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), accessKey{}, a)))
	})
}

func access(r *http.Request) ledger.Access {
	a, _ := r.Context().Value(accessKey{}).(ledger.Access)
	return a
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil
}

// listBooks
//
//	@Summary	Books the user is a member of
//	@Tags		books
//	@Produce	json
//	@Success	200	{array}	BookDTO
//	@Router		/api/books [get]
func (h *handlers) listBooks(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	books, err := h.svc.ListBooks(r.Context(), id.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]BookDTO, len(books))
	for i, b := range books {
		out[i] = bookDTO(b.Book, b.Role)
	}
	response.JSON(w, http.StatusOK, out)
}

type createBookRequest struct {
	Name         string `json:"name"`
	BaseCurrency string `json:"base_currency"`
}

// createBook
//
//	@Summary	Create a book seeded with the personal chart of accounts
//	@Tags		books
//	@Accept		json
//	@Produce	json
//	@Param		body	body		createBookRequest	true	"book"
//	@Success	201		{object}	BookDTO
//	@Failure	422		{object}	response.ErrorBody
//	@Router		/api/books [post]
func (h *handlers) createBook(w http.ResponseWriter, r *http.Request) {
	var req createBookRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	b, err := h.svc.CreateBook(r.Context(), id.User.ID, req.Name, req.BaseCurrency)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, bookDTO(b, db.MemberRoleOwner))
}

// getBook
//
//	@Summary	One book and the caller's role in it
//	@Tags		books
//	@Produce	json
//	@Param		bookID	path		int	true	"book id"
//	@Success	200		{object}	BookDTO
//	@Router		/api/books/{bookID} [get]
func (h *handlers) getBook(w http.ResponseWriter, r *http.Request) {
	a := access(r)
	response.JSON(w, http.StatusOK, bookDTO(a.Book, a.Role))
}

type updateBookRequest struct {
	Name                    string     `json:"name"`
	LockDate                *Date      `json:"lock_date" swaggertype:"string" format:"date"`
	InterestDividendCfClass db.CfClass `json:"interest_dividend_cf_class"`
}

// updateBook
//
//	@Summary	Rename, lock, or set IAS 7 choices (owner)
//	@Tags		books
//	@Accept		json
//	@Produce	json
//	@Param		bookID	path		int					true	"book id"
//	@Param		body	body		updateBookRequest	true	"book"
//	@Success	200		{object}	BookDTO
//	@Router		/api/books/{bookID} [patch]
func (h *handlers) updateBook(w http.ResponseWriter, r *http.Request) {
	var req updateBookRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	a := access(r)
	b, err := h.svc.UpdateBook(r.Context(), a, ledger.BookUpdate{
		Name: req.Name, LockDate: req.LockDate.timePtr(), InterestDividendCfClass: req.InterestDividendCfClass,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, bookDTO(b, a.Role))
}

// listMembers
//
//	@Summary	Members of a book
//	@Tags		members
//	@Produce	json
//	@Param		bookID	path	int	true	"book id"
//	@Success	200		{array}	MemberDTO
//	@Router		/api/books/{bookID}/members [get]
func (h *handlers) listMembers(w http.ResponseWriter, r *http.Request) {
	ms, err := h.svc.ListMembers(r.Context(), access(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]MemberDTO, len(ms))
	for i, m := range ms {
		out[i] = MemberDTO{UserID: m.UserID, Username: m.Username, DisplayName: m.DisplayName, Role: m.Role}
	}
	response.JSON(w, http.StatusOK, out)
}

type addMemberRequest struct {
	Username string        `json:"username"`
	Role     db.MemberRole `json:"role"`
}

// addMember
//
//	@Summary	Add a user to a book (owner)
//	@Tags		members
//	@Accept		json
//	@Param		bookID	path	int					true	"book id"
//	@Param		body	body	addMemberRequest	true	"member"
//	@Success	204
//	@Router		/api/books/{bookID}/members [post]
func (h *handlers) addMember(w http.ResponseWriter, r *http.Request) {
	var req addMemberRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.AddMember(r.Context(), access(r), req.Username, req.Role); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

type roleRequest struct {
	Role db.MemberRole `json:"role"`
}

// updateMember
//
//	@Summary	Change a member's role (owner)
//	@Tags		members
//	@Accept		json
//	@Param		bookID	path	int			true	"book id"
//	@Param		userID	path	int			true	"user id"
//	@Param		body	body	roleRequest	true	"role"
//	@Success	204
//	@Router		/api/books/{bookID}/members/{userID} [patch]
func (h *handlers) updateMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathID(r, "userID")
	if !ok {
		badParam(w, "userID", "invalid")
		return
	}
	var req roleRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.UpdateMemberRole(r.Context(), access(r), userID, req.Role); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// removeMember
//
//	@Summary	Remove a member, or leave the book yourself
//	@Tags		members
//	@Param		bookID	path	int	true	"book id"
//	@Param		userID	path	int	true	"user id"
//	@Success	204
//	@Router		/api/books/{bookID}/members/{userID} [delete]
func (h *handlers) removeMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathID(r, "userID")
	if !ok {
		badParam(w, "userID", "invalid")
		return
	}
	if err := h.svc.RemoveMember(r.Context(), access(r), userID); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

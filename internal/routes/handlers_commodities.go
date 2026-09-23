package routes

import (
	"net/http"
	"strconv"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

// commodities
//
//	@Summary	Everything an account can hold: currencies, securities, points
//	@Tags		reference
//	@Produce	json
//	@Success	200	{array}	CommodityDTO
//	@Router		/api/commodities [get]
func (h *handlers) commodities(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.Commodities(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]CommodityDTO, len(cs))
	for i, c := range cs {
		out[i] = commodityDTO(c)
	}
	response.JSON(w, http.StatusOK, out)
}

type createCommodityRequest struct {
	Code          string           `json:"code"`
	Kind          db.CommodityKind `json:"kind"`
	Name          string           `json:"name"`
	Decimals      *int16           `json:"decimals"`
	QuoteCurrency *string          `json:"quote_currency"`
	ExchangeMIC   *string          `json:"exchange_mic"`
	ContractSize  *decimal.Decimal `json:"contract_size" swaggertype:"string"`
}

// createCommodity
//
//	@Summary	Add a security or a points programme (editor of this book)
//	@Description	Commodities are global like rates; the book only authorises the caller.
//	@Tags		reference
//	@Accept		json
//	@Produce	json
//	@Param		bookID	path		int						true	"book id"
//	@Param		body	body		createCommodityRequest	true	"commodity"
//	@Success	201		{object}	CommodityDTO
//	@Failure	422		{object}	response.ErrorBody
//	@Router		/api/books/{bookID}/commodities [post]
func (h *handlers) createCommodity(w http.ResponseWriter, r *http.Request) {
	var req createCommodityRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	c, err := h.svc.CreateCommodity(r.Context(), access(r), ledger.CommodityInput(req))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, commodityDTO(c))
}

// costBasis
//
//	@Summary	What an account holds and what it cost (average cost input)
//	@Tags		accounts
//	@Produce	json
//	@Param		bookID		path		int		true	"book id"
//	@Param		accountID	path		int		true	"account id"
//	@Param		as_of		query		string	false	"YYYY-MM-DD, default today"
//	@Param		exclude		query		int		false	"transaction id to leave out (the one being edited)"
//	@Success	200			{object}	CostBasisDTO
//	@Router		/api/books/{bookID}/accounts/{accountID}/cost [get]
func (h *handlers) costBasis(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "accountID")
	if !ok {
		badParam(w, "accountID", "invalid")
		return
	}
	asOf, ok := queryDate(r, "as_of")
	if !ok {
		badParam(w, "as_of", "invalid")
		return
	}
	if asOf == nil {
		t := todayUTC()
		asOf = &t
	}
	var exclude int64
	if v := r.URL.Query().Get("exclude"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			badParam(w, "exclude", "invalid")
			return
		}
		exclude = n
	}
	cb, err := h.svc.AccountCostBasis(r.Context(), access(r), id, *asOf, exclude)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, CostBasisDTO{Quantity: cb.Quantity, Cost: cb.Cost, UnitCost: cb.UnitCost()})
}

type identityRequest struct {
	Username        string `json:"username"`
	Email           string `json:"email"`
	CurrentPassword string `json:"current_password"`
}

// updateIdentity
//
//	@Summary	Change username and email (needs the current password)
//	@Tags		me
//	@Accept		json
//	@Produce	json
//	@Param		body	body		identityRequest	true	"identity"
//	@Success	200		{object}	UserDTO
//	@Failure	409		{object}	response.ErrorBody	"username or email taken"
//	@Failure	422		{object}	response.ErrorBody
//	@Router		/api/me/account [patch]
func (h *handlers) updateIdentity(w http.ResponseWriter, r *http.Request) {
	var req identityRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	u, err := h.svc.UpdateIdentity(r.Context(), id.User, req.CurrentPassword, req.Username, req.Email)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, userDTO(u))
}

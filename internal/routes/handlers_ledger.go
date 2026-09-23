package routes

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

// ---- accounts ----

// listAccounts
//
//	@Summary	The book's chart of accounts, archived ones included
//	@Tags		accounts
//	@Produce	json
//	@Param		bookID	path	int	true	"book id"
//	@Success	200		{array}	AccountDTO
//	@Router		/api/books/{bookID}/accounts [get]
func (h *handlers) listAccounts(w http.ResponseWriter, r *http.Request) {
	as, err := h.svc.ListAccounts(r.Context(), access(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]AccountDTO, len(as))
	for i, a := range as {
		out[i] = accountDTO(a)
	}
	response.JSON(w, http.StatusOK, out)
}

type openingDTO struct {
	Amount     decimal.Decimal  `json:"amount" swaggertype:"string"`
	BaseAmount *decimal.Decimal `json:"base_amount,omitempty" swaggertype:"string"`
	Date       Date             `json:"date" swaggertype:"string" format:"date"`
}

type createAccountRequest struct {
	ParentID       *int64          `json:"parent_id"`
	Class          db.AccountClass `json:"class"`
	Name           *string         `json:"name"`
	Code           *string         `json:"code"`
	Commodity      *string         `json:"commodity"`
	IsCurrent      bool            `json:"is_current"`
	IsCash         bool            `json:"is_cash"`
	CfClass        db.CfClass      `json:"cf_class"`
	IsPlaceholder  bool            `json:"is_placeholder"`
	OpeningBalance *openingDTO     `json:"opening_balance"`
}

// createAccount
//
//	@Summary	Create an account, optionally with an opening balance
//	@Tags		accounts
//	@Accept		json
//	@Produce	json
//	@Param		bookID	path		int						true	"book id"
//	@Param		body	body		createAccountRequest	true	"account"
//	@Success	201		{object}	AccountDTO
//	@Failure	422		{object}	response.ErrorBody
//	@Router		/api/books/{bookID}/accounts [post]
func (h *handlers) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in := ledger.AccountInput{
		ParentID: req.ParentID, Class: req.Class, Name: req.Name, Code: req.Code, Commodity: req.Commodity,
		IsCurrent: req.IsCurrent, IsCash: req.IsCash, CfClass: req.CfClass, IsPlaceholder: req.IsPlaceholder,
	}
	if o := req.OpeningBalance; o != nil {
		in.Opening = &ledger.Opening{Amount: o.Amount, BaseAmount: o.BaseAmount, Date: o.Date.Time}
	}
	a, err := h.svc.CreateAccount(r.Context(), access(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, accountDTO(a))
}

type updateAccountRequest struct {
	ParentID      *int64     `json:"parent_id"`
	Name          *string    `json:"name"`
	Code          *string    `json:"code"`
	IsCurrent     bool       `json:"is_current"`
	IsCash        bool       `json:"is_cash"`
	CfClass       db.CfClass `json:"cf_class"`
	IsPlaceholder bool       `json:"is_placeholder"`
}

// updateAccount
//
//	@Summary	Replace an account's editable fields (class and commodity are fixed)
//	@Tags		accounts
//	@Accept		json
//	@Produce	json
//	@Param		bookID		path		int						true	"book id"
//	@Param		accountID	path		int						true	"account id"
//	@Param		body		body		updateAccountRequest	true	"account"
//	@Success	200			{object}	AccountDTO
//	@Router		/api/books/{bookID}/accounts/{accountID} [patch]
func (h *handlers) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "accountID")
	if !ok {
		badParam(w, "accountID", "invalid")
		return
	}
	var req updateAccountRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	a, err := h.svc.UpdateAccount(r.Context(), access(r), id, ledger.AccountUpdate(req))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, accountDTO(a))
}

type archiveRequest struct {
	Archived bool `json:"archived"`
}

// archiveAccount
//
//	@Summary	Archive or restore an account
//	@Tags		accounts
//	@Accept		json
//	@Produce	json
//	@Param		bookID		path		int				true	"book id"
//	@Param		accountID	path		int				true	"account id"
//	@Param		body		body		archiveRequest	true	"archived flag"
//	@Success	200			{object}	AccountDTO
//	@Router		/api/books/{bookID}/accounts/{accountID}/archive [post]
func (h *handlers) archiveAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "accountID")
	if !ok {
		badParam(w, "accountID", "invalid")
		return
	}
	var req archiveRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	a, err := h.svc.SetAccountArchived(r.Context(), access(r), id, req.Archived)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, accountDTO(a))
}

// deleteAccount
//
//	@Summary	Delete an account that never held a posting
//	@Tags		accounts
//	@Param		bookID		path	int	true	"book id"
//	@Param		accountID	path	int	true	"account id"
//	@Success	204
//	@Failure	409	{object}	response.ErrorBody
//	@Router		/api/books/{bookID}/accounts/{accountID} [delete]
func (h *handlers) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "accountID")
	if !ok {
		badParam(w, "accountID", "invalid")
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), access(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// ---- transactions ----

// todayUTC is the default "as of" date for reports and rates.
func todayUTC() time.Time { return time.Now().UTC().Truncate(24 * time.Hour) }

func queryDate(r *http.Request, name string) (*time.Time, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, true
	}
	t, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return nil, false
	}
	return &t, true
}

// listTransactions
//
//	@Summary	A page of transactions, newest first
//	@Tags		transactions
//	@Produce	json
//	@Param		bookID	path		int		true	"book id"
//	@Param		from	query		string	false	"YYYY-MM-DD, inclusive"
//	@Param		to		query		string	false	"YYYY-MM-DD, inclusive"
//	@Param		account	query		int		false	"account id; includes its descendants"
//	@Param		q		query		string	false	"matches payee or memo"
//	@Param		tag		query		string	false	"tag name"
//	@Param		cursor	query		string	false	"next_cursor from the previous page"
//	@Param		limit	query		int		false	"page size, max 200"
//	@Success	200		{object}	TransactionPageDTO
//	@Router		/api/books/{bookID}/transactions [get]
func (h *handlers) listTransactions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := ledger.ListFilter{Query: q.Get("q"), Tag: q.Get("tag"), Cursor: q.Get("cursor")}
	var ok bool
	if f.From, ok = queryDate(r, "from"); !ok {
		badParam(w, "from", "invalid")
		return
	}
	if f.To, ok = queryDate(r, "to"); !ok {
		badParam(w, "to", "invalid")
		return
	}
	if v := q.Get("account"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			badParam(w, "account", "invalid")
			return
		}
		f.AccountID = &id
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			badParam(w, "limit", "invalid")
			return
		}
		f.Limit = n
	}
	views, next, err := h.svc.ListTransactions(r.Context(), access(r), f)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := TransactionPageDTO{Transactions: make([]TransactionDTO, len(views)), NextCursor: next}
	for i, v := range views {
		out.Transactions[i] = transactionDTO(v)
	}
	response.JSON(w, http.StatusOK, out)
}

// createTransaction
//
//	@Summary	Record a transaction
//	@Description	Lines are signed (debit > 0). A foreign-currency line may carry base_amount; otherwise it is converted at the latest rate on or before the date. A residue of one minor unit goes to FX gains/losses.
//	@Tags		transactions
//	@Accept		json
//	@Produce	json
//	@Param		bookID	path		int					true	"book id"
//	@Param		body	body		TransactionInputDTO	true	"transaction"
//	@Success	201		{object}	TransactionDTO
//	@Failure	422		{object}	response.ErrorBody
//	@Router		/api/books/{bookID}/transactions [post]
func (h *handlers) createTransaction(w http.ResponseWriter, r *http.Request) {
	var req TransactionInputDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	v, err := h.svc.CreateTransaction(r.Context(), access(r), req.toLedger())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, transactionDTO(v))
}

// getTransaction
//
//	@Summary	One transaction
//	@Tags		transactions
//	@Produce	json
//	@Param		bookID			path		int	true	"book id"
//	@Param		transactionID	path		int	true	"transaction id"
//	@Success	200				{object}	TransactionDTO
//	@Router		/api/books/{bookID}/transactions/{transactionID} [get]
func (h *handlers) getTransaction(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "transactionID")
	if !ok {
		badParam(w, "transactionID", "invalid")
		return
	}
	v, err := h.svc.GetTransaction(r.Context(), access(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, transactionDTO(v))
}

// updateTransaction
//
//	@Summary	Replace a transaction's header, lines and tags
//	@Tags		transactions
//	@Accept		json
//	@Produce	json
//	@Param		bookID			path		int					true	"book id"
//	@Param		transactionID	path		int					true	"transaction id"
//	@Param		body			body		TransactionInputDTO	true	"transaction"
//	@Success	200				{object}	TransactionDTO
//	@Router		/api/books/{bookID}/transactions/{transactionID} [put]
func (h *handlers) updateTransaction(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "transactionID")
	if !ok {
		badParam(w, "transactionID", "invalid")
		return
	}
	var req TransactionInputDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	v, err := h.svc.UpdateTransaction(r.Context(), access(r), id, req.toLedger())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, transactionDTO(v))
}

// deleteTransaction
//
//	@Summary	Delete a transaction
//	@Tags		transactions
//	@Param		bookID			path	int	true	"book id"
//	@Param		transactionID	path	int	true	"transaction id"
//	@Success	204
//	@Router		/api/books/{bookID}/transactions/{transactionID} [delete]
func (h *handlers) deleteTransaction(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "transactionID")
	if !ok {
		badParam(w, "transactionID", "invalid")
		return
	}
	if err := h.svc.DeleteTransaction(r.Context(), access(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// listTags
//
//	@Summary	Tag names used in the book
//	@Tags		transactions
//	@Produce	json
//	@Param		bookID	path	int	true	"book id"
//	@Success	200		{array}	string
//	@Router		/api/books/{bookID}/tags [get]
func (h *handlers) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.svc.ListTags(r.Context(), access(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.Name
	}
	response.JSON(w, http.StatusOK, out)
}

// ---- balances ----

// balances
//
//	@Summary	Every account's balance as of a date (debit > 0), rolled up the tree
//	@Tags		reports
//	@Produce	json
//	@Param		bookID	path		int		true	"book id"
//	@Param		as_of	query		string	false	"YYYY-MM-DD, default today"
//	@Success	200		{object}	BalancesDTO
//	@Router		/api/books/{bookID}/balances [get]
func (h *handlers) balances(w http.ResponseWriter, r *http.Request) {
	asOf, ok := queryDate(r, "as_of")
	if !ok {
		badParam(w, "as_of", "invalid")
		return
	}
	if asOf == nil {
		today := todayUTC()
		asOf = &today
	}
	b, err := h.svc.Balances(r.Context(), access(r), *asOf)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, balancesDTO(b))
}

// ---- prices ----

// listPrices
//
//	@Summary	Recent exchange rates
//	@Tags		prices
//	@Produce	json
//	@Param		bookID		path	int		true	"book id"
//	@Param		commodity	query	string	false	"currency on either side"
//	@Param		limit		query	int		false	"max 500"
//	@Success	200			{array}	PriceDTO
//	@Router		/api/books/{bookID}/prices [get]
func (h *handlers) listPrices(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ps, err := h.svc.ListPrices(r.Context(), r.URL.Query().Get("commodity"), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]PriceDTO, len(ps))
	for i, p := range ps {
		out[i] = priceDTO(p)
	}
	response.JSON(w, http.StatusOK, out)
}

type priceRequest struct {
	Commodity string          `json:"commodity"`
	Quote     string          `json:"quote"`
	Date      Date            `json:"date" swaggertype:"string" format:"date"`
	Rate      decimal.Decimal `json:"rate" swaggertype:"string"`
}

// addPrice
//
//	@Summary	Record a manual exchange rate: 1 commodity = rate quote
//	@Tags		prices
//	@Accept		json
//	@Produce	json
//	@Param		bookID	path		int				true	"book id"
//	@Param		body	body		priceRequest	true	"rate"
//	@Success	201		{object}	PriceDTO
//	@Router		/api/books/{bookID}/prices [post]
func (h *handlers) addPrice(w http.ResponseWriter, r *http.Request) {
	var req priceRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	p, err := h.svc.AddPrice(r.Context(), access(r), ledger.PriceInput{
		Commodity: req.Commodity, Quote: req.Quote, Date: req.Date.Time, Rate: req.Rate,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, priceDTO(p))
}

// deletePrice
//
//	@Summary	Delete a manual rate
//	@Tags		prices
//	@Param		bookID	path	int	true	"book id"
//	@Param		priceID	path	int	true	"price id"
//	@Success	204
//	@Router		/api/books/{bookID}/prices/{priceID} [delete]
func (h *handlers) deletePrice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "priceID")
	if !ok {
		badParam(w, "priceID", "invalid")
		return
	}
	if err := h.svc.DeletePrice(r.Context(), access(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// rate
//
//	@Summary	The rate the entry form should pre-fill
//	@Tags		prices
//	@Produce	json
//	@Param		bookID	path		int		true	"book id"
//	@Param		from	query		string	true	"currency"
//	@Param		to		query		string	true	"currency"
//	@Param		date	query		string	false	"YYYY-MM-DD, default today"
//	@Success	200		{object}	RateDTO	"rate is null when unknown"
//	@Router		/api/books/{bookID}/rate [get]
func (h *handlers) rate(w http.ResponseWriter, r *http.Request) {
	from := strings.ToUpper(r.URL.Query().Get("from"))
	to := strings.ToUpper(r.URL.Query().Get("to"))
	if from == "" || to == "" {
		badParam(w, "from", "required")
		return
	}
	on, ok := queryDate(r, "date")
	if !ok {
		badParam(w, "date", "invalid")
		return
	}
	if on == nil {
		today := todayUTC()
		on = &today
	}
	rate, found, err := h.svc.Rate(r.Context(), from, to, *on)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := RateDTO{From: from, To: to, Date: Date{*on}}
	if found {
		out.Rate = &rate
	}
	response.JSON(w, http.StatusOK, out)
}

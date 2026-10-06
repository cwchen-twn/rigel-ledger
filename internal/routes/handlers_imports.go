package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

// An import batch may carry a year of a busy card: 5,000 rows with their
// raw source records.
const maxImportBytes = 16 << 20

var (
	importPath  = regexp.MustCompile(`^/api/books/\d+/imports/?$`)
	sourcesPath = regexp.MustCompile(`^/api/books/\d+/imports/sources/?$`)
)

// tokenAllowed is everything an API token (a sync runner) may do.
func tokenAllowed(method, path string) bool {
	switch {
	case method == http.MethodGet && (path == "/api/me" || path == "/api/books"):
		return true
	case method == http.MethodPost && importPath.MatchString(path):
		return true
	case method == http.MethodGet && sourcesPath.MatchString(path):
		return true
	}
	return false
}

type CreateTokenDTO struct {
	Label string `json:"label"`
	// Days until it expires; default 365, at most 730.
	Days int `json:"days"`
}

type TokenCreatedDTO struct {
	// Shown this once; only its hash is stored.
	Token   string     `json:"token"`
	Session SessionDTO `json:"session"`
}

// createToken
//
//	@Summary	Make an API token for a sync runner or a script
//	@Description	The token may only send import batches and read /api/me and /api/books.
//	@Tags		me
//	@Accept		json
//	@Produce	json
//	@Param		body	body		CreateTokenDTO	true	"label and lifetime"
//	@Success	201		{object}	TokenCreatedDTO
//	@Router		/api/me/tokens [post]
func (h *handlers) createToken(w http.ResponseWriter, r *http.Request) {
	var req CreateTokenDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	days, err := tokenDays(req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	token, s, err := h.auth.CreateToken(r.Context(), id.User, req.Label, time.Duration(days)*24*time.Hour, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, TokenCreatedDTO{Token: token, Session: sessionDTO(s)})
}

// tokenDays validates a token request: a label, and 1-730 days (default 365).
func tokenDays(req CreateTokenDTO) (int, error) {
	if req.Label == "" || len(req.Label) > 80 {
		return 0, ledger.FieldError("label", "required", "a token needs a label")
	}
	if req.Days < 0 || req.Days > 730 {
		return 0, ledger.FieldError("days", "out_of_range", "1 to 730 days")
	}
	if req.Days == 0 {
		return 365, nil
	}
	return req.Days, nil
}

func sessionDTO(s db.Session) SessionDTO {
	return SessionDTO{ID: s.ID, Kind: s.Kind, Label: s.Label, UserAgent: s.UserAgent,
		CreatedAt: timestamp(s.CreatedAt), LastUsedAt: timestamp(s.LastUsedAt), ExpiresAt: timestamp(s.ExpiresAt)}
}

type ImportAccountDTO struct {
	// The source's own id for the account (masked account number, card id).
	ID       string `json:"id"`
	Label    string `json:"label"`
	Currency string `json:"currency"`
	// brokerage: holds securities (holding and trade rows) and settles
	// through a cash account; cash (the default) is everything else.
	Kind string `json:"kind,omitempty" enums:"cash,brokerage"`
}

type ImportRowInDTO struct {
	Kind    string `json:"kind" enums:"transaction,balance,holding,trade,invoice"`
	Account string `json:"account"`
	// The source's id for the row, stable across resends.
	ID           string          `json:"id"`
	Date         Date            `json:"date" swaggertype:"string"`
	Amount       decimal.Decimal `json:"amount" swaggertype:"string"`
	Currency     string          `json:"currency"`
	Description  string          `json:"description"`
	Counterparty string          `json:"counterparty"`
	Pending      bool            `json:"pending"`
	// The source record as it came, for the audit; never credentials.
	Raw json.RawMessage `json:"raw,omitempty" swaggertype:"object"`
	// The ref of the batch's file that is this row's evidence (an order
	// email, an e-invoice, a statement); attached when the row is accepted.
	File string `json:"file,omitempty"`

	// holding and trade rows: the security as NAMESPACE:SYMBOL (XTAI:2330),
	// registered with this name and quote currency the first time.
	Security      string `json:"security,omitempty"`
	SecurityName  string `json:"security_name,omitempty"`
	QuoteCurrency string `json:"quote_currency,omitempty"`
	// Units held (holding) or moved: > 0 in, < 0 out (trade).
	Units *decimal.Decimal `json:"units,omitempty" swaggertype:"string"`
	// Per unit, in the quote currency, when the source knows it.
	Price *decimal.Decimal `json:"price,omitempty" swaggertype:"string"`
	// What the trade settled for, signed on the settlement account (a buy
	// < 0), when the source knows it; otherwise confirmed on accept.
	Cash *decimal.Decimal `json:"cash,omitempty" swaggertype:"string"`
	// invoice rows: its lines. The row's amount is the invoice's total,
	// signed on the account that paid (a purchase < 0); counterparty is
	// the seller.
	Items []InvoiceItemDTO `json:"items,omitempty"`
}

// InvoiceItemDTO is one line of an invoice: what it cost (> 0; a discount
// < 0), and its quantity and unit price when the invoice says.
type InvoiceItemDTO struct {
	Description string           `json:"description"`
	Quantity    *decimal.Decimal `json:"quantity,omitempty" swaggertype:"string"`
	UnitPrice   *decimal.Decimal `json:"unit_price,omitempty" swaggertype:"string"`
	Amount      decimal.Decimal  `json:"amount" swaggertype:"string"`
	// In the queue: the category a rule gives the line.
	AccountID *int64 `json:"account_id,omitempty"`
}

// ImportFileDTO is evidence a batch carries: an image or a PDF, up to 10 MiB,
// counted in the batch's 16 MiB.
type ImportFileDTO struct {
	Ref      string `json:"ref"`
	Filename string `json:"filename"`
	Data     []byte `json:"data" swaggertype:"string" format:"base64"`
}

type ImportBatchDTO struct {
	Connector string             `json:"connector"`
	Label     string             `json:"label"`
	Accounts  []ImportAccountDTO `json:"accounts"`
	Rows      []ImportRowInDTO   `json:"rows"`
	Files     []ImportFileDTO    `json:"files,omitempty"`
}

type ImportResultDTO struct {
	BatchID    int64 `json:"batch_id"`
	Received   int   `json:"received"`
	Staged     int   `json:"staged"`
	Duplicates int   `json:"duplicates"`
	Balances   int   `json:"balances"`
}

// importBatch
//
//	@Summary	Stage a batch of rows from a source for review
//	@Description	Amounts are signed on the account: money in (or a card payment) > 0. A row already staged (same connector and id) is counted as a duplicate.
//	@Tags		imports
//	@Accept		json
//	@Produce	json
//	@Param		bookID	path		int				true	"book id"
//	@Param		body	body		ImportBatchDTO	true	"batch"
//	@Success	201		{object}	ImportResultDTO
//	@Failure	422		{object}	response.ErrorBody
//	@Router		/api/books/{bookID}/imports [post]
func (h *handlers) importBatch(w http.ResponseWriter, r *http.Request) {
	var req ImportBatchDTO
	if err := response.DecodeLimit(w, r, &req, maxImportBytes); err != nil {
		h.fail(w, r, err)
		return
	}
	in := importInput(req)
	res, err := h.svc.Import(r.Context(), access(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, importResultDTO(res))
}

func importInput(req ImportBatchDTO) ledger.ImportInput {
	in := ledger.ImportInput{Connector: req.Connector, Label: req.Label}
	for _, a := range req.Accounts {
		in.Accounts = append(in.Accounts, ledger.ImportAccount{ExternalID: a.ID, Label: a.Label, Currency: a.Currency, Kind: a.Kind})
	}
	for _, row := range req.Rows {
		in.Rows = append(in.Rows, ledger.ImportRowInput{Kind: row.Kind, Account: row.Account, ID: row.ID, Date: row.Date.Time,
			Amount: row.Amount, Currency: row.Currency, Description: row.Description, Counterparty: row.Counterparty,
			Pending: row.Pending, Raw: row.Raw, File: row.File,
			Security: row.Security, SecurityName: row.SecurityName, QuoteCurrency: row.QuoteCurrency,
			Units: row.Units, Price: row.Price, Cash: row.Cash, Items: invoiceItems(row.Items)})
	}
	for _, f := range req.Files {
		in.Files = append(in.Files, ledger.ImportFile{Ref: f.Ref, FileInput: ledger.FileInput{Filename: f.Filename, Bytes: f.Data}})
	}
	return in
}

func importResultDTO(res ledger.ImportResult) ImportResultDTO {
	return ImportResultDTO{BatchID: res.BatchID, Received: res.Received, Staged: res.Staged, Duplicates: res.Duplicates, Balances: res.Balances}
}

type SourceAccountDTO struct {
	ID         int64   `json:"id"`
	Connector  string  `json:"connector"`
	ExternalID string  `json:"external_id"`
	Label      string  `json:"label"`
	Currency   *string `json:"currency"`
	Kind       string  `json:"kind" enums:"cash,brokerage"`
	// For a brokerage, the parent its securities' accounts are made under.
	AccountID *int64 `json:"account_id"`
	// For a brokerage, where its trades' cash goes.
	SettlementAccountID *int64 `json:"settlement_account_id"`
	Pending             int64  `json:"pending"`
}

// listSources
//
//	@Summary	The accounts sources have sent, and what each is mapped to
//	@Tags		imports
//	@Produce	json
//	@Param		bookID	path	int	true	"book id"
//	@Success	200		{array}	SourceAccountDTO
//	@Router		/api/books/{bookID}/imports/sources [get]
func (h *handlers) listSources(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.SourceAccounts(r.Context(), access(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]SourceAccountDTO, len(rows))
	for i, s := range rows {
		out[i] = SourceAccountDTO{ID: s.ID, Connector: s.Connector, ExternalID: s.ExternalID, Label: s.Label,
			Currency: s.Currency, Kind: s.Kind, AccountID: s.AccountID, SettlementAccountID: s.SettlementAccountID, Pending: s.Pending}
	}
	response.JSON(w, http.StatusOK, out)
}

// MapSourceDTO replaces the mapping; null unmaps.
type MapSourceDTO struct {
	// The account rows book to; for a brokerage, the parent of its securities.
	AccountID *int64 `json:"account_id"`
	// A brokerage only: the account its trades settle through.
	SettlementAccountID *int64 `json:"settlement_account_id"`
}

// mapSource
//
//	@Summary	Map a source account to one of the book's accounts
//	@Description	Its waiting rows are matched again at once.
//	@Tags		imports
//	@Accept		json
//	@Param		bookID		path	int				true	"book id"
//	@Param		sourceID	path	int				true	"source account id"
//	@Param		body		body	MapSourceDTO	true	"mapping"
//	@Success	204
//	@Router		/api/books/{bookID}/imports/sources/{sourceID} [patch]
func (h *handlers) mapSource(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(r, "sourceID")
	if !ok {
		badParam(w, "sourceID", "invalid")
		return
	}
	var req MapSourceDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.svc.MapSourceAccount(r.Context(), access(r), sid, ledger.SourceMapping{AccountID: req.AccountID, SettlementAccountID: req.SettlementAccountID}); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

type ImportRowDTO struct {
	ID                 int64           `json:"id"`
	Kind               string          `json:"kind"`
	ExternalID         string          `json:"external_id"`
	Date               Date            `json:"date" swaggertype:"string"`
	Amount             decimal.Decimal `json:"amount" swaggertype:"string"`
	Currency           string          `json:"currency"`
	Description        string          `json:"description"`
	Counterparty       string          `json:"counterparty"`
	Pending            bool            `json:"pending"`
	Proposal           string          `json:"proposal" enums:"new,duplicate,clears,transfer"`
	ProposedAccountID  *int64          `json:"proposed_account_id"`
	MatchTransactionID *int64          `json:"match_transaction_id"`
	MatchRowID         *int64          `json:"match_row_id"`
	RuleID             *int64          `json:"rule_id"`
	SourceAccountID    int64           `json:"source_account_id"`
	SourceLabel        string          `json:"source_label"`
	Connector          string          `json:"connector"`
	// The mapped account; null while the source account is unmapped.
	AccountID *int64 `json:"account_id"`
	// The row's evidence (GET /api/books/{bookID}/attachments/{id}).
	AttachmentID *int64 `json:"attachment_id"`
	// holding and trade rows.
	Security     *string          `json:"security"`
	SecurityName *string          `json:"security_name"`
	Units        *decimal.Decimal `json:"units" swaggertype:"string"`
	Price        *decimal.Decimal `json:"price" swaggertype:"string"`
	Cash         *decimal.Decimal `json:"cash" swaggertype:"string"`
	// The brokerage's settlement account; a trade waits for it.
	SettlementAccountID *int64 `json:"settlement_account_id"`
	// invoice rows: the lines, each with the category a rule gives it.
	Items []InvoiceItemDTO `json:"items"`
}

// importQueue
//
//	@Summary	Rows waiting for review, newest first
//	@Tags		imports
//	@Produce	json
//	@Param		bookID	path	int	true	"book id"
//	@Success	200		{array}	ImportRowDTO
//	@Router		/api/books/{bookID}/imports/queue [get]
func (h *handlers) importQueue(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.Queue(r.Context(), access(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]ImportRowDTO, len(rows))
	for i, q := range rows {
		out[i] = ImportRowDTO{ID: q.ID, Kind: q.Kind, ExternalID: q.ExternalID, Date: Date{q.Date}, Amount: q.Amount,
			Currency: q.Currency, Description: q.Description, Counterparty: q.Counterparty, Pending: q.Pending,
			Proposal: q.Proposal, ProposedAccountID: q.ProposedAccountID, MatchTransactionID: q.MatchTransactionID,
			MatchRowID: q.MatchRowID, RuleID: q.RuleID, SourceAccountID: q.SourceAccountID, SourceLabel: q.SourceLabel,
			Connector: q.Connector, AccountID: q.AccountID, AttachmentID: q.AttachmentID,
			Security: q.Security, SecurityName: q.SecurityName, Units: decPtr(q.Units), Price: decPtr(q.Price), Cash: decPtr(q.Cash),
			SettlementAccountID: q.SettlementAccountID, Items: queueItems(q.Items)}
	}
	response.JSON(w, http.StatusOK, out)
}

type AcceptRowsDTO struct {
	RowIDs []int64 `json:"row_ids"`
	// Book them all against this category instead of the proposals.
	AccountID *int64 `json:"account_id"`
	// A trade's settled cash, by row id, signed on its settlement account
	// (a buy < 0), when the source did not send it.
	Cash map[string]decimal.Decimal `json:"cash,omitempty" swaggertype:"object,string"`
	// Invoices: split the expense by the categories their items' rules give.
	Split bool `json:"split,omitempty"`
}

type AcceptFailureDTO struct {
	RowID int64  `json:"row_id"`
	Code  string `json:"code"`
}

type AcceptResultDTO struct {
	Accepted     []int64            `json:"accepted"`
	Transactions []int64            `json:"transactions"`
	Failed       []AcceptFailureDTO `json:"failed"`
}

// acceptRows
//
//	@Summary	Accept rows as proposed (or against one category)
//	@Description	Each row is its own database transaction: one that fails (say, a new row without a category) does not stop the others.
//	@Tags		imports
//	@Accept		json
//	@Produce	json
//	@Param		bookID	path		int				true	"book id"
//	@Param		body	body		AcceptRowsDTO	true	"rows"
//	@Success	200		{object}	AcceptResultDTO
//	@Router		/api/books/{bookID}/imports/accept [post]
func (h *handlers) acceptRows(w http.ResponseWriter, r *http.Request) {
	var req AcceptRowsDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if len(req.RowIDs) == 0 || len(req.RowIDs) > 500 {
		response.Error(w, http.StatusUnprocessableEntity, "invalid_input", "1 to 500 rows",
			map[string]string{"row_ids": "out_of_range"})
		return
	}
	a := access(r)
	out := AcceptResultDTO{Accepted: []int64{}, Transactions: []int64{}, Failed: []AcceptFailureDTO{}}
	for _, id := range req.RowIDs {
		in := ledger.AcceptInput{CategoryID: req.AccountID, Split: req.Split}
		if c, ok := req.Cash[strconv.FormatInt(id, 10)]; ok {
			in.Cash = &c
		}
		tx, err := h.svc.Accept(r.Context(), a, id, in)
		if err != nil {
			var le *ledger.Error
			if !errors.As(err, &le) || le.Kind == ledger.KindForbidden {
				h.fail(w, r, err)
				return
			}
			code := le.Code
			for _, c := range le.Fields {
				code = c // the field's code says more ("required") than invalid_input
			}
			out.Failed = append(out.Failed, AcceptFailureDTO{RowID: id, Code: code})
			continue
		}
		out.Accepted = append(out.Accepted, id)
		out.Transactions = append(out.Transactions, tx)
	}
	response.JSON(w, http.StatusOK, out)
}

type IgnoreRowsDTO struct {
	RowIDs []int64 `json:"row_ids"`
}

// ignoreRows
//
//	@Summary	Drop rows from the queue
//	@Description	They stay on record, so the same source rows are never staged again.
//	@Tags		imports
//	@Accept		json
//	@Param		bookID	path	int				true	"book id"
//	@Param		body	body	IgnoreRowsDTO	true	"rows"
//	@Success	204
//	@Router		/api/books/{bookID}/imports/ignore [post]
func (h *handlers) ignoreRows(w http.ResponseWriter, r *http.Request) {
	var req IgnoreRowsDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	a := access(r)
	for _, id := range req.RowIDs {
		if err := h.svc.IgnoreRow(r.Context(), a, id); err != nil {
			var le *ledger.Error
			if errors.As(err, &le) && le.Code == "already_decided" {
				continue
			}
			h.fail(w, r, err)
			return
		}
	}
	response.NoContent(w)
}

type ImportRuleDTO struct {
	ID              int64  `json:"id"`
	Pattern         string `json:"pattern"`
	SourceAccountID *int64 `json:"source_account_id"`
	AccountID       int64  `json:"account_id"`
}

type ImportRuleInputDTO struct {
	// Matched case-insensitively against the description and counterparty.
	Pattern string `json:"pattern"`
	// Only rows from this source account; null for all.
	SourceAccountID *int64 `json:"source_account_id"`
	AccountID       int64  `json:"account_id"`
}

// listRules
//
//	@Summary	Categorisation rules
//	@Tags		imports
//	@Produce	json
//	@Param		bookID	path	int	true	"book id"
//	@Success	200		{array}	ImportRuleDTO
//	@Router		/api/books/{bookID}/imports/rules [get]
func (h *handlers) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.svc.Rules(r.Context(), access(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]ImportRuleDTO, len(rules))
	for i, x := range rules {
		out[i] = ImportRuleDTO{ID: x.ID, Pattern: x.Pattern, SourceAccountID: x.SourceAccountID, AccountID: x.AccountID}
	}
	response.JSON(w, http.StatusOK, out)
}

// createRule
//
//	@Summary	Always categorise matching rows to an account
//	@Description	Waiting rows without a category are matched again at once.
//	@Tags		imports
//	@Accept		json
//	@Produce	json
//	@Param		bookID	path		int					true	"book id"
//	@Param		body	body		ImportRuleInputDTO	true	"rule"
//	@Success	201		{object}	ImportRuleDTO
//	@Router		/api/books/{bookID}/imports/rules [post]
func (h *handlers) createRule(w http.ResponseWriter, r *http.Request) {
	var req ImportRuleInputDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	x, err := h.svc.CreateRule(r.Context(), access(r), req.Pattern, req.AccountID, req.SourceAccountID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, ImportRuleDTO{ID: x.ID, Pattern: x.Pattern, SourceAccountID: x.SourceAccountID, AccountID: x.AccountID})
}

// deleteRule
//
//	@Summary	Delete a categorisation rule
//	@Tags		imports
//	@Param		bookID	path	int	true	"book id"
//	@Param		ruleID	path	int	true	"rule id"
//	@Success	204
//	@Router		/api/books/{bookID}/imports/rules/{ruleID} [delete]
func (h *handlers) deleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "ruleID")
	if !ok {
		badParam(w, "ruleID", "invalid")
		return
	}
	if err := h.svc.DeleteRule(r.Context(), access(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

type DriftDTO struct {
	AccountID int64           `json:"account_id"`
	Date      Date            `json:"date" swaggertype:"string"`
	Asserted  decimal.Decimal `json:"asserted" swaggertype:"string"`
	Booked    decimal.Decimal `json:"booked" swaggertype:"string"`
	Source    string          `json:"source"`
}

// drift
//
//	@Summary	Accounts whose institution balance differs from the books
//	@Description	The newest balance each source reported, against the books on that date, in the account's commodity.
//	@Tags		imports
//	@Produce	json
//	@Param		bookID	path	int	true	"book id"
//	@Success	200		{array}	DriftDTO
//	@Router		/api/books/{bookID}/drift [get]
func (h *handlers) drift(w http.ResponseWriter, r *http.Request) {
	ds, err := h.svc.Drifts(r.Context(), access(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]DriftDTO, len(ds))
	for i, x := range ds {
		out[i] = DriftDTO{AccountID: x.AccountID, Date: Date{x.Date}, Asserted: x.Asserted, Booked: x.Booked, Source: x.Source}
	}
	response.JSON(w, http.StatusOK, out)
}

func decPtr(d decimal.NullDecimal) *decimal.Decimal {
	if !d.Valid {
		return nil
	}
	return &d.Decimal
}

func invoiceItems(in []InvoiceItemDTO) []ledger.InvoiceItem {
	var out []ledger.InvoiceItem
	for _, it := range in {
		out = append(out, ledger.InvoiceItem{Description: it.Description, Quantity: it.Quantity, UnitPrice: it.UnitPrice, Amount: it.Amount})
	}
	return out
}

// queueItems reads a row's stored lines (the ledger's InvoiceItem JSON).
func queueItems(raw []byte) []InvoiceItemDTO {
	if len(raw) == 0 {
		return nil
	}
	var items []InvoiceItemDTO
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	return items
}

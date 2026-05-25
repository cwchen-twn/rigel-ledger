package models

import (
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/shopspring/decimal"
)

type Journal struct {
	JournalID     int             `json:"journalId" db:"journal_id"`
	UserID        string          `json:"userId" db:"user_id"`
	TransacID     int             `json:"transacId" db:"transac_id"`
	TransacDate   time.Time       `json:"transacDate" db:"transac_date"`
	PostingDate   time.Time       `json:"postingDate" db:"posting_date"`
	PhotoAddr     string          `json:"photoAddr" db:"photo_addr"`
	Description   string          `json:"description" db:"description"`
	ReferenceNo   string          `json:"referenceNo" db:"reference_no"`
	IsReconciled  bool            `json:"isReconciled" db:"is_reconciled"`
	ExchangeRate  decimal.Decimal `json:"exchangeRate" db:"exchange_rate"`
	StockPriceRef int             `json:"stockPriceRef" db:"stock_price_ref"`
	CreatedAt     time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time       `json:"updatedAt" db:"updated_at"`
	Postings      []Posting       `json:"postings" db:"postings"`
}

type Posting struct {
	PostingID    int             `json:"postingId" db:"posting_id"`
	JournalID    int             `json:"journalId" db:"journal_id"`
	LedgerID     int             `json:"ledgerId" db:"ledger_id"`
	PostingType  string          `json:"postingType" db:"posting_type"`
	Amount       decimal.Decimal `json:"amount" db:"amount"`
	CurrencyCode string          `json:"currencyCode" db:"currency_code"`
	Description  string          `json:"description" db:"description"`
	CreatedAt    time.Time       `json:"createdAt" db:"created_at"`
}

type DataTableRequest struct {
	Draw   int
	Start  int
	Length int
	Search struct {
		Value string
		Regex bool
	}
}

type DataTableResponse struct {
	Data     []Journal `json:"data"`
	Draw     int       `json:"draw"`
	Total    int       `json:"total"`
	Filtered int       `json:"filtered"`
}

func FindTransactionsByUserID(userID string, dr DataTableRequest, conn *sqlx.DB) (DataTableResponse, error) {
	query := `
		SELECT
			journal_id,
			user_id,
			transac_id,
			transac_date,
			posting_date,
			photo_addr,
			description,
			reference_no,
			is_reconciled,
			exchange_rate,
			stock_price_ref,
			created_at,
			updated_at
		FROM
			user_ledger_journal
		WHERE
			user_id = $1
		ORDER BY
			transac_date DESC
		LIMIT $2 OFFSET $3
	`

	journals := []Journal{}
	err := conn.Select(&journals, query, userID, dr.Length, dr.Start)
	if err != nil {
		return DataTableResponse{}, err
	}

	// Collect all journal IDs for a single batch postings query (avoid N+1)
	if len(journals) > 0 {
		journalIDs := make([]int, len(journals))
		for i, j := range journals {
			journalIDs[i] = j.JournalID
		}

		postingsQueryBase := `
			SELECT
				posting_id,
				journal_id,
				ledger_id,
				posting_type,
				amount,
				currency_code,
				description,
				created_at
			FROM
				user_ledger_postings
			WHERE
				journal_id IN (?)
			ORDER BY
				created_at DESC
		`
		postingsQuery, args, err2 := sqlx.In(postingsQueryBase, journalIDs)
		if err2 != nil {
			return DataTableResponse{}, err2
		}
		postingsQuery = conn.Rebind(postingsQuery)

		allPostings := []Posting{}
		err = conn.Select(&allPostings, postingsQuery, args...)
		if err != nil {
			return DataTableResponse{}, err
		}

		// Group postings by journal_id in Go
		postingsByJournal := make(map[int][]Posting, len(journals))
		for _, p := range allPostings {
			postingsByJournal[p.JournalID] = append(postingsByJournal[p.JournalID], p)
		}
		for i := range journals {
			journals[i].Postings = postingsByJournal[journals[i].JournalID]
			if journals[i].Postings == nil {
				journals[i].Postings = []Posting{}
			}
		}
	}

	countQuery := `
		SELECT
			COUNT(*)
		FROM
			user_ledger_journal
		WHERE
			user_id = $1
	`

	var total int
	err = conn.Get(&total, countQuery, userID)
	if err != nil {
		return DataTableResponse{}, err
	}

	return DataTableResponse{
		Draw:     dr.Draw,
		Data:     journals,
		Total:    total,
		Filtered: total,
	}, nil
}

func (dr DataTableResponse) ToJSONString() (string, error) {
	json, err := json.Marshal(dr)
	if err != nil {
		return "", err
	}
	return string(json), nil
}

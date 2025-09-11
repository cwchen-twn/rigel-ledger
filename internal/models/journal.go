package models

import (
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
)

type Journal struct {
	JournalID     int       `json:"journalId" db:"journal_id"`
	UserID        int       `json:"userId" db:"user_id"`
	TransacID     int       `json:"transacId" db:"transac_id"`
	TransacDate   time.Time `json:"transacDate" db:"transac_date"`
	PostingDate   time.Time `json:"postingDate" db:"posting_date"`
	PhotoAddr     string    `json:"photoAddr" db:"photo_addr"`
	Description   string    `json:"description" db:"description"`
	ReferenceNo   string    `json:"referenceNo" db:"reference_no"`
	IsReconciled  bool      `json:"isReconciled" db:"is_reconciled"`
	ExchangeRate  float64   `json:"exchangeRate" db:"exchange_rate"`
	StockPriceRef int       `json:"stockPriceRef" db:"stock_price_ref"`
	CreatedAt     time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time `json:"updatedAt" db:"updated_at"`
	Postings      []Posting `json:"postings" db:"postings"`
}

type Posting struct {
	PostingID    int       `json:"postingId" db:"posting_id"`
	JournalID    int       `json:"journalId" db:"journal_id"`
	LedgerID     int       `json:"ledgerId" db:"ledger_id"`
	PostingType  string    `json:"postingType" db:"posting_type"`
	Amount       float64   `json:"amount" db:"amount"`
	CurrencyCode string    `json:"currencyCode" db:"currency_code"`
	Description  string    `json:"description" db:"description"`
	CreatedAt    time.Time `json:"createdAt" db:"created_at"`
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
	for i := range journals {
		journals[i].Postings, err = journals[i].findPostings(conn)
		if err != nil {
			return DataTableResponse{}, err
		}
	}

	query = `
		SELECT
			COUNT(*)
		FROM
			user_ledger_journal
		WHERE
			user_id = $1
	`

	var total int
	err = conn.Get(&total, query, userID)
	if err != nil {
		return DataTableResponse{}, err
	}

	return DataTableResponse{
		Draw:     dr.Draw,
		Data:     journals,
		Total:    total,
		Filtered: len(journals),
	}, nil
}

func (j *Journal) findPostings(conn *sqlx.DB) ([]Posting, error) {
	query := `
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
			journal_id = $1
		ORDER BY
			created_at DESC
	`
	postings := []Posting{}
	err := conn.Select(&postings, query, j.JournalID)
	return postings, err
}

func (dr DataTableResponse) ToJSONString() (string, error) {
	json, err := json.Marshal(dr)
	if err != nil {
		return "", err
	}
	return string(json), nil
}

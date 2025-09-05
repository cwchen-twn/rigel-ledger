package models

import (
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
)

type Journal struct {
	JournalID     int       `db:"journal_id"`
	UserID        int       `db:"user_id"`
	TransacID     int       `db:"transac_id"`
	TransacDate   time.Time `db:"transac_date"`
	PostingDate   time.Time `db:"posting_date"`
	PhotoAddr     string    `db:"photo_addr"`
	Description   string    `db:"description"`
	ReferenceNo   string    `db:"reference_no"`
	IsReconciled  bool      `db:"is_reconciled"`
	ExchangeRate  float64   `db:"exchange_rate"`
	StockPriceRef int       `db:"stock_price_ref"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
	Postings      []Posting `db:"postings"`
}

type Posting struct {
	PostingID    int       `db:"posting_id"`
	JournalID    int       `db:"journal_id"`
	LedgerID     int       `db:"ledger_id"`
	PostingType  string    `db:"posting_type"`
	Amount       float64   `db:"amount"`
	CurrencyCode string    `db:"currency_code"`
	Description  string    `db:"description"`
	CreatedAt    time.Time `db:"created_at"`
}

func FindByUserID(userID string, limit int, conn *sqlx.DB) ([]Journal, error) {
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
		LIMIT $2
	`

	journals := []Journal{}
	err := conn.Select(&journals, query, userID, limit)
	if err != nil {
		return nil, err
	}
	for i := range journals {
		journals[i].Postings, err = journals[i].findPostings(conn)
		if err != nil {
			return nil, err
		}
	}
	return journals, nil
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

type Journals []Journal

func (js Journals) ToJSONString() (string, error) {
	json, err := json.Marshal(js)
	if err != nil {
		return "", err
	}
	return string(json), nil
}

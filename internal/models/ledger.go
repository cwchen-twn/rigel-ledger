package models

import (
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
)

type Ledger struct {
	LedgerID     int        `json:"ledger_id"`
	LedgerOwner  string     `json:"ledger_owner"`
	LedgerName   string     `json:"ledger_name"`
	LedgerTypeID string     `json:"ledger_type_id"`
	LedgerType   LedgerType `json:"ledger_type"`
	Currency     string     `json:"currency"`
	Balance      float64    `json:"balance"`
	LedgerStatus int        `json:"ledger_status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type LedgerType struct {
	LedgerTypeID  string `json:"ledger_type_id"`
	FirstGrade    string `json:"first_grade"`
	SecondGrade   string `json:"second_grade"`
	ThirdGrade    string `json:"third_grade"`
	TypeName      string `json:"type_name"`
	TypeNameZH    string `json:"type_name_zh"`
	DescriptionEN string `json:"description_en"`
	DescriptionZH string `json:"description_zh"`
	IsActive      bool   `json:"is_active"`
}

type Ledgers []Ledger

func FindLedgersByUserID(userID string, conn *sqlx.DB) (Ledgers, error) {
	query := `
		SELECT
			ledger_id,
			ledger_owner,
			ledger_name,
			ledger_type_id,
			currency,
			balance,
			ledger_status,
			created_at,
			updated_at
		FROM
			user_ledgers
		WHERE
			ledger_owner = $1
	`
	ledgers := Ledgers{}
	err := conn.Select(&ledgers, query, userID)
	if err != nil {
		return Ledgers{}, err
	}

	queryLt := `
		SELECT
			ledger_type_id,
			first_grade,
			second_grade,
			third_grade,
			type_name,
			type_name_zh,
			description_en,
			description_zh,
			is_active,
			created_at,
			updated_at
		FROM
			ref_ledger_types
		WHERE
			ledger_type_id = $1
	`
	for i, l := range ledgers {
		lt := LedgerType{}
		err := conn.Get(lt, queryLt, l.LedgerTypeID)
		if err != nil {
			return Ledgers{}, err
		}
		ledgers[i].LedgerType = lt
	}
	return ledgers, nil
}

func (ls *Ledgers) Save(conn *sqlx.DB) error {
	query := `
		INSERT INTO user_ledgers (
			ledger_owner,
			ledger_name,
			ledger_type_id,
			currency,
			balance
		) VALUES (:ledger_owner, :ledger_name, :ledger_type_id, :currency, :balance)
	`
	_, err := conn.NamedExec(query, ls)
	if err != nil {
		return err
	}
	return nil
}

func (ls Ledgers) ToJSONString() (string, error) {
	json, err := json.Marshal(ls)
	if err != nil {
		return "", err
	}
	return string(json), nil
}

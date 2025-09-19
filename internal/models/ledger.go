package models

import (
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
)

type Ledger struct {
	LedgerID     int        `json:"ledgerID" db:"ledger_id"`
	LedgerOwner  string     `json:"ledgerOwner" db:"ledger_owner"`
	LedgerName   string     `json:"ledgerName" db:"ledger_name"`
	LedgerTypeID string     `json:"ledgerTypeID" db:"ledger_type_id"`
	LedgerType   LedgerType `json:"ledgerType" db:"ledger_type"`
	Currency     string     `json:"currency" db:"currency"`
	Balance      float64    `json:"balance" db:"balance"`
	LedgerStatus int        `json:"ledgerStatus" db:"ledger_status"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time  `json:"updatedAt" db:"updated_at"`
}

type LedgerType struct {
	LedgerTypeID  string `json:"ledgerTypeID" db:"ledger_type_id"`
	FirstGrade    string `json:"firstGrade" db:"first_grade"`
	SecondGrade   string `json:"secondGrade" db:"second_grade"`
	ThirdGrade    string `json:"thirdGrade" db:"third_grade"`
	TypeName      string `json:"typeName" db:"type_name"`
	TypeNameZH    string `json:"typeNameZH" db:"type_name_zh"`
	DescriptionEN string `json:"descriptionEN" db:"description_en"`
	DescriptionZH string `json:"descriptionZH" db:"description_zh"`
	IsActive      bool   `json:"isActive" db:"is_active"`
}

type Currency struct {
	AlphabeticCode string `json:"alphabeticCode" db:"alphabetic_code"`
	NumericCode    int    `json:"numericCode" db:"numeric_code"`
	MinorUnit      int    `json:"minorUnit" db:"minor_unit"`
	CurrencyName   string `json:"currencyName" db:"currency_name"`
}

type Ledgers []Ledger

type LedgerTypes []LedgerType

type Currencies []Currency

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
			is_active
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

func FindLedgerTypes(conn *sqlx.DB) (LedgerTypes, error) {
	query := `
		SELECT
			ledger_type_id,
			COALESCE(first_grade, '') first_grade,
			COALESCE(second_grade, '') second_grade,
			COALESCE(third_grade, '') third_grade,
			COALESCE(type_name, '') type_name,
			COALESCE(type_name_zh, '') type_name_zh,
			COALESCE(description_en, '') description_en,
			COALESCE(description_zh, '') description_zh,
			is_active
		FROM
			ref_ledger_types
		WHERE
			is_active = TRUE
	`
	ledgerTypes := LedgerTypes{}
	err := conn.Select(&ledgerTypes, query)
	if err != nil {
		return LedgerTypes{}, err
	}
	return ledgerTypes, nil
}

func FindCurrencies(conn *sqlx.DB) (Currencies, error) {
	query := `
		SELECT
			alphabetic_code,
			numeric_code,
			minor_unit,
			currency_name
		FROM
			ref_currencies_iso4217
	`
	currencies := Currencies{}
	err := conn.Select(&currencies, query)
	if err != nil {
		return Currencies{}, err
	}
	return currencies, nil
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

func (lts LedgerTypes) ToJSONString() (string, error) {
	json, err := json.Marshal(lts)
	if err != nil {
		return "", err
	}
	return string(json), nil
}

func (cs Currencies) ToJSONString() (string, error) {
	json, err := json.Marshal(cs)
	if err != nil {
		return "", err
	}
	return string(json), nil
}

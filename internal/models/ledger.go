package models

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/jmoiron/sqlx"
	"github.com/shopspring/decimal"
)

var (
	ErrLedgerNameRequired   = errors.New("ledger name is required")
	ErrLedgerTypeIDRequired = errors.New("ledger type id is required")
	ErrCurrencyRequired     = errors.New("currency is required")
	ErrBalanceRequired      = errors.New("balance is required")
)

type Ledger struct {
	LedgerID      int             `json:"ledgerID" db:"ledger_id"`
	LedgerOwner   string          `json:"ledgerOwner" db:"ledger_owner"`
	LedgerName    string          `json:"ledgerName" db:"ledger_name"`
	LedgerTypeID  string          `json:"ledgerTypeID" db:"ledger_type_id"`
	LedgerType    LedgerType      `json:"ledgerType" db:"ledger_type"`
	Currency      string          `json:"currency" db:"currency"`
	Balance       decimal.Decimal `json:"balance" db:"balance"`
	LedgerStatus  int             `json:"ledgerStatus" db:"ledger_status"`
	CreatedAt     string          `json:"createdAt" db:"created_at"`
	UpdatedAt     string          `json:"updatedAt" db:"updated_at"`
	PostingsCount int             `json:"postingsCount" db:"postings_count"`
}

type LedgerType struct {
	LedgerTypeID string `json:"ledgerTypeID" db:"ledger_type_id"`
	FirstGrade   string `json:"firstGrade" db:"first_grade"`
	SecondGrade  string `json:"secondGrade" db:"second_grade"`
	ThirdGrade   string `json:"thirdGrade" db:"third_grade"`
	TypeName     string `json:"typeName" db:"type_name"`
	Description  string `json:"description" db:"description"`
	IsActive     bool   `json:"isActive" db:"is_active"`
}

type LedgerTypesFirstGrade struct {
	FirstGrade  string `json:"firstGrade" db:"first_grade"`
	TypeName    string `json:"typeName" db:"type_name"`
	Description string `json:"description" db:"description"`
}

type Currency struct {
	AlphabeticCode string `json:"alphabeticCode" db:"alphabetic_code"`
	NumericCode    int    `json:"numericCode" db:"numeric_code"`
	MinorUnit      int    `json:"minorUnit" db:"minor_unit"`
	CurrencyName   string `json:"currencyName" db:"currency_name"`
}

type Ledgers []Ledger

type LedgerTypes []LedgerType
type LedgerTypesFirstGrades []LedgerTypesFirstGrade
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
			CASE
				WHEN created_at IS NULL THEN ''
				ELSE TO_CHAR(created_at, 'YYYY-MM-DD HH24:MI')
			END created_at,
			CASE
				WHEN updated_at IS NULL THEN ''
				ELSE TO_CHAR(updated_at, 'YYYY-MM-DD HH24:MI')
			END updated_at,
			(SELECT COUNT(*) FROM user_ledger_postings WHERE ledger_id = ledger_id) postings_count
		FROM
			user_ledgers
		WHERE
			ledger_owner = $1
			AND ledger_status < 2
	`
	ledgers := Ledgers{}
	err := conn.Select(&ledgers, query, userID)
	if err != nil {
		return Ledgers{}, err
	}

	queryLt := `
		SELECT
			ledger_type_id,
			CONCAT(t.first_grade, '. ', fg.type_name, ' (', fg.type_name_zh, ')') first_grade,
			CONCAT(t.second_grade, '. ', sg.type_name, ' (', sg.type_name_zh, ')') second_grade,
			CONCAT(t.third_grade, '. ', tg.type_name, ' (', tg.type_name_zh, ')') third_grade,
			CONCAT(t.type_name, ' (', t.type_name_zh, ')') type_name,
			CASE
				WHEN t.description_en IS NULL THEN ''
				ELSE CONCAT(t.description_en, ' (', t.description_zh, ')')
			END description,
			is_active
		FROM
			ref_ledger_types t
		JOIN ref_ledger_first_grade fg USING (first_grade)
		JOIN ref_ledger_second_grade sg USING (first_grade, second_grade)
		JOIN ref_ledger_third_grade tg USING (first_grade, second_grade, third_grade)
		WHERE
			ledger_type_id = $1
	`
	for i, l := range ledgers {
		lt := LedgerType{}
		err := conn.Get(&lt, queryLt, l.LedgerTypeID)
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
			t.first_grade,
			t.second_grade,
			t.third_grade,
			CONCAT(t.type_name, ' (', t.type_name_zh, ')') type_name,
			CASE
				WHEN t.description_en IS NULL THEN ''
				ELSE CONCAT(t.description_en, ' (', t.description_zh, ')')
			END description,
			is_active
		FROM
			ref_ledger_types t
		JOIN ref_ledger_first_grade fg USING (first_grade)
		JOIN ref_ledger_second_grade sg USING (first_grade, second_grade)
		JOIN ref_ledger_third_grade tg USING (first_grade, second_grade, third_grade)
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

func FindLedgerTypesFirstGrade(conn *sqlx.DB) (LedgerTypesFirstGrades, error) {
	query := `
		SELECT
			first_grade,
			CONCAT(fg.type_name, ' (', fg.type_name_zh, ')') type_name,
			CONCAT(fg.description_en, ' (', fg.description_zh, ')') description
		FROM
			ref_ledger_first_grade fg
	`
	ledgerTypesFirstGrades := LedgerTypesFirstGrades{}
	err := conn.Select(&ledgerTypesFirstGrades, query)
	if err != nil {
		return LedgerTypesFirstGrades{}, err
	}
	return ledgerTypesFirstGrades, nil
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

func DeleteLedger(ledgerID int, username string, conn *sqlx.DB) error {
	tx := conn.MustBegin()
	_, err := tx.Exec(`SELECT set_config('app.current_user', $1, TRUE);`, username)
	if err != nil {
		tx.Rollback()
		return err
	}
	_, err = tx.Exec(`UPDATE user_ledgers SET ledger_status = 2 WHERE ledger_id = $1;`, ledgerID)
	if err != nil {
		tx.Rollback()
		return err
	}
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		return err
	}
	return nil
}

func (ls *Ledgers) Save(conn *sqlx.DB, username string) error {

	currencies, err := FindCurrencies(conn)
	if err != nil {
		return err
	}
	ledgerTypes, err := FindLedgerTypes(conn)
	if err != nil {
		return err
	}

	tx := conn.MustBegin()
	sq := `SELECT set_config('app.current_user', $1, TRUE);`
	_, err = tx.Exec(sq, username)
	if err != nil {
		tx.Rollback()
		return err
	}

	query := `
		INSERT INTO user_ledgers (
			ledger_owner,
			ledger_name,
			ledger_type_id,
			currency,
			balance
		) VALUES (:ledger_owner, :ledger_name, :ledger_type_id, :currency, :balance);
	`

	for _, ledger := range *ls {
		if err := ledger.validateBeforeUpdate(currencies, ledgerTypes); err != nil {
			return err
		}
		_, err = tx.NamedExec(query, ledger)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		return err
	}

	return nil
}

func (ls *Ledgers) Edit(conn *sqlx.DB, username string) error {

	currencies, err := FindCurrencies(conn)
	if err != nil {
		return err
	}
	ledgerTypes, err := FindLedgerTypes(conn)
	if err != nil {
		return err
	}

	tx := conn.MustBegin()
	sq := `SELECT set_config('app.current_user', $1, TRUE);`
	_, err = tx.Exec(sq, username)
	if err != nil {
		tx.Rollback()
		return err
	}

	query := `
	UPDATE
		user_ledgers
	SET
		ledger_name = :ledger_name,
		ledger_type_id = :ledger_type_id,
		currency = :currency,
		balance = :balance,
		ledger_status = :ledger_status
	WHERE
		ledger_id = :ledger_id;
	`
	for _, ledger := range *ls {
		if err := ledger.validateBeforeUpdate(currencies, ledgerTypes); err != nil {
			return err
		}
		_, err = tx.NamedExec(query, ledger)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		return err
	}
	return nil
}

func (l *Ledger) validateBeforeUpdate(currencies Currencies, ledgerTypes LedgerTypes) error {
	if l.LedgerName == "" {
		return ErrLedgerNameRequired
	}
	if !slices.ContainsFunc(ledgerTypes, func(lt LedgerType) bool {
		return lt.LedgerTypeID == l.LedgerTypeID
	}) {
		return ErrLedgerTypeIDRequired
	}
	if !slices.ContainsFunc(currencies, func(c Currency) bool {
		return c.AlphabeticCode == l.Currency
	}) {
		return ErrCurrencyRequired
	}
	if l.Balance.IsNegative() {
		return ErrBalanceRequired
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

func (ltsfgs LedgerTypesFirstGrades) ToJSONString() (string, error) {
	json, err := json.Marshal(ltsfgs)
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

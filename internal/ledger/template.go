package ledger

import (
	"context"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// Accounts every book needs. They can be renamed but never deleted or archived.
const (
	KeyOpeningBalances = "opening_balances"
	KeyFXGainLoss      = "fx_gain_loss"
)

func isSystemKey(k *string) bool {
	return k != nil && (*k == KeyOpeningBalances || *k == KeyFXGainLoss)
}

type templateAccount struct {
	key         string
	parent      string
	class       db.AccountClass
	current     bool
	cash        bool
	cf          db.CfClass
	placeholder bool
}

// personalTemplate is copied into every new book. Names come from the
// frontend's i18n files (account.template.<key>); the database stores only
// the key until the user renames an account.
//
// Parents must come before their children.
var personalTemplate = []templateAccount{
	// Assets
	{key: "cash", class: db.AccountClassAsset, current: true, cash: true},
	{key: "bank", class: db.AccountClassAsset, current: true, placeholder: true},
	{key: "bank_checking", parent: "bank", class: db.AccountClassAsset, current: true, cash: true},
	{key: "bank_savings", parent: "bank", class: db.AccountClassAsset, current: true, cash: true},
	{key: "ewallet", class: db.AccountClassAsset, current: true, cash: true},
	{key: "receivables", class: db.AccountClassAsset, current: true},
	{key: "investments", class: db.AccountClassAsset, cf: db.CfClassInvesting, placeholder: true},
	{key: "brokerage_cash", parent: "investments", class: db.AccountClassAsset, current: true, cash: true},
	{key: "securities", parent: "investments", class: db.AccountClassAsset, cf: db.CfClassInvesting},
	{key: "property", class: db.AccountClassAsset, cf: db.CfClassInvesting, placeholder: true},
	{key: "property_home", parent: "property", class: db.AccountClassAsset, cf: db.CfClassInvesting},
	{key: "property_vehicle", parent: "property", class: db.AccountClassAsset, cf: db.CfClassInvesting},
	{key: "retirement", class: db.AccountClassAsset, cf: db.CfClassInvesting},

	// Liabilities
	{key: "credit_cards", class: db.AccountClassLiability, current: true, placeholder: true},
	{key: "credit_card", parent: "credit_cards", class: db.AccountClassLiability, current: true},
	{key: "loans", class: db.AccountClassLiability, cf: db.CfClassFinancing, placeholder: true},
	{key: "loan_mortgage", parent: "loans", class: db.AccountClassLiability, cf: db.CfClassFinancing},
	{key: "loan_car", parent: "loans", class: db.AccountClassLiability, cf: db.CfClassFinancing},
	{key: "loan_personal", parent: "loans", class: db.AccountClassLiability, current: true, cf: db.CfClassFinancing},
	{key: "payables", class: db.AccountClassLiability, current: true},

	// Equity. Retained earnings are computed, never an account.
	{key: KeyOpeningBalances, class: db.AccountClassEquity},

	// Income
	{key: "salary", class: db.AccountClassIncome},
	{key: "bonus", class: db.AccountClassIncome},
	{key: "interest_income", class: db.AccountClassIncome},
	{key: "dividends", class: db.AccountClassIncome},
	{key: "realized_gains", class: db.AccountClassIncome, cf: db.CfClassInvesting},
	{key: KeyFXGainLoss, class: db.AccountClassIncome},
	// Card cash back and points. Treated as income, not as a discount on each
	// purchase, so an expense keeps what it actually cost.
	{key: "card_rewards", class: db.AccountClassIncome},
	{key: "gifts_received", class: db.AccountClassIncome},
	{key: "other_income", class: db.AccountClassIncome},

	// Expenses
	{key: "housing", class: db.AccountClassExpense, placeholder: true},
	{key: "rent", parent: "housing", class: db.AccountClassExpense},
	{key: "utilities", parent: "housing", class: db.AccountClassExpense},
	{key: "home_maintenance", parent: "housing", class: db.AccountClassExpense},
	{key: "food", class: db.AccountClassExpense, placeholder: true},
	{key: "groceries", parent: "food", class: db.AccountClassExpense},
	{key: "dining", parent: "food", class: db.AccountClassExpense},
	{key: "transport", class: db.AccountClassExpense},
	{key: "health", class: db.AccountClassExpense},
	{key: "insurance", class: db.AccountClassExpense},
	{key: "education", class: db.AccountClassExpense},
	{key: "children", class: db.AccountClassExpense},
	{key: "pets", class: db.AccountClassExpense},
	{key: "clothing", class: db.AccountClassExpense},
	{key: "entertainment", class: db.AccountClassExpense},
	{key: "travel", class: db.AccountClassExpense},
	{key: "gifts_donations", class: db.AccountClassExpense},
	{key: "taxes", class: db.AccountClassExpense},
	{key: "fees", class: db.AccountClassExpense},
	{key: "interest_expense", class: db.AccountClassExpense, cf: db.CfClassFinancing},
	{key: "subscriptions", class: db.AccountClassExpense},
	{key: "other_expense", class: db.AccountClassExpense},
}

// seedTemplate inserts personalTemplate into a new book. Balance-sheet
// accounts take the book's base currency; income and expense accounts hold none.
func seedTemplate(ctx context.Context, q *db.Queries, bookID int64, base string) error {
	ids := make(map[string]int64, len(personalTemplate))
	for _, t := range personalTemplate {
		key := t.key
		cf := t.cf
		if cf == "" {
			cf = db.CfClassOperating
		}
		p := db.CreateAccountParams{
			BookID:        bookID,
			Class:         t.class,
			TemplateKey:   &key,
			IsCurrent:     t.current,
			IsCash:        t.cash,
			CfClass:       cf,
			IsPlaceholder: t.placeholder,
		}
		if t.parent != "" {
			id := ids[t.parent]
			p.ParentID = &id
		}
		if holdsCommodity(t.class) {
			c := base
			p.Commodity = &c
		}
		a, err := q.CreateAccount(ctx, p)
		if err != nil {
			return err
		}
		ids[t.key] = a.ID
	}
	return nil
}

func holdsCommodity(c db.AccountClass) bool {
	return c == db.AccountClassAsset || c == db.AccountClassLiability || c == db.AccountClassEquity
}

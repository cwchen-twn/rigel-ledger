package ledger

import (
	"testing"
)

func TestRebaseRetranslatesEveryPosting(t *testing.T) {
	f, ids := reportFixture(t)
	// A foreign line entered with its own base (100 USD booked as 3,100 TWD,
	// not the day's 3,000): re-translated it is 100 USD, while the TWD side
	// becomes 3,100 / 30 = 103.33, so 3.33 goes to FX gain/loss.
	f.post(t, "2026-02-20",
		LineInput{AccountID: ids["usd"], Amount: d("100"), BaseAmount: dp("3100")},
		LineInput{AccountID: f.keys["bank_checking"], Amount: d("-3100")})

	// Nothing known about PYG: the dry run lists what is missing.
	plan, err := f.svc.Rebase(f.ctx, f.acc, "pyg", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Gaps) == 0 || plan.Gaps[0].To != "PYG" {
		t.Fatalf("PYG plan = %+v", plan)
	}
	_, err = f.svc.Rebase(f.ctx, f.acc, "PYG", false)
	wantCode(t, err, "rebase_rates_missing")

	plan, err = f.svc.Rebase(f.ctx, f.acc, "USD", true)
	if err != nil || len(plan.Gaps) != 0 || plan.Transactions != 5 || plan.Adjusted != 1 || !plan.Residue.Equal(d("3.33")) {
		t.Fatalf("USD dry run = %+v %v", plan, err)
	}
	if acc, _ := f.svc.ResolveAccess(f.ctx, f.user.ID, f.acc.Book.ID); acc.Book.BaseCurrency != "TWD" {
		t.Fatal("the dry run changed the book")
	}

	if _, err := f.svc.Rebase(f.ctx, f.acc, "USD", false); err != nil {
		t.Fatal(err)
	}
	acc, _ := f.svc.ResolveAccess(f.ctx, f.user.ID, f.acc.Book.ID)
	if acc.Book.BaseCurrency != "USD" {
		t.Fatalf("base = %s", acc.Book.BaseCurrency)
	}
	// In USD the book reads as the translated TWD book did: 500 USD cash,
	// 5 AAPL x 120 = 600, and checking 44,900 TWD at 32 -- now the foreign
	// balance, revalued (it cost 1,496.67).
	bs, err := f.svc.BalanceSheet(f.ctx, acc, day("2026-03-31"), "USD")
	if err != nil {
		t.Fatal(err)
	}
	if !bs.TotalAssets.Equal(bs.TotalEquity) {
		t.Fatalf("unbalanced after rebase: %s vs %s", bs.TotalAssets, bs.TotalEquity)
	}
	usd := lineOf(bs.Lines, ids["usd"])
	if !usd.Amount.Equal(d("600")) || usd.Revalued {
		t.Fatalf("USD account after rebase = %+v", usd)
	}
	fx := balanceOf(mustBalances(t, f, acc), f.keys[KeyFXGainLoss])
	if !fx.BaseAmount.Equal(d("3.33")) {
		t.Fatalf("FX gain/loss = %s, want 3.33", fx.BaseAmount)
	}
	// Idempotent guard, and owners only.
	_, err = f.svc.Rebase(f.ctx, acc, "USD", false)
	wantCode(t, err, "invalid_input")
}

func mustBalances(t *testing.T, f *fixture, a Access) Balances {
	t.Helper()
	b, err := f.svc.Balances(f.ctx, a, day("2026-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRebaseRefusesALockedBook(t *testing.T) {
	f, _ := reportFixture(t)
	if _, err := f.store.Pool.Exec(f.ctx, "UPDATE books SET lock_date = '2026-01-31' WHERE id = $1", f.acc.Book.ID); err != nil {
		t.Fatal(err)
	}
	acc, _ := f.svc.ResolveAccess(f.ctx, f.user.ID, f.acc.Book.ID)
	_, err := f.svc.Rebase(f.ctx, acc, "USD", false)
	wantCode(t, err, "book_locked")
}

func TestTagReport(t *testing.T) {
	f, _ := reportFixture(t)
	tagged := func(date string, tags []string, lines ...LineInput) {
		t.Helper()
		if _, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{Date: day(date), Lines: lines, Tags: tags}); err != nil {
			t.Fatal(err)
		}
	}
	tagged("2026-03-01", []string{"Japan 2026"},
		LineInput{AccountID: f.keys["travel"], Amount: d("12000")},
		LineInput{AccountID: f.keys["credit_card"], Amount: d("-12000")})
	tagged("2026-03-02", []string{"Japan 2026"},
		LineInput{AccountID: f.keys["dining"], Amount: d("1500")},
		LineInput{AccountID: f.keys["cash"], Amount: d("-1500")})
	tagged("2026-03-10", []string{"Dora"},
		LineInput{AccountID: f.keys["education"], Amount: d("800")},
		LineInput{AccountID: f.keys["bank_checking"], Amount: d("-800")})

	r, err := f.svc.Tags(f.ctx, f.acc, "TWD", day("2026-03-31"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tags) != 2 || r.Tags[0].Name != "Dora" || r.Tags[1].Name != "Japan 2026" || !r.Tags[1].Expenses.Equal(d("13500")) ||
		r.Tags[1].Transactions != 2 {
		t.Fatalf("tags = %+v", r.Tags)
	}
	detail, err := f.svc.Tag(f.ctx, f.acc, "japan 2026", "TWD", day("2026-03-31"))
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Expenses.Equal(d("13500")) || !lineOf(detail.Lines, f.keys["food"]).Total.Equal(d("1500")) {
		t.Fatalf("detail = %+v", detail)
	}
	// In USD at the date's rate (32).
	usd, err := f.svc.Tag(f.ctx, f.acc, "Japan 2026", "USD", day("2026-03-31"))
	if err != nil || !usd.Expenses.Equal(d("421.88")) {
		t.Fatalf("in USD = %s %v", usd.Expenses, err)
	}
}

func TestDeleteBookRemovesEverything(t *testing.T) {
	f, _ := reportFixture(t)
	if _, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{Date: day("2026-03-01"), Tags: []string{"Japan"},
		Lines: []LineInput{{AccountID: f.keys["travel"], Amount: d("100")}, {AccountID: f.keys["cash"], Amount: d("-100")}}}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.svc.DeleteBook(f.ctx, f.acc, "family"), "invalid_input") // the name is "Family"

	if _, err := f.store.Pool.Exec(f.ctx, "UPDATE books SET lock_date = '2026-01-01' WHERE id = $1", f.acc.Book.ID); err != nil {
		t.Fatal(err)
	}
	locked, _ := f.svc.ResolveAccess(f.ctx, f.user.ID, f.acc.Book.ID)
	wantCode(t, f.svc.DeleteBook(f.ctx, locked, "Family"), "book_locked")
	if _, err := f.store.Pool.Exec(f.ctx, "UPDATE books SET lock_date = NULL WHERE id = $1", f.acc.Book.ID); err != nil {
		t.Fatal(err)
	}

	if err := f.svc.DeleteBook(f.ctx, f.acc, " Family "); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := f.store.Pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM books) + (SELECT count(*) FROM accounts)
		+ (SELECT count(*) FROM transactions) + (SELECT count(*) FROM postings) + (SELECT count(*) FROM tags)`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d rows left behind", left)
	}
	u, _ := f.store.GetUserByID(f.ctx, f.user.ID)
	if u.DefaultBookID != nil {
		t.Fatal("the default book still points at the deleted one")
	}
}

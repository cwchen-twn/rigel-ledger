package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/dbtest"
)

type fixture struct {
	ctx   context.Context
	store *db.Store
	svc   *Service
	user  db.User
	acc   Access
	keys  map[string]int64 // template_key -> account id
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func newUser(t *testing.T, f *fixture, name string) db.User {
	t.Helper()
	u, err := f.svc.CreateUser(f.ctx, NewUser{Username: name, Email: name + "@example.com", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// setup creates a user and a TWD book seeded with the personal template.
func setup(t *testing.T) *fixture {
	t.Helper()
	store := dbtest.New(t)
	f := &fixture{ctx: context.Background(), store: store, svc: NewService(store)}
	f.user = newUser(t, f, "alice")
	book, err := f.svc.CreateBook(f.ctx, f.user.ID, "Family", "TWD")
	if err != nil {
		t.Fatal(err)
	}
	f.acc, err = f.svc.ResolveAccess(f.ctx, f.user.ID, book.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.keys = map[string]int64{}
	accts, err := f.svc.ListAccounts(f.ctx, f.acc)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range accts {
		if a.TemplateKey != nil {
			f.keys[*a.TemplateKey] = a.ID
		}
	}
	return f
}

func (f *fixture) usdAccount(t *testing.T, name string) int64 {
	t.Helper()
	usd := "USD"
	a, err := f.svc.CreateAccount(f.ctx, f.acc, AccountInput{
		Class: db.AccountClassAsset, Name: &name, Commodity: &usd, IsCurrent: true, IsCash: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func (f *fixture) rate(t *testing.T, from, to, on, rate string) {
	t.Helper()
	if _, err := f.svc.AddPrice(f.ctx, f.acc, PriceInput{Commodity: from, Quote: to, Date: day(on), Rate: d(rate)}); err != nil {
		t.Fatal(err)
	}
}

func wantCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	var le *Error
	if !errors.As(err, &le) {
		t.Fatalf("error = %v, want ledger error %q", err, code)
	}
	if le.Code != code {
		t.Fatalf("code = %q (%s), want %q", le.Code, le.Message, code)
	}
	return le
}

func balanceOf(b Balances, id int64) AccountBalance {
	for _, a := range b.Accounts {
		if a.AccountID == id {
			return a
		}
	}
	return AccountBalance{}
}

func TestCreateBookSeedsTemplate(t *testing.T) {
	f := setup(t)
	if len(f.keys) != len(personalTemplate) {
		t.Fatalf("seeded %d accounts, want %d", len(f.keys), len(personalTemplate))
	}
	if f.acc.Role != db.MemberRoleOwner {
		t.Fatalf("creator role = %s, want owner", f.acc.Role)
	}
	u, _ := f.store.GetUserByID(f.ctx, f.user.ID)
	if u.DefaultBookID == nil || *u.DefaultBookID != f.acc.Book.ID {
		t.Fatal("first book did not become the default")
	}
	cash, _ := f.store.GetAccount(f.ctx, db.GetAccountParams{BookID: f.acc.Book.ID, ID: f.keys["cash"]})
	if cash.Commodity == nil || *cash.Commodity != "TWD" || !cash.IsCash {
		t.Fatalf("cash account = %+v, want a TWD cash account", cash)
	}
	groceries, _ := f.store.GetAccount(f.ctx, db.GetAccountParams{BookID: f.acc.Book.ID, ID: f.keys["groceries"]})
	if groceries.Commodity != nil || groceries.ParentID == nil || *groceries.ParentID != f.keys["food"] {
		t.Fatalf("groceries = %+v, want no commodity and parent food", groceries)
	}
}

func TestBaseCurrencyTransactionAndBalances(t *testing.T) {
	f := setup(t)
	_, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date: day("2026-09-01"), Payee: "Carrefour",
		Lines: []LineInput{
			{AccountID: f.keys["groceries"], Amount: d("500")},
			{AccountID: f.keys["cash"], Amount: d("-500")},
		},
		Tags: []string{"alice", "Alice", " "},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.svc.Balances(f.ctx, f.acc, day("2026-09-30"))
	if err != nil {
		t.Fatal(err)
	}
	if !b.Check.IsZero() {
		t.Fatalf("check = %s, want 0", b.Check)
	}
	if got := balanceOf(b, f.keys["food"]).TotalBaseAmount; !got.Equal(d("500")) {
		t.Fatalf("food subtree = %s, want 500", got)
	}
	if got := balanceOf(b, f.keys["cash"]).BaseAmount; !got.Equal(d("-500")) {
		t.Fatalf("cash = %s, want -500", got)
	}
	if got := b.ClassTotals[db.AccountClassExpense]; !got.Equal(d("500")) {
		t.Fatalf("expense total = %s, want 500", got)
	}
	before, _ := f.svc.Balances(f.ctx, f.acc, day("2026-08-31"))
	if !balanceOf(before, f.keys["cash"]).BaseAmount.IsZero() {
		t.Fatal("as_of did not exclude a later transaction")
	}
	tags, _ := f.svc.ListTags(f.ctx, f.acc)
	if len(tags) != 1 {
		t.Fatalf("tags = %v, want the duplicate collapsed to one", tags)
	}
}

func TestForeignLineUsesRate(t *testing.T) {
	f := setup(t)
	usd := f.usdAccount(t, "Chase")
	f.rate(t, "USD", "TWD", "2026-09-01", "32.1")
	v, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date: day("2026-09-02"),
		Lines: []LineInput{
			{AccountID: f.keys["travel"], Commodity: "USD", Amount: d("10")},
			{AccountID: usd, Amount: d("-10")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Postings[0].BaseAmount.Equal(d("321")) || !v.Postings[1].BaseAmount.Equal(d("-321")) {
		t.Fatalf("base amounts = %s / %s, want 321 / -321", v.Postings[0].BaseAmount, v.Postings[1].BaseAmount)
	}
}

func TestFXRoundingResidueGoesToFXAccount(t *testing.T) {
	f := setup(t)
	usd := f.usdAccount(t, "Chase")
	f.rate(t, "USD", "TWD", "2026-09-01", "32.5")
	// 3 x 0.01 USD = 0.325 -> 0.33 TWD each (0.99); -0.03 USD = -0.975 -> -0.98.
	v, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date: day("2026-09-01"),
		Lines: []LineInput{
			{AccountID: f.keys["dining"], Commodity: "USD", Amount: d("0.01")},
			{AccountID: f.keys["dining"], Commodity: "USD", Amount: d("0.01")},
			{AccountID: f.keys["dining"], Commodity: "USD", Amount: d("0.01")},
			{AccountID: usd, Amount: d("-0.03")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Postings) != 5 {
		t.Fatalf("postings = %d, want 4 + an FX rounding line", len(v.Postings))
	}
	fx := v.Postings[4]
	if fx.AccountID != f.keys[KeyFXGainLoss] || !fx.BaseAmount.Equal(d("-0.01")) {
		t.Fatalf("rounding line = %+v, want -0.01 to fx_gain_loss", fx)
	}
}

func TestUnbalancedAndMissingRateAreRejected(t *testing.T) {
	f := setup(t)
	_, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date:  day("2026-09-01"),
		Lines: []LineInput{{AccountID: f.keys["groceries"], Amount: d("500")}, {AccountID: f.keys["cash"], Amount: d("-400")}},
	})
	wantCode(t, err, "unbalanced")

	usd := f.usdAccount(t, "Chase")
	_, err = f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date:  day("2026-09-01"),
		Lines: []LineInput{{AccountID: f.keys["groceries"], Amount: d("320")}, {AccountID: usd, Amount: d("-10")}},
	})
	le := wantCode(t, err, "invalid_input")
	if le.Fields["lines[1].base_amount"] != "rate_missing" {
		t.Fatalf("fields = %v, want lines[1].base_amount=rate_missing", le.Fields)
	}

	// With the base amount given explicitly, the same entry is fine.
	base := d("-320")
	if _, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date:  day("2026-09-01"),
		Lines: []LineInput{{AccountID: f.keys["groceries"], Amount: d("320")}, {AccountID: usd, Amount: d("-10"), BaseAmount: &base}},
	}); err != nil {
		t.Fatal(err)
	}

	_, err = f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date:  day("2026-09-01"),
		Lines: []LineInput{{AccountID: f.keys["food"], Amount: d("1")}, {AccountID: f.keys["cash"], Amount: d("-1")}},
	})
	if le := wantCode(t, err, "invalid_input"); le.Fields["lines[0].account_id"] != "placeholder" {
		t.Fatalf("fields = %v, want placeholder", le.Fields)
	}
}

func TestRateOnInverseAndCross(t *testing.T) {
	f := setup(t)
	f.rate(t, "USD", "TWD", "2026-09-01", "32")
	f.rate(t, "USD", "PYG", "2026-09-01", "7300")
	q := f.store.Queries

	r, ok, err := RateOn(f.ctx, q, "TWD", "USD", day("2026-09-05"))
	if err != nil || !ok || !r.Equal(d("0.03125")) {
		t.Fatalf("TWD->USD = %s %v %v, want 0.03125", r, ok, err)
	}
	r, ok, err = RateOn(f.ctx, q, "PYG", "TWD", day("2026-09-05"))
	want := decimal.NewFromInt(1).DivRound(d("7300"), rateDivisionPlaces).Mul(d("32"))
	if err != nil || !ok || !r.Equal(want) {
		t.Fatalf("PYG->TWD = %s %v %v, want %s", r, ok, err, want)
	}
	if _, ok, _ := RateOn(f.ctx, q, "USD", "TWD", day("2026-08-31")); ok {
		t.Fatal("a rate dated after the lookup date was used")
	}
}

func TestOpeningBalance(t *testing.T) {
	f := setup(t)
	name, usd := "Schwab", "USD"
	base := d("3200")
	a, err := f.svc.CreateAccount(f.ctx, f.acc, AccountInput{
		Class: db.AccountClassAsset, Name: &name, Commodity: &usd,
		Opening: &Opening{Amount: d("100"), BaseAmount: &base, Date: day("2026-01-01")},
	})
	if err != nil {
		t.Fatal(err)
	}
	card := "Visa"
	c, err := f.svc.CreateAccount(f.ctx, f.acc, AccountInput{
		ParentID: ptr(f.keys["credit_cards"]), Name: &card,
		Opening: &Opening{Amount: d("1500"), Date: day("2026-01-01")},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := f.svc.Balances(f.ctx, f.acc, day("2026-01-31"))
	if got := balanceOf(b, a.ID); !got.BaseAmount.Equal(d("3200")) || !got.Amounts[0].Amount.Equal(d("100")) {
		t.Fatalf("Schwab = %+v, want 100 USD / 3200 TWD", got)
	}
	if got := balanceOf(b, c.ID).BaseAmount; !got.Equal(d("-1500")) {
		t.Fatalf("Visa = %s, want -1500 (credit)", got)
	}
	if got := balanceOf(b, f.keys[KeyOpeningBalances]).BaseAmount; !got.Equal(d("-1700")) {
		t.Fatalf("opening balances = %s, want -1700", got)
	}
}

func TestLockDate(t *testing.T) {
	f := setup(t)
	v, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date:  day("2026-06-30"),
		Lines: []LineInput{{AccountID: f.keys["rent"], Amount: d("20000")}, {AccountID: f.keys["bank_checking"], Amount: d("-20000")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	lock := day("2026-06-30")
	if _, err := f.svc.UpdateBook(f.ctx, f.acc, BookUpdate{Name: "Family", LockDate: &lock, InterestDividendCfClass: db.CfClassOperating}); err != nil {
		t.Fatal(err)
	}
	f.acc, _ = f.svc.ResolveAccess(f.ctx, f.user.ID, f.acc.Book.ID)

	wantCode(t, f.svc.DeleteTransaction(f.ctx, f.acc, v.ID), "book_locked")
	_, err = f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date:  day("2026-06-01"),
		Lines: []LineInput{{AccountID: f.keys["rent"], Amount: d("1")}, {AccountID: f.keys["cash"], Amount: d("-1")}},
	})
	wantCode(t, err, "book_locked")

	// The trigger holds even if Go's check is bypassed.
	_, err = f.store.Pool.Exec(f.ctx, "DELETE FROM transactions WHERE id = $1", v.ID)
	wantCode(t, translate(err, "transaction"), "book_locked")
}

func TestRolesAndMembership(t *testing.T) {
	f := setup(t)
	bob := newUser(t, f, "bob")

	if _, err := f.svc.ResolveAccess(f.ctx, bob.ID, f.acc.Book.ID); err == nil {
		t.Fatal("a non-member resolved access to the book")
	} else if le := wantCode(t, err, "not_found"); le.Kind != KindNotFound {
		t.Fatal("non-member should see not found")
	}

	if err := f.svc.AddMember(f.ctx, f.acc, "bob", db.MemberRoleViewer); err != nil {
		t.Fatal(err)
	}
	bobAcc, err := f.svc.ResolveAccess(f.ctx, bob.ID, f.acc.Book.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.CreateTransaction(f.ctx, bobAcc, TransactionInput{
		Date:  day("2026-09-01"),
		Lines: []LineInput{{AccountID: f.keys["rent"], Amount: d("1")}, {AccountID: f.keys["cash"], Amount: d("-1")}},
	})
	if le := wantCode(t, err, "role"); le.Kind != KindForbidden {
		t.Fatal("viewer write should be forbidden")
	}

	wantCode(t, f.svc.RemoveMember(f.ctx, f.acc, f.user.ID), "last_owner")
	wantCode(t, f.svc.UpdateMemberRole(f.ctx, f.acc, f.user.ID, db.MemberRoleEditor), "last_owner")
	if err := f.svc.RemoveMember(f.ctx, bobAcc, bob.ID); err != nil {
		t.Fatalf("a viewer could not leave the book: %v", err)
	}
}

func TestListTransactionsFiltersAndPages(t *testing.T) {
	f := setup(t)
	post := func(date string, acct string, amt string, tags ...string) {
		t.Helper()
		if _, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
			Date: day(date), Payee: acct,
			Lines: []LineInput{{AccountID: f.keys[acct], Amount: d(amt)}, {AccountID: f.keys["cash"], Amount: d(amt).Neg()}},
			Tags:  tags,
		}); err != nil {
			t.Fatal(err)
		}
	}
	post("2026-09-01", "groceries", "100", "bob")
	post("2026-09-02", "dining", "200")
	post("2026-09-03", "rent", "300")
	post("2026-09-04", "groceries", "400")

	page1, next, err := f.svc.ListTransactions(f.ctx, f.acc, ListFilter{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 3 || next == "" || !page1[0].Date.Equal(day("2026-09-04")) {
		t.Fatalf("page1 = %d rows, next %q", len(page1), next)
	}
	page2, next2, _ := f.svc.ListTransactions(f.ctx, f.acc, ListFilter{Limit: 3, Cursor: next})
	if len(page2) != 1 || next2 != "" || !page2[0].Date.Equal(day("2026-09-01")) {
		t.Fatalf("page2 = %d rows, next %q", len(page2), next2)
	}

	food := f.keys["food"]
	byFood, _, _ := f.svc.ListTransactions(f.ctx, f.acc, ListFilter{AccountID: &food})
	if len(byFood) != 3 {
		t.Fatalf("food subtree matched %d, want 3", len(byFood))
	}
	byTag, _, _ := f.svc.ListTransactions(f.ctx, f.acc, ListFilter{Tag: "BOB"})
	if len(byTag) != 1 || byTag[0].Tags[0] != "bob" {
		t.Fatalf("tag filter matched %d", len(byTag))
	}
	byText, _, _ := f.svc.ListTransactions(f.ctx, f.acc, ListFilter{Query: "rent"})
	if len(byText) != 1 {
		t.Fatalf("text filter matched %d, want 1", len(byText))
	}
}

func TestAccountDeletionRules(t *testing.T) {
	f := setup(t)
	wantCode(t, f.svc.DeleteAccount(f.ctx, f.acc, f.keys[KeyFXGainLoss]), "system_account")
	wantCode(t, f.svc.DeleteAccount(f.ctx, f.acc, f.keys["food"]), "has_children")
	if _, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date:  day("2026-09-01"),
		Lines: []LineInput{{AccountID: f.keys["pets"], Amount: d("1")}, {AccountID: f.keys["cash"], Amount: d("-1")}},
	}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.svc.DeleteAccount(f.ctx, f.acc, f.keys["pets"]), "has_postings")
	if err := f.svc.DeleteAccount(f.ctx, f.acc, f.keys["children"]); err != nil {
		t.Fatalf("an unused account could not be deleted: %v", err)
	}
}

// A card purchase in USD is recorded on the day with an estimated rate, then
// corrected in place once Visa settles: its real TWD amount, the FX fee and
// the cash back. The expense ends up at Visa's rate, with no FX gain or loss,
// because the card itself is a TWD account.
func TestCardPurchaseEstimatedThenSettled(t *testing.T) {
	f := setup(t)
	card, travel, fees, rewards := f.keys["credit_card"], f.keys["travel"], f.keys["fees"], f.keys["card_rewards"]
	f.rate(t, "USD", "TWD", "2026-09-21", "31.50")

	est := d("-9450") // 300 x 31.50, what the form pre-fills
	v, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date: day("2026-09-22"), Payee: "OTA flight",
		Lines: []LineInput{
			{AccountID: travel, Commodity: "USD", Amount: d("300")},
			{AccountID: card, Amount: est},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v.Postings[1].Status != db.PostingStatusUncleared || !v.Postings[0].BaseAmount.Equal(d("9450")) {
		t.Fatalf("estimate = %+v", v.Postings)
	}

	visa := d("9468") // 300 x 31.56
	cleared := day("2026-09-24")
	v, err = f.svc.UpdateTransaction(f.ctx, f.acc, v.ID, TransactionInput{
		Date: day("2026-09-22"), Payee: "OTA flight",
		Lines: []LineInput{
			{AccountID: travel, Commodity: "USD", Amount: d("300"), BaseAmount: &visa},
			{AccountID: fees, Amount: d("142"), Memo: "FX fee 1.5%"},
			{AccountID: card, Amount: d("-9610"), Status: db.PostingStatusCleared, ClearedOn: &cleared},
			{AccountID: card, Amount: d("189"), Status: db.PostingStatusCleared, ClearedOn: &cleared, Memo: "cash back 2%"},
			{AccountID: rewards, Amount: d("-189")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Postings) != 5 {
		t.Fatalf("postings = %d, want 5 and no FX rounding line", len(v.Postings))
	}

	b, _ := f.svc.Balances(f.ctx, f.acc, day("2026-09-30"))
	if got := balanceOf(b, card).BaseAmount; !got.Equal(d("-9421")) {
		t.Fatalf("card = %s, want -9421 (9468 + 142 - 189)", got)
	}
	if got := balanceOf(b, travel).BaseAmount; !got.Equal(d("9468")) {
		t.Fatalf("travel = %s, want 9468 at Visa's rate", got)
	}
	if got := balanceOf(b, f.keys[KeyFXGainLoss]).BaseAmount; !got.IsZero() {
		t.Fatalf("fx gain/loss = %s, want 0", got)
	}
}

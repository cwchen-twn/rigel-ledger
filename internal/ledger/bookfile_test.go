package ledger

import (
	"encoding/json"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// canonical replaces the exporting database's ids with stable ones (an
// account's template key or name path), so two exports of the same book from
// different databases compare equal.
func canonical(t *testing.T, f BookFile) string {
	t.Helper()
	byID := map[int64]FileAccount{}
	for _, a := range f.Accounts {
		byID[a.ID] = a
	}
	var key func(id int64) string
	key = func(id int64) string {
		a := byID[id]
		k := ""
		if a.TemplateKey != nil {
			k = "@" + *a.TemplateKey
		} else if a.Name != nil {
			k = *a.Name
		}
		if a.ParentID != nil {
			k = key(*a.ParentID) + "/" + k
		}
		return k
	}
	keys := make([]string, 0, len(f.Accounts))
	for _, a := range f.Accounts {
		keys = append(keys, key(a.ID))
	}
	sort.Strings(keys)
	stable := map[string]int64{}
	for i, k := range keys {
		stable[k] = int64(i + 1)
	}
	id := func(old int64) int64 { return stable[key(old)] }
	idp := func(old *int64) *int64 {
		if old == nil {
			return nil
		}
		n := id(*old)
		return &n
	}

	accts := make([]FileAccount, len(f.Accounts))
	for i, a := range f.Accounts {
		a.ID, a.ParentID = id(a.ID), idp(a.ParentID)
		accts[i] = a
	}
	sort.Slice(accts, func(i, j int) bool { return accts[i].ID < accts[j].ID })
	f.Accounts = accts
	for i := range f.Transactions {
		for j := range f.Transactions[i].Lines {
			f.Transactions[i].Lines[j].AccountID = id(f.Transactions[i].Lines[j].AccountID)
		}
		for j := range f.Transactions[i].Items {
			f.Transactions[i].Items[j].AccountID = idp(f.Transactions[i].Items[j].AccountID)
		}
	}
	for i := range f.Assertions {
		f.Assertions[i].AccountID = id(f.Assertions[i].AccountID)
	}
	sort.Slice(f.Assertions, func(i, j int) bool {
		a, b := f.Assertions[i], f.Assertions[j]
		return a.AccountID < b.AccountID || (a.AccountID == b.AccountID && a.Date < b.Date)
	})
	srcIDs := map[int64]int64{}
	for i := range f.SourceAccounts {
		srcIDs[f.SourceAccounts[i].ID] = int64(i + 1)
		f.SourceAccounts[i].ID = int64(i + 1)
		f.SourceAccounts[i].AccountID = idp(f.SourceAccounts[i].AccountID)
		f.SourceAccounts[i].SettlementAccountID = idp(f.SourceAccounts[i].SettlementAccountID)
	}
	for i := range f.Rules {
		f.Rules[i].AccountID = id(f.Rules[i].AccountID)
		if s := f.Rules[i].SourceAccountID; s != nil {
			n := srcIDs[*s]
			f.Rules[i].SourceAccountID = &n
		}
	}
	f.ExportedAt = time.Time{}
	f.Book.Name = ""
	b, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A book with a bit of everything the file carries: foreign cash, a share,
// tags, an archived account, a manual balance, a rule and a lock date.
func bookFileFixture(t *testing.T) (*fixture, map[string]int64) {
	t.Helper()
	f, ids := reportFixture(t)
	if _, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{
		Date: day("2026-02-20"), Payee: "Lunch", Memo: "with the team", Tags: []string{"work", "Trip 2026"},
		Lines: []LineInput{{AccountID: f.keys["dining"], Amount: d("450")}, {AccountID: f.keys["bank_checking"], Amount: d("-450")}},
	}); err != nil {
		t.Fatal(err)
	}
	closed := f.usdAccount(t, "Closed USD")
	f.post(t, "2026-01-05",
		LineInput{AccountID: closed, Amount: d("10")},
		LineInput{AccountID: f.keys[KeyOpeningBalances], Amount: d("-300")})
	f.post(t, "2026-01-06",
		LineInput{AccountID: ids["usd"], Amount: d("10")},
		LineInput{AccountID: closed, Amount: d("-10")})
	if _, err := f.svc.SetAccountArchived(f.ctx, f.acc, closed, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetBalance(f.ctx, f.acc, ids["usd"], day("2026-03-31"), d("510")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateRule(f.ctx, f.acc, "px mart", f.keys["groceries"], nil); err != nil {
		t.Fatal(err)
	}
	lock := day("2026-01-31")
	b, err := f.svc.UpdateBook(f.ctx, f.acc, BookUpdate{Name: f.acc.Book.Name, LockDate: &lock, InterestDividendCfClass: db.CfClassInvesting})
	if err != nil {
		t.Fatal(err)
	}
	f.acc.Book = b
	return f, ids
}

func TestBookFileRoundTrip(t *testing.T) {
	f, _ := bookFileFixture(t)
	file, err := f.svc.ExportBook(f.ctx, f.acc)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Commodities) != 1 || file.Commodities[0].Code != "XNAS:AAPL" || len(file.Prices) != 3 {
		t.Errorf("commodities %+v, %d prices", file.Commodities, len(file.Prices))
	}

	// Through JSON, as a file would travel.
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	var back BookFile
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.ImportBook(f.ctx, f.user.ID, back, "Family (imported)")
	if err != nil {
		t.Fatal(err)
	}
	if res.Book.Name != "Family (imported)" || res.Transactions != len(file.Transactions) || res.Assertions != 1 {
		t.Errorf("result %+v", res)
	}
	// The rates are already here: none added, none changed.
	if res.PricesAdded != 0 || res.PricesSkipped != 3 {
		t.Errorf("prices added %d skipped %d", res.PricesAdded, res.PricesSkipped)
	}
	if res.Book.LockDate == nil || fileDate(*res.Book.LockDate) != "2026-01-31" || res.Book.InterestDividendCfClass != db.CfClassInvesting {
		t.Errorf("book settings %+v", res.Book)
	}

	acc2, err := f.svc.ResolveAccess(f.ctx, f.user.ID, res.Book.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.svc.ExportBook(f.ctx, acc2)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := canonical(t, file), canonical(t, again); a != b {
		t.Errorf("the re-export differs from the export:\n%s\n---\n%s", a, b)
	}

	// And the statements agree.
	bs1, err := f.svc.BalanceSheet(f.ctx, f.acc, day("2026-03-31"), "")
	if err != nil {
		t.Fatal(err)
	}
	bs2, err := f.svc.BalanceSheet(f.ctx, acc2, day("2026-03-31"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !bs1.TotalAssets.Equal(bs2.TotalAssets) || !bs1.Unrealised.Equal(bs2.Unrealised) || !bs1.TotalEquity.Equal(bs2.TotalEquity) {
		t.Errorf("balance sheets: %s/%s/%s vs %s/%s/%s", bs1.TotalAssets, bs1.Unrealised, bs1.TotalEquity, bs2.TotalAssets, bs2.Unrealised, bs2.TotalEquity)
	}
}

func TestBookFileImportFailsWhole(t *testing.T) {
	f, _ := bookFileFixture(t)
	file, err := f.svc.ExportBook(f.ctx, f.acc)
	if err != nil {
		t.Fatal(err)
	}
	before, err := f.svc.ListBooks(f.ctx, f.user.ID)
	if err != nil {
		t.Fatal(err)
	}

	// The last transaction no longer balances: refused, not repaired, and
	// nothing of the book is left behind.
	bad := file
	bad.Transactions = append([]FileTransaction(nil), file.Transactions...)
	last := &bad.Transactions[len(bad.Transactions)-1]
	last.Lines = append([]FileLine(nil), last.Lines...)
	last.Lines[0].Amount = last.Lines[0].Amount.Add(d("1"))
	last.Lines[0].BaseAmount = last.Lines[0].BaseAmount.Add(d("1"))
	_, err = f.svc.ImportBook(f.ctx, f.user.ID, bad, "")
	le := wantCode(t, err, "unbalanced")
	if _, ok := le.Fields["transactions["+strconv.Itoa(len(bad.Transactions)-1)+"].lines"]; !ok {
		t.Errorf("fields %v do not name the transaction", le.Fields)
	}
	after, err := f.svc.ListBooks(f.ctx, f.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("%d books after a failed import, %d before", len(after), len(before))
	}

	// A commodity this install knows with another meaning is refused.
	conflict := file
	conflict.Commodities = []FileCommodity{{Code: "XNAS:AAPL", Kind: db.CommodityKindSecurity, Name: "Apple", Decimals: 4, QuoteCurrency: ptr("TWD")}}
	_, err = f.svc.ImportBook(f.ctx, f.user.ID, conflict, "")
	wantCode(t, err, "commodity_conflict")

	other := file
	other.Format = "something-else"
	_, err = f.svc.ImportBook(f.ctx, f.user.ID, other, "")
	wantCode(t, err, "file_format")
}

// An imported price only fills a day nobody has a rate for.
func TestBookFileImportAddsMissingPricesOnly(t *testing.T) {
	f, _ := bookFileFixture(t)
	file, err := f.svc.ExportBook(f.ctx, f.acc)
	if err != nil {
		t.Fatal(err)
	}
	file.Prices = append(file.Prices,
		FilePrice{Commodity: "USD", Quote: "TWD", Date: "2026-03-31", Rate: d("99"), Source: "manual"}, // the day has a rate
		FilePrice{Commodity: "USD", Quote: "TWD", Date: "2026-02-27", Rate: d("31"), Source: "manual"}, // a new day
		FilePrice{Commodity: "USD", Quote: "TWD", Date: "2026-01-10", Rate: d("29"), Source: "manual"}, // frozen by the lock
	)
	res, err := f.svc.ImportBook(f.ctx, f.user.ID, file, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.PricesAdded != 1 {
		t.Errorf("prices added %d, want 1", res.PricesAdded)
	}
	r, ok, err := RateOn(f.ctx, f.store.Queries, "USD", "TWD", day("2026-03-31"))
	if err != nil || !ok || !r.Equal(d("32")) {
		t.Errorf("USD/TWD on 03-31 = %s %v %v, want the existing 32", r, ok, err)
	}
	ps, err := f.store.ListPrices(f.ctx, db.ListPricesParams{Commodity: ptr("USD"), Source: ptr("import"), Lim: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || fileDate(ps[0].Date) != "2026-02-27" {
		t.Errorf("imported prices %+v", ps)
	}
}

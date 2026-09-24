package ledger

import (
	"testing"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// A TWD book with foreign cash and a share, where every figure can be
// worked out by hand:
//
//	01-01  open a USD account with 1,000 USD at 30        (opening balance 30,000)
//	02-01  salary 50,000 into checking
//	02-10  groceries 2,000 from checking
//	02-15  buy 5 AAPL at 100 USD from the USD account      (500 USD = 15,000 at 30)
//	03-31  USD closes at 32; AAPL at 120 USD
func reportFixture(t *testing.T) (*fixture, map[string]int64) {
	t.Helper()
	f := setup(t)
	f.commodity(t, CommodityInput{Code: "XNAS:AAPL", Kind: db.CommodityKindSecurity, Name: "Apple", QuoteCurrency: ptr("USD")})
	ids := map[string]int64{
		"usd":  f.usdAccount(t, "US bank"),
		"aapl": f.holding(t, "Apple shares", "XNAS:AAPL"),
	}
	f.rate(t, "USD", "TWD", "2026-01-01", "30")
	f.post(t, "2026-01-01",
		LineInput{AccountID: ids["usd"], Amount: d("1000")},
		LineInput{AccountID: f.keys[KeyOpeningBalances], Amount: d("-30000")})
	f.post(t, "2026-02-01",
		LineInput{AccountID: f.keys["bank_checking"], Amount: d("50000")},
		LineInput{AccountID: f.keys["salary"], Amount: d("-50000")})
	f.post(t, "2026-02-10",
		LineInput{AccountID: f.keys["groceries"], Amount: d("2000")},
		LineInput{AccountID: f.keys["bank_checking"], Amount: d("-2000")})
	f.post(t, "2026-02-15",
		LineInput{AccountID: ids["aapl"], Amount: d("5"), UnitCost: dp("100")},
		LineInput{AccountID: ids["usd"], Amount: d("-500")})
	f.rate(t, "USD", "TWD", "2026-03-31", "32")
	f.rate(t, "XNAS:AAPL", "USD", "2026-03-31", "120")
	return f, ids
}

func lineOf(lines []ReportLine, id int64) ReportLine {
	for _, l := range lines {
		if l.AccountID == id {
			return l
		}
	}
	return ReportLine{}
}

func TestBalanceSheetAtClosingRates(t *testing.T) {
	f, ids := reportFixture(t)
	bs, err := f.svc.BalanceSheet(f.ctx, f.acc, day("2026-03-31"), "")
	if err != nil {
		t.Fatal(err)
	}
	// Cash 48,000 at cost; 500 USD at 32 = 16,000 (cost 15,000);
	// 5 AAPL x 120 x 32 = 19,200 (cost 15,000).
	want := map[string]string{
		"TotalAssets": "83200", "TotalLiabilities": "0",
		"EquityAccounts": "30000", "AccumulatedResult": "48000", "Unrealised": "5200", "TotalEquity": "83200",
	}
	got := map[string]string{
		"TotalAssets": bs.TotalAssets.String(), "TotalLiabilities": bs.TotalLiabilities.String(),
		"EquityAccounts": bs.EquityAccounts.String(), "AccumulatedResult": bs.AccumulatedResult.String(),
		"Unrealised": bs.Unrealised.String(), "TotalEquity": bs.TotalEquity.String(),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %s, want %s", k, got[k], v)
		}
	}
	usd, aapl := lineOf(bs.Lines, ids["usd"]), lineOf(bs.Lines, ids["aapl"])
	if !usd.Amount.Equal(d("16000")) || !usd.Historical.Equal(d("15000")) || !usd.Revalued {
		t.Errorf("USD line = %+v", usd)
	}
	if !aapl.Amount.Equal(d("19200")) || !aapl.Historical.Equal(d("15000")) {
		t.Errorf("AAPL line = %+v", aapl)
	}
	if len(bs.Missing) != 0 || len(bs.RatesUsed) != 2 {
		t.Errorf("rates used %+v, missing %v", bs.RatesUsed, bs.Missing)
	}
	for _, r := range bs.RatesUsed {
		if r.Date.Format("2006-01-02") != "2026-03-31" {
			t.Errorf("rate %s->%s dated %s", r.From, r.To, r.Date.Format("2006-01-02"))
		}
	}

	// Before the March rates: USD still at 30 and AAPL has no price, so it
	// stays at cost and is reported missing.
	early, err := f.svc.BalanceSheet(f.ctx, f.acc, day("2026-02-28"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !early.Unrealised.IsZero() || len(early.Missing) != 1 || early.Missing[0] != "XNAS:AAPL" {
		t.Errorf("February: unrealised %s, missing %v", early.Unrealised, early.Missing)
	}

	// The same statement in USD, translated at the closing rate (1/32).
	inUSD, err := f.svc.BalanceSheet(f.ctx, f.acc, day("2026-03-31"), "usd")
	if err != nil {
		t.Fatal(err)
	}
	if inUSD.Currency != "USD" || !inUSD.TotalAssets.Equal(d("2600")) || !inUSD.TotalAssets.Equal(inUSD.TotalEquity) {
		t.Errorf("in USD: %s %s = %s", inUSD.Currency, inUSD.TotalAssets, inUSD.TotalEquity)
	}
}

func TestIncomeStatementIncludesUnrealisedChange(t *testing.T) {
	f, _ := reportFixture(t)
	is, err := f.svc.IncomeStatement(f.ctx, f.acc, day("2026-01-01"), day("2026-03-31"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !is.Income.Equal(d("50000")) || !is.Expenses.Equal(d("2000")) || !is.Unrealised.Equal(d("5200")) || !is.NetResult.Equal(d("53200")) {
		t.Fatalf("income %s expenses %s unrealised %s net %s", is.Income, is.Expenses, is.Unrealised, is.NetResult)
	}
	// February alone: no price change inside it (USD still 30, no AAPL price).
	feb, err := f.svc.IncomeStatement(f.ctx, f.acc, day("2026-02-01"), day("2026-02-28"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !feb.NetResult.Equal(d("48000")) || !feb.Unrealised.IsZero() {
		t.Fatalf("February net %s unrealised %s", feb.NetResult, feb.Unrealised)
	}
	if _, err := f.svc.IncomeStatement(f.ctx, f.acc, day("2026-03-01"), day("2026-02-01"), ""); err == nil {
		t.Fatal("a backwards period was accepted")
	}
}

func TestCashFlowDirectMethod(t *testing.T) {
	f, ids := reportFixture(t)
	cf, err := f.svc.CashFlow(f.ctx, f.acc, day("2026-01-01"), day("2026-03-31"), "")
	if err != nil {
		t.Fatal(err)
	}
	// Opening: the 30,000 opening-balance entry is money already held, not a
	// flow. Operating: salary 50,000 - groceries 2,000. Investing: the shares,
	// -15,000. Closing: 48,000 + 500 USD at 32. FX: the 1,000 revaluation.
	want := map[string]string{"Opening": "30000", "Operating": "48000", "Investing": "-15000",
		"Financing": "0", "FXEffect": "1000", "Closing": "64000"}
	got := map[string]string{"Opening": cf.Opening.String(), "Operating": cf.Operating.String(),
		"Investing": cf.Investing.String(), "Financing": cf.Financing.String(),
		"FXEffect": cf.FXEffect.String(), "Closing": cf.Closing.String()}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %s, want %s", k, got[k], v)
		}
	}
	var sharesLine CashFlowLine
	for _, l := range cf.Lines {
		if l.AccountID == ids["aapl"] {
			sharesLine = l
		}
		if l.AccountID == f.keys[KeyOpeningBalances] {
			t.Error("the opening balance was reported as a flow")
		}
	}
	if sharesLine.Class != db.CfClassInvesting || !sharesLine.Amount.Equal(d("-15000")) {
		t.Errorf("shares line = %+v", sharesLine)
	}
	// A transfer between two cash accounts is no flow at all.
	f.post(t, "2026-03-15",
		LineInput{AccountID: f.keys["bank_savings"], Amount: d("10000")},
		LineInput{AccountID: f.keys["bank_checking"], Amount: d("-10000")})
	again, err := f.svc.CashFlow(f.ctx, f.acc, day("2026-01-01"), day("2026-03-31"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !again.Operating.Equal(cf.Operating) || !again.Closing.Equal(cf.Closing) {
		t.Errorf("a cash-to-cash transfer changed the flows: %s %s", again.Operating, again.Closing)
	}
}

package ledger

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

func (f *fixture) commodity(t *testing.T, in CommodityInput) db.Commodity {
	t.Helper()
	c, err := f.svc.CreateCommodity(f.ctx, f.acc, in)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *fixture) holding(t *testing.T, name, commodity string) int64 {
	t.Helper()
	a, err := f.svc.CreateAccount(f.ctx, f.acc, AccountInput{Class: db.AccountClassAsset, Name: &name, Commodity: &commodity})
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func (f *fixture) post(t *testing.T, date string, lines ...LineInput) TransactionView {
	t.Helper()
	v, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{Date: day(date), Lines: lines})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func dp(s string) *decimal.Decimal { v := d(s); return &v }

// Miles are bought, earned for free, then spent on a ticket plus cash tax.
// They carry no market price, only cost; redemption uses average cost.
func TestMilesBuyEarnRedeem(t *testing.T) {
	f := setup(t)
	f.commodity(t, CommodityInput{Code: "miles:eva", Kind: db.CommodityKindPoints, Name: "EVA Infinity MileageLands"})
	miles := f.holding(t, "EVA miles", "MILES:EVA")

	// Buy 20,000 miles for 12,000 TWD.
	f.post(t, "2026-01-10",
		LineInput{AccountID: miles, Amount: d("20000"), BaseAmount: dp("12000")},
		LineInput{AccountID: f.keys["bank_checking"], Amount: d("-12000")})
	// Earn 1,000 miles at zero cost.
	f.post(t, "2026-02-01",
		LineInput{AccountID: miles, Amount: d("1000"), BaseAmount: dp("0")},
		LineInput{AccountID: f.keys["card_rewards"], Commodity: "MILES:EVA", Amount: d("-1000"), BaseAmount: dp("0")})

	// A ticket for 10,500 miles + 2,000 TWD tax. The miles leave at average
	// cost: 12,000 / 21,000 per mile x 10,500 = 6,000 TWD.
	v := f.post(t, "2026-03-01",
		LineInput{AccountID: f.keys["travel"], Amount: d("8000")},
		LineInput{AccountID: miles, Amount: d("-10500")},
		LineInput{AccountID: f.keys["credit_card"], Amount: d("-2000")})
	if got := v.Postings[1].BaseAmount; !got.Equal(d("-6000")) {
		t.Fatalf("redeemed miles cost = %s, want -6000", got)
	}

	cb, err := f.svc.AccountCostBasis(f.ctx, f.acc, miles, day("2026-12-31"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !cb.Quantity.Equal(d("10500")) || !cb.Cost.Equal(d("6000")) {
		t.Fatalf("left = %s miles at %s TWD, want 10500 at 6000", cb.Quantity, cb.Cost)
	}

	// More than held is refused; all that is held takes the exact remaining cost.
	_, err = f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{Date: day("2026-04-01"), Lines: []LineInput{
		{AccountID: f.keys["travel"], Amount: d("1")},
		{AccountID: miles, Amount: d("-10501")},
	}})
	if le := wantCode(t, err, "invalid_input"); le.Fields["lines[1].amount"] != "exceeds_holdings" {
		t.Fatalf("fields = %v", le.Fields)
	}
	v = f.post(t, "2026-04-01",
		LineInput{AccountID: f.keys["travel"], Amount: d("6000")},
		LineInput{AccountID: miles, Amount: d("-10500")})
	if !v.Postings[1].BaseAmount.Equal(d("-6000")) {
		t.Fatalf("closing redemption = %s, want -6000", v.Postings[1].BaseAmount)
	}

	// Editing a redemption does not count the redemption itself.
	v, err = f.svc.UpdateTransaction(f.ctx, f.acc, v.ID, TransactionInput{Date: day("2026-04-01"), Lines: []LineInput{
		{AccountID: f.keys["travel"], Amount: d("6000")},
		{AccountID: miles, Amount: d("-10500")},
	}})
	if err != nil || !v.Postings[1].BaseAmount.Equal(d("-6000")) {
		t.Fatalf("edited redemption = %v / %v", v.Postings, err)
	}
}

func TestPointsNeedACostWhenAcquired(t *testing.T) {
	f := setup(t)
	f.commodity(t, CommodityInput{Code: "PTS:CATHAY", Kind: db.CommodityKindPoints, Name: "Cathay United points"})
	pts := f.holding(t, "Card points", "PTS:CATHAY")
	_, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{Date: day("2026-01-01"), Lines: []LineInput{
		{AccountID: pts, Amount: d("500")},
		{AccountID: f.keys["card_rewards"], Commodity: "PTS:CATHAY", Amount: d("-500")},
	}})
	if le := wantCode(t, err, "invalid_input"); le.Fields["lines[0].base_amount"] != "cost_required" {
		t.Fatalf("fields = %v", le.Fields)
	}
	_, err = f.svc.AddPrice(f.ctx, f.acc, PriceInput{Commodity: "PTS:CATHAY", Quote: "TWD", Date: day("2026-01-01"), Rate: d("0.3")})
	if le := wantCode(t, err, "invalid_input"); le.Fields["commodity"] != "unpriced" {
		t.Fatalf("fields = %v", le.Fields)
	}
}

// Shares: bought with a USD unit price (converted at the rate), sold at
// average cost with the gain booked to realized_gains.
func TestSharesBuyWithUnitCostAndSell(t *testing.T) {
	f := setup(t)
	f.commodity(t, CommodityInput{Code: "XNAS:AAPL", Kind: db.CommodityKindSecurity, Name: "Apple", QuoteCurrency: ptr("USD")})
	aapl := f.holding(t, "AAPL", "XNAS:AAPL")
	usd := f.usdAccount(t, "Firstrade cash")
	f.rate(t, "USD", "TWD", "2026-01-01", "32")

	buy := f.post(t, "2026-01-05",
		LineInput{AccountID: aapl, Amount: d("10"), UnitCost: dp("150")},
		LineInput{AccountID: usd, Amount: d("-1500")})
	if !buy.Postings[0].BaseAmount.Equal(d("48000")) || !buy.Postings[0].UnitCost.Decimal.Equal(d("150")) {
		t.Fatalf("buy = %+v, want base 48000 and unit cost 150", buy.Postings[0])
	}

	// Sell 4 at 200 USD: cost leaves at 48,000 x 4/10 = 19,200; proceeds 800 USD = 25,600 TWD.
	sell := f.post(t, "2026-06-01",
		LineInput{AccountID: usd, Amount: d("800")},
		LineInput{AccountID: aapl, Amount: d("-4")},
		LineInput{AccountID: f.keys["realized_gains"], Amount: d("-6400")})
	if !sell.Postings[1].BaseAmount.Equal(d("-19200")) {
		t.Fatalf("cost released = %s, want -19200", sell.Postings[1].BaseAmount)
	}
}

func TestCreateCommodityRules(t *testing.T) {
	f := setup(t)
	bad := []struct {
		in    CommodityInput
		field string
	}{
		{CommodityInput{Code: "AAPL", Kind: db.CommodityKindSecurity, Name: "x", QuoteCurrency: ptr("USD")}, "code"},
		{CommodityInput{Code: "XNAS:AAPL", Kind: db.CommodityKindSecurity, Name: "x"}, "quote_currency"},
		{CommodityInput{Code: "XNAS:AAPL", Kind: db.CommodityKindSecurity, Name: "x", QuoteCurrency: ptr("MILES")}, "quote_currency"},
		{CommodityInput{Code: "MILES:EVA", Kind: db.CommodityKindPoints, Name: "x", QuoteCurrency: ptr("TWD")}, "kind"},
		{CommodityInput{Code: "TWD2:X", Kind: db.CommodityKindCurrency, Name: "x"}, "kind"},
	}
	for _, c := range bad {
		_, err := f.svc.CreateCommodity(f.ctx, f.acc, c.in)
		if le := wantCode(t, err, "invalid_input"); le.Fields[c.field] == "" {
			t.Fatalf("%+v: fields = %v, want %s", c.in, le.Fields, c.field)
		}
	}
	tx := f.commodity(t, CommodityInput{Code: "XTAF:TX", Kind: db.CommodityKindSecurity, Name: "TAIEX futures",
		QuoteCurrency: ptr("TWD"), ContractSize: dp("200")})
	if !tx.ContractSize.Valid || !tx.ContractSize.Decimal.Equal(d("200")) || tx.Decimals != 4 {
		t.Fatalf("TX = %+v", tx)
	}
	// Created at runtime, it is immediately usable by an account.
	f.holding(t, "TX position", "XTAF:TX")
	_, err := f.svc.CreateCommodity(f.ctx, f.acc, CommodityInput{Code: "XTAF:TX", Kind: db.CommodityKindSecurity, Name: "again", QuoteCurrency: ptr("TWD")})
	wantCode(t, err, "duplicate")
}

func TestUpdateIdentity(t *testing.T) {
	f := setup(t)
	newUser(t, f, "bob")
	_, err := f.svc.UpdateIdentity(f.ctx, f.user, "wrong", "alicia", "alicia@example.com")
	if le := wantCode(t, err, "invalid_input"); le.Fields["current_password"] != "wrong" {
		t.Fatalf("fields = %v", le.Fields)
	}
	_, err = f.svc.UpdateIdentity(f.ctx, f.user, "correct horse", "bob", "alicia@example.com")
	if le := wantCode(t, err, "duplicate"); le.Fields["username"] != "taken" {
		t.Fatalf("fields = %v", le.Fields)
	}
	_, err = f.svc.UpdateIdentity(f.ctx, f.user, "correct horse", "alicia", "bob@example.com")
	if le := wantCode(t, err, "duplicate"); le.Fields["email"] != "taken" {
		t.Fatalf("fields = %v", le.Fields)
	}
	u, err := f.svc.UpdateIdentity(f.ctx, f.user, "correct horse", " Alicia ", "alicia@example.com")
	if err != nil || u.Username != "alicia" || u.Email != "alicia@example.com" {
		t.Fatalf("u = %+v, err %v", u, err)
	}
}

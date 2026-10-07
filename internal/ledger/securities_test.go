package ledger

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

func dp2(s string) *decimal.Decimal { v := d(s); return &v }

// A month from 集保 for one broker account, with the bank's own lines.
func brokerBatch() ImportInput {
	return ImportInput{Connector: "tw-tdcc", Label: "September",
		Accounts: []ImportAccount{
			{ExternalID: "9800-1234567", Label: "元大 1234567", Currency: "TWD", Kind: KindBrokerage},
			{ExternalID: "chk", Label: "Settlement", Currency: "TWD"},
		},
		Rows: []ImportRowInput{
			{Kind: "trade", Account: "9800-1234567", ID: "t1", Date: day("2026-09-02"), Description: "集保 買進",
				Security: "xtai:2330", SecurityName: "台積電", Units: dp2("1000"), Price: dp2("580")},
			{Kind: "trade", Account: "9800-1234567", ID: "t2", Date: day("2026-09-20"), Description: "集保 賣出",
				Security: "XTAI:2330", Units: dp2("-400"), Price: dp2("600"), Cash: dp2("239000")},
			{Kind: "holding", Account: "9800-1234567", ID: "h-0930-2330", Date: day("2026-09-30"),
				Security: "XTAI:2330", Units: dp2("1000")},
			// The bank's side of the buy, T+2, fee included.
			{Kind: "transaction", Account: "chk", ID: "b1", Date: day("2026-09-04"), Amount: d("-580826"), Description: "股票交割"},
		},
	}
}

func TestSecuritiesFromABroker(t *testing.T) {
	f := setup(t)
	if _, err := f.svc.Import(f.ctx, f.acc, brokerBatch()); err != nil {
		t.Fatal(err)
	}
	c, err := f.svc.validCommodity(f.ctx, "XTAI:2330")
	if err != nil || c.Name != "台積電" || c.Kind != db.CommodityKindSecurity || *c.QuoteCurrency != "TWD" {
		t.Fatalf("the security was not registered: %+v %v", c, err)
	}
	// A resend stages nothing twice.
	if res, err := f.svc.Import(f.ctx, f.acc, brokerBatch()); err != nil || res.Duplicates != 4 {
		t.Fatalf("resend = %+v %v", res, err)
	}

	// A cash account cannot settle through another; a brokerage's parent is an asset.
	srcs, _ := f.svc.SourceAccounts(f.ctx, f.acc)
	var broker, chk int64
	for _, s := range srcs {
		switch s.ExternalID {
		case "9800-1234567":
			broker = s.ID
		case "chk":
			chk = s.ID
		}
	}
	inv, checking := f.keys["investments"], f.keys["bank_checking"]
	_, err = f.svc.MapSourceAccount(f.ctx, f.acc, chk, SourceMapping{AccountID: &checking, SettlementAccountID: &checking})
	wantCode(t, err, "invalid_input")
	groceries := f.keys["groceries"]
	_, err = f.svc.MapSourceAccount(f.ctx, f.acc, broker, SourceMapping{AccountID: &groceries})
	wantCode(t, err, "invalid_input")
	_, err = f.svc.MapSourceAccount(f.ctx, f.acc, broker, SourceMapping{AccountID: &inv, SettlementAccountID: &inv})
	if le := wantCode(t, err, "invalid_input"); le.Fields["settlement_account_id"] != "invalid" {
		t.Fatalf("a group as settlement: %+v", le.Fields)
	}

	if _, err := f.svc.MapSourceAccount(f.ctx, f.acc, chk, SourceMapping{AccountID: &checking}); err != nil {
		t.Fatal(err)
	}
	// The parent only: the holding applies, a trade waits for the settlement account.
	if _, err := f.svc.MapSourceAccount(f.ctx, f.acc, broker, SourceMapping{AccountID: &inv}); err != nil {
		t.Fatal(err)
	}
	q := f.queue(t)
	if _, waiting := q["tw-tdcc:h-0930-2330"]; waiting {
		t.Fatal("the holding still waits after its account was mapped")
	}
	_, err = f.svc.Accept(f.ctx, f.acc, q["tw-tdcc:t2"].ID, AcceptInput{})
	wantCode(t, err, "settlement_unmapped")
	if _, err := f.svc.MapSourceAccount(f.ctx, f.acc, broker, SourceMapping{AccountID: &inv, SettlementAccountID: &checking}); err != nil {
		t.Fatal(err)
	}

	// The security's account was made under the parent, once.
	var sec db.Account
	accts, _ := f.svc.ListAccounts(f.ctx, f.acc)
	for _, a := range accts {
		if a.Commodity != nil && *a.Commodity == "XTAI:2330" {
			if sec.ID != 0 {
				t.Fatal("two accounts for one security")
			}
			sec = a
		}
	}
	if sec.ID == 0 || *sec.ParentID != inv || *sec.Name != "2330 台積電" || sec.CfClass != db.CfClassInvesting {
		t.Fatalf("security account = %+v", sec)
	}

	// The buy has no cash from 集保: confirmed from the bank's line.
	buy := q["tw-tdcc:t1"].ID
	_, err = f.svc.Accept(f.ctx, f.acc, buy, AcceptInput{})
	if le := wantCode(t, err, "invalid_input"); le.Fields["cash"] != "required" {
		t.Fatalf("buy without cash: %+v", le.Fields)
	}
	_, err = f.svc.Accept(f.ctx, f.acc, buy, AcceptInput{Cash: dp2("580826")})
	if le := wantCode(t, err, "invalid_input"); le.Fields["cash"] != "sign" {
		t.Fatalf("buy with cash in: %+v", le.Fields)
	}
	txn, err := f.svc.Accept(f.ctx, f.acc, buy, AcceptInput{Cash: dp2("-580826")})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, txn)
	if len(v.Postings) != 2 || !v.Postings[0].Amount.Equal(d("1000")) || !v.Postings[0].BaseAmount.Equal(d("580826")) ||
		!v.Postings[0].UnitCost.Decimal.Equal(d("580")) || v.Payee != "Buy 2330" {
		t.Fatalf("buy = %s %+v", v.Payee, v.Postings)
	}
	// The bank's settlement line is now the duplicate it is.
	if p := f.queue(t)["tw-tdcc:b1"]; p.Proposal != "duplicate" || p.MatchTransactionID == nil || *p.MatchTransactionID != txn {
		t.Fatalf("bank settlement row = %s %v", p.Proposal, p.MatchTransactionID)
	}

	// The sale: average cost leaves (400/1000 of 580,826), the rest is a gain.
	sale, err := f.svc.Accept(f.ctx, f.acc, q["tw-tdcc:t2"].ID, AcceptInput{})
	if err != nil {
		t.Fatal(err)
	}
	v, _ = f.svc.GetTransaction(f.ctx, f.acc, sale)
	if len(v.Postings) != 3 || !v.Postings[0].BaseAmount.Equal(d("-232330.4")) || !v.Postings[1].BaseAmount.Equal(d("239000")) ||
		v.Postings[2].AccountID != f.keys["realized_gains"] || !v.Postings[2].Amount.Equal(d("-6669.6")) {
		t.Fatalf("sale = %+v", v.Postings)
	}

	// 集保 said 1,000 at the end of September; the books hold 600: drift.
	drifts, err := f.svc.Drifts(f.ctx, f.acc)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 || drifts[0].AccountID != sec.ID || !drifts[0].Asserted.Equal(d("1000")) || !drifts[0].Booked.Equal(d("600")) {
		t.Fatalf("drifts = %+v", drifts)
	}
}

func TestSecurityRowsAreChecked(t *testing.T) {
	f := setup(t)
	bad := []struct {
		row   ImportRowInput
		field string
		code  string
	}{
		{ImportRowInput{Kind: "trade", Account: "chk", ID: "1", Date: day("2026-09-02"), Security: "XTAI:2330", Units: dp2("1")}, "rows[0].account", "invalid"},
		{ImportRowInput{Kind: "trade", Account: "b", ID: "1", Date: day("2026-09-02"), Security: "2330", Units: dp2("1")}, "rows[0].security", "invalid"},
		{ImportRowInput{Kind: "trade", Account: "b", ID: "1", Date: day("2026-09-02"), Security: "XTAI:2330"}, "rows[0].units", "required"},
		{ImportRowInput{Kind: "trade", Account: "b", ID: "1", Date: day("2026-09-02"), Security: "XTAI:2330", Units: dp2("10"), Cash: dp2("100")}, "rows[0].cash", "sign"},
		{ImportRowInput{Kind: "holding", Account: "b", ID: "1", Date: day("2026-09-02"), Security: "XTAI:2330", Units: dp2("-1")}, "rows[0].units", "sign"},
		{ImportRowInput{Kind: "trade", Account: "b", ID: "1", Date: day("2026-09-02"), Security: "XTAI:2330", Units: dp2("1.00001")}, "rows[0].units", "too_precise"},
	}
	for _, c := range bad {
		in := ImportInput{Connector: "tw-tdcc", Accounts: []ImportAccount{
			{ExternalID: "b", Currency: "TWD", Kind: KindBrokerage}, {ExternalID: "chk", Currency: "TWD"}}, Rows: []ImportRowInput{c.row}}
		_, err := f.svc.Import(f.ctx, f.acc, in)
		if le := wantCode(t, err, "invalid_input"); le.Fields[c.field] != c.code {
			t.Errorf("%+v: fields = %v, want %s=%s", c.row, le.Fields, c.field, c.code)
		}
	}
}

// One broker account, two sources (#81): 集保 sends each trade's units a
// settlement day late and without cash, Shioaji the same trade with its
// cash. The one with cash books, 集保's goes with it, both share one
// security account, and once 集保's view is set aside it imports nothing.
func TestTradesFromTwoSources(t *testing.T) {
	f := setup(t)
	inv, checking := f.keys["investments"], f.keys["bank_checking"]
	tdcc := func(rows ...ImportRowInput) ImportInput {
		return ImportInput{Connector: "tw-tdcc", Accounts: []ImportAccount{{ExternalID: "9A9c-0000001", Currency: "TWD", Kind: KindBrokerage}}, Rows: rows}
	}
	trade := func(acct, id, date, units string, cash *decimal.Decimal) ImportRowInput {
		return ImportRowInput{Kind: "trade", Account: acct, ID: id, Date: day(date), Security: "XTAI:2330", Units: dp2(units), Cash: cash}
	}
	if _, err := f.svc.Import(f.ctx, f.acc, tdcc(
		trade("9A9c-0000001", "t1", "2026-09-04", "1000", nil),
		trade("9A9c-0000001", "t2", "2026-09-22", "-400", nil),
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Import(f.ctx, f.acc, ImportInput{Connector: "tw-shioaji",
		Accounts: []ImportAccount{{ExternalID: "sinopac-stock-0001", Currency: "TWD", Kind: KindBrokerage}},
		Rows: []ImportRowInput{
			trade("sinopac-stock-0001", "s1", "2026-09-02", "1000", dp2("-580826")),
			trade("sinopac-stock-0001", "s2", "2026-09-20", "-400", dp2("239000")),
		}}); err != nil {
		t.Fatal(err)
	}
	srcs, _ := f.svc.SourceAccounts(f.ctx, f.acc)
	ids := map[string]int64{}
	for _, s := range srcs {
		ids[s.Connector] = s.ID
	}
	for _, c := range []string{"tw-tdcc", "tw-shioaji"} {
		if _, err := f.svc.MapSourceAccount(f.ctx, f.acc, ids[c], SourceMapping{AccountID: &inv, SettlementAccountID: &checking}); err != nil {
			t.Fatal(err)
		}
	}

	// 集保's rows wait for Shioaji's, which carry the cash.
	q := f.queue(t)
	for tdccID, shioajiID := range map[string]string{"tw-tdcc:t1": "tw-shioaji:s1", "tw-tdcc:t2": "tw-shioaji:s2"} {
		r, other := q[tdccID], q[shioajiID]
		if r.Proposal != "duplicate" || r.MatchRowID == nil || *r.MatchRowID != other.ID || other.Proposal != "new" {
			t.Fatalf("%s = %s %v, %s = %s", tdccID, r.Proposal, r.MatchRowID, shioajiID, other.Proposal)
		}
	}
	_, err := f.svc.Accept(f.ctx, f.acc, q["tw-tdcc:t1"].ID, AcceptInput{})
	wantCode(t, err, "match_pending")

	// Accepting Shioaji's books it once; 集保's goes with it.
	buy, err := f.svc.Accept(f.ctx, f.acc, q["tw-shioaji:s1"].ID, AcceptInput{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Accept(f.ctx, f.acc, q["tw-shioaji:s2"].ID, AcceptInput{}); err != nil {
		t.Fatal(err)
	}
	q = f.queue(t)
	if len(q) != 0 {
		t.Fatalf("still waiting: %v", q)
	}
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, buy)
	if !v.Postings[0].Amount.Equal(d("1000")) || !v.Postings[0].BaseAmount.Equal(d("580826")) {
		t.Fatalf("buy = %+v", v.Postings)
	}

	// 集保's units still land on the same account: one "2330" for both.
	if _, err := f.svc.Import(f.ctx, f.acc, tdcc(ImportRowInput{Kind: "holding", Account: "9A9c-0000001", ID: "h1",
		Date: day("2026-09-30"), Security: "XTAI:2330", Units: dp2("600")})); err != nil {
		t.Fatal(err)
	}
	var sec int64
	accts, _ := f.svc.ListAccounts(f.ctx, f.acc)
	for _, a := range accts {
		if a.Commodity != nil && *a.Commodity == "XTAI:2330" {
			if sec != 0 {
				t.Fatal("two accounts for one security")
			}
			sec = a.ID
		}
	}
	if drifts, err := f.svc.Drifts(f.ctx, f.acc); err != nil || len(drifts) != 0 {
		t.Fatalf("drifts = %+v %v", drifts, err)
	}

	// A trade typed in by hand: 集保's row for it is a duplicate of it.
	typed := f.post(t, "2026-10-05",
		LineInput{AccountID: sec, Amount: d("50"), BaseAmount: dp2("30000")},
		LineInput{AccountID: checking, Amount: d("-30000")})
	if _, err := f.svc.Import(f.ctx, f.acc, tdcc(trade("9A9c-0000001", "t3", "2026-10-07", "50", nil))); err != nil {
		t.Fatal(err)
	}
	r := f.queue(t)["tw-tdcc:t3"]
	if r.Proposal != "duplicate" || r.MatchTransactionID == nil || *r.MatchTransactionID != typed.ID {
		t.Fatalf("t3 = %s %v", r.Proposal, r.MatchTransactionID)
	}
	if got, err := f.svc.Accept(f.ctx, f.acc, r.ID, AcceptInput{}); err != nil || got != typed.ID {
		t.Fatalf("accept t3 = %d %v", got, err)
	}

	// 集保's view of this broker set aside: what waits and what comes is ignored.
	if _, err := f.svc.Import(f.ctx, f.acc, tdcc(trade("9A9c-0000001", "t4", "2026-10-08", "10", nil))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.MapSourceAccount(f.ctx, f.acc, ids["tw-tdcc"], SourceMapping{Ignored: true}); err != nil {
		t.Fatal(err)
	}
	if _, waiting := f.queue(t)["tw-tdcc:t4"]; waiting {
		t.Fatal("an ignored source's row still waits")
	}
	res, err := f.svc.Import(f.ctx, f.acc, tdcc(trade("9A9c-0000001", "t5", "2026-10-09", "10", nil)))
	if err != nil || res.Staged != 1 {
		t.Fatalf("import = %+v %v", res, err)
	}
	if len(f.queue(t)) != 0 {
		t.Fatalf("an ignored source's new row waits: %v", f.queue(t))
	}
	srcs, _ = f.svc.SourceAccounts(f.ctx, f.acc)
	for _, s := range srcs {
		if s.ID == ids["tw-tdcc"] && (!s.Ignored || s.Pending != 0) {
			t.Fatalf("source = %+v", s)
		}
	}
}

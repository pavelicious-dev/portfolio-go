package ingest

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portfolio/database"
	"portfolio/models"
)

func TestClassifyCash(t *testing.T) {
	cases := []struct {
		memo   string
		amount float64
		want   models.CashCategory
	}{
		{"CASH RECEIPTS / ELECTRONIC FUND TRANSFERS", 5000, models.CashDeposit},
		{"CASH RECEIPTS / ELECTRONIC FUND TRANSFERS", -300, models.CashWithdrawal},
		{"DISBURSEMENT INITIATED BY Some Name", -1000, models.CashWithdrawal},
		{"BABA(US01609W1027) CASH DIVIDEND USD 1.00 PER SHARE (Ordinary Dividend)", 166, models.CashDividend},
		{"YMAX(US88636J6597) PAYMENT IN LIEU OF DIVIDEND (Ordinary Dividend)", 28, models.CashDividend},
		{"QQQ(US46090E1038) PAYMENT IN LIEU OF DIVIDEND (Ordinary Dividend)", -481, models.CashDividend},
		{"YMAX(US88636J6597) CASH DIVIDEND USD 0.1469 PER SHARE - US TAX", -10, models.CashDividendTax},
		{"YMAX(US88636J6597) PAYMENT IN LIEU OF DIVIDEND - US TAX", -4, models.CashDividendTax},
		{"BABA(US01609W1027) CASH DIVIDEND USD 1.00 PER SHARE - FEE", -3, models.CashFee},
		{"NIO(62914V106) ADR Fee USD 0.02 PER SHARE - FEE", -15, models.CashFee},
		{"INTERDEPOT SETTLEMENT FEE FOR: INVITAE C FOR NOV 2022", -10, models.CashFee},
		{"ADJUSTMENT: COMMISSION", -0.6, models.CashFee},
		{"ADJUSTMENT: OTHER", 0.13, models.CashFee},
		{"USD CREDIT INT FOR OCT-2022", 4.9, models.CashInterest},
		{"USD DEBIT INT FOR SEP-2022", -17.8, models.CashInterest},
		{"USD SHORT CREDIT INTEREST FOR SEP-2025", 7.8, models.CashInterest},
		{"USD IBKR MANAGED SECURITIES (SYEP) INTEREST FOR OCT-2022", 0.05, models.CashInterest},
		{"USD BORROW FEES FOR OCT-2022", -0.16, models.CashBorrowFee},
		{"WITHHOLDING @ 20% ON CREDIT INT FOR NOV-2022", -1.19, models.CashInterestTax},
		{"WITHHOLDING @ 20% ON SHRT CREDIT INT FOR SEP-2025", -1.5, models.CashInterestTax},
		{"V*******86:OPRA NP L1 FOR SEP 2022", -1.51, models.CashDataFee},
		{"V*******86:SNAPSHOTVALUENONPRO FOR OCT 2022", 9.92, models.CashDataFee},
		{"CANCEL[V*******86:OPRA NP L1] FOR OCT 2025", 1.28, models.CashDataFee},
		{"CASH TRADE: EUR.USD", 28495, models.CashFXConversion},
	}
	for _, c := range cases {
		got, ok := classifyCash(c.memo, c.amount)
		if !ok || got != c.want {
			t.Errorf("%q: %v %v, want %v", c.memo, got, ok, c.want)
		}
	}

	if _, ok := classifyCash("SOMETHING NEW AND UNKNOWN", 1); ok {
		t.Error("unknown memo must not be classified")
	}
}

func TestSymbolFromMemo(t *testing.T) {
	for memo, want := range map[string]string{
		"BABA(US01609W1027) CASH DIVIDEND USD 1.00 PER SHARE": "BABA",
		"BRK.B(US0846707026) CASH DIVIDEND":                   "BRK.B",
		"USD CREDIT INT FOR OCT-2022":                         "",
		"CASH RECEIPTS / ELECTRONIC FUND TRANSFERS":           "",
	} {
		if got := symbolFromMemo(memo); got != want {
			t.Errorf("%q: %q, want %q", memo, got, want)
		}
	}
}

const cashSample = `<OFX>
<INVSTMTMSGSRSV1><INVSTMTTRNRS><INVSTMTRS>
<CURDEF>EUR</CURDEF>
<INVACCTFROM><ACCTID>U1</ACCTID></INVACCTFROM>
<INVTRANLIST>
<INVBANKTRAN><STMTTRN><TRNTYPE>DEP</TRNTYPE><DTPOSTED>20250707202000.000[-4:EDT]</DTPOSTED><TRNAMT>2000</TRNAMT><FITID>d1</FITID>
<MEMO>CASH RECEIPTS / ELECTRONIC FUND TRANSFERS</MEMO><CURRENCY><CURRATE>1.0</CURRATE><CURSYM>EUR</CURSYM></CURRENCY></STMTTRN></INVBANKTRAN>
<INVBANKTRAN><STMTTRN><TRNTYPE>INT</TRNTYPE><DTPOSTED>20250703202000.000[-4:EDT]</DTPOSTED><TRNAMT>-17.5</TRNAMT><FITID>i1</FITID>
<MEMO>USD DEBIT INT FOR JUN-2025</MEMO><CURRENCY><CURRATE>0.85</CURRATE><CURSYM>USD</CURSYM></CURRENCY></STMTTRN></INVBANKTRAN>
<INVBANKTRAN><STMTTRN><TRNTYPE>OTHER</TRNTYPE><DTPOSTED>20250703202000.000[-4:EDT]</DTPOSTED><TRNAMT>1</TRNAMT><FITID>x1</FITID>
<MEMO>A BOOKING NOBODY KNOWS</MEMO></STMTTRN></INVBANKTRAN>
<INCOME><INVTRAN><FITID>div1</FITID><DTTRADE>20250124202000.000[-5:EST]</DTTRADE><MEMO>YMAX(US88636J6597) CASH DIVIDEND USD 0.172 PER SHARE (Ordinary Dividend)</MEMO></INVTRAN>
<SECID><UNIQUEID>88636J659</UNIQUEID></SECID><INCOMETYPE>DIV</INCOMETYPE><TOTAL>81.95</TOTAL><CURRENCY><CURRATE>0.9529</CURRATE><CURSYM>USD</CURSYM></CURRENCY></INCOME>
<INCOME><INVTRAN><FITID>misc1</FITID><DTTRADE>20250812202000.000[-4:EDT]</DTTRADE><MEMO>FUTURE TRADE PR: 5532.3511</MEMO></INVTRAN>
<SECID><UNIQUEID>711280067</UNIQUEID></SECID><INCOMETYPE>MISC</INCOMETYPE><TOTAL>42.29</TOTAL><CURRENCY><CURRATE>0.8566</CURRATE><CURSYM>USD</CURSYM></CURRENCY></INCOME>
<INVEXPENSE><INVTRAN><FITID>exp1</FITID><DTTRADE>20240125121202.000[-5:EST]</DTTRADE><MEMO>CFD TRADE PR: 394.04</MEMO></INVTRAN>
<SECID><UNIQUEID>134771477</UNIQUEID></SECID><TOTAL>-0.92</TOTAL><CURRENCY><CURRATE>0.922</CURRATE><CURSYM>USD</CURSYM></CURRENCY></INVEXPENSE>
<BUYOTHER><INVBUY><INVTRAN><FITID>cfd1</FITID><DTTRADE>20240125121202.000[-5:EST]</DTTRADE></INVTRAN><SECID><UNIQUEID>134771477</UNIQUEID></SECID><UNITS>5</UNITS><UNITPRICE>394</UNITPRICE><COMMISSION>0.92</COMMISSION><TOTAL>0.00</TOTAL></INVBUY></BUYOTHER>
<BUYDEBT><INVBUY><INVTRAN><FITID>b1</FITID><DTTRADE>20250714202000.000[-4:EDT]</DTTRADE></INVTRAN><SECID><UNIQUEID>912797PG6</UNIQUEID></SECID>
<UNITS>20000</UNITS><UNITPRICE>85.5688392</UNITPRICE><COMMISSION>4.28645</COMMISSION><TOTAL>-17118.05428923</TOTAL><CURRENCY><CURRATE>0.8573</CURRATE><CURSYM>USD</CURSYM></CURRENCY></INVBUY></BUYDEBT>
<SELLDEBT><INVSELL><INVTRAN><FITID>s1</FITID><DTTRADE>20250730202000.000[-4:EDT]</DTTRADE><MEMO>(US912797PG65) TBILL MATURITY (912797PG6, B 07/31/25, US912797PG65)</MEMO></INVTRAN>
<SECID><UNIQUEID>912797PG6</UNIQUEID></SECID><UNITS>-20000.0</UNITS><UNITPRICE>0.87601</UNITPRICE><TOTAL>17520.2</TOTAL><CURRENCY><CURRATE>0.876</CURRATE><CURSYM>USD</CURSYM></CURRENCY></INVSELL><SELLREASON>MATURITY</SELLREASON></SELLDEBT>
<SELLDEBT><INVSELL><INVTRAN><FITID>s2</FITID><DTTRADE>20250918202000.000[-4:EDT]</DTTRADE></INVTRAN>
<SECID><UNIQUEID>912797PG6</UNIQUEID></SECID><UNITS>-35000</UNITS><UNITPRICE>84.78829287</UNITPRICE><COMMISSION>4.2422</COMMISSION><TOTAL>29671.66030518</TOTAL></INVSELL><SELLREASON>SELL</SELLREASON></SELLDEBT>
</INVTRANLIST>
</INVSTMTRS></INVSTMTTRNRS></INVSTMTMSGSRSV1>
<SECLISTMSGSRSV1><SECLIST>
<DEBTINFO><SECINFO><SECID><UNIQUEID>912797PG6</UNIQUEID></SECID><SECNAME>912797PG6 B 07/31/25</SECNAME><TICKER>912797PG6</TICKER><FIID>757830649</FIID></SECINFO></DEBTINFO>
</SECLIST></SECLISTMSGSRSV1>
</OFX>`

func TestParseCashEvents(t *testing.T) {
	file, err := ParseQFX(strings.NewReader(cashSample))
	if err != nil {
		t.Fatal(err)
	}

	if len(file.CashEvents) != 3 {
		t.Fatalf("%d cash events: %+v", len(file.CashEvents), file.CashEvents)
	}
	byID := map[string]models.CashEvent{}
	for _, e := range file.CashEvents {
		byID[e.SourceID] = e
	}

	if d := byID["d1"]; d.Category != models.CashDeposit || d.Amount.Float() != 2000 || d.Currency != "EUR" || d.FxToBase != 1 {
		t.Errorf("deposit: %+v", d)
	}
	if i := byID["i1"]; i.Category != models.CashInterest || i.Currency != "USD" || i.FxToBase != 0.85 || i.AmountBase() != -17.5*0.85 {
		t.Errorf("interest: %+v", i)
	}
	div := byID["div1"]
	if div.Category != models.CashDividend || div.Symbol != "YMAX" || div.Amount.Float() != 81.95 {
		t.Errorf("dividend: %+v", div)
	}
	if !div.At.Equal(time.Date(2025, 1, 25, 1, 20, 0, 0, time.UTC)) {
		t.Errorf("dividend time: %v", div.At)
	}

	// the unknown booking is reported, futures and CFD records are counted
	if len(file.Skipped) != 1 || !strings.Contains(file.Skipped[0], "NOBODY KNOWS") {
		t.Errorf("skipped: %v", file.Skipped)
	}
	if file.Ignored["futures and CFD settlements"] != 2 || file.Ignored["futures, CFD and currency trades"] != 1 {
		t.Errorf("ignored: %v", file.Ignored)
	}
}

func TestParseTBills(t *testing.T) {
	file, err := ParseQFX(strings.NewReader(cashSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Trades) != 3 {
		t.Fatalf("%d trades", len(file.Trades))
	}
	buy, maturity, sale := file.Trades[0], file.Trades[1], file.Trades[2]

	in := buy.Instrument
	if in.Type != models.Bond || in.Symbol != "912797PG6" || in.ConID != "757830649" || in.Currency != "EUR" ||
		in.Expiry.Format(time.DateOnly) != "2025-07-31" {
		t.Errorf("instrument: %+v", in)
	}

	// the cash flow of each trade equals the broker's TOTAL, which is in EUR
	for name, c := range map[string]struct {
		trade models.Trade
		total float64
	}{
		"buy":      {buy, -17118.05428923},
		"maturity": {maturity, 17520.2},
		"sale":     {sale, 29671.66030518},
	} {
		if got := c.trade.Cash().Float(); got < c.total-0.02 || got > c.total+0.02 {
			t.Errorf("%s: cash %v, want %v", name, got, c.total)
		}
		if c.trade.FxToBase != 1 {
			t.Errorf("%s: amounts are in base currency, rate = %v", name, c.trade.FxToBase)
		}
	}

	if buy.Quantity.Float() != 20000 || buy.Effect != models.EffectOpen || buy.Commission.Float() != 4.28645 {
		t.Errorf("buy: %+v", buy)
	}
	if maturity.Kind != models.KindExpire || maturity.Quantity.Float() != -20000 || maturity.Effect != models.EffectClose {
		t.Errorf("maturity: %+v", maturity)
	}
	if sale.Kind != models.KindTrade || sale.Quantity.Float() != -35000 {
		t.Errorf("sale: %+v", sale)
	}
}

func TestImportStoresCashEventsAndBonds(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(filepath.Join(t.TempDir(), "cash.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	file, err := ParseQFX(strings.NewReader(cashSample))
	if err != nil {
		t.Fatal(err)
	}
	data := database.ImportData{Trades: file.Trades, CashEvents: file.CashEvents}

	for run, want := range []database.ImportResult{{Trades: 3, CashEvents: 3}, {}} { // the second import is a no-op
		got, err := db.Import(ctx, file.Account, "EUR", data)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("run %d: %+v, want %+v", run, got, want)
		}
	}

	events, err := db.GetCashEvents(ctx)
	if err != nil || len(events) != 3 {
		t.Fatalf("%d events, %v", len(events), err)
	}
	for _, e := range events { // oldest first
		if e.ID == 0 || e.Memo == "" || e.Currency == "" {
			t.Errorf("incomplete: %+v", e)
		}
	}
	if !events[0].At.Before(events[2].At) {
		t.Errorf("not ordered by time: %v ... %v", events[0].At, events[2].At)
	}

	trades, err := db.GetTrades(ctx, database.TradeQuery{})
	if err != nil || len(trades) != 3 || trades[0].Instrument.Type != models.Bond {
		t.Fatalf("%d trades, %v", len(trades), err)
	}
}

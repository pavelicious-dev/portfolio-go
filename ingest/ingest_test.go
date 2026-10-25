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

const qfxSample = `OFXHEADER:100
DATA:OFXSGML

<OFX>
<INVSTMTMSGSRSV1><INVSTMTTRNRS><INVSTMTRS>
<CURDEF>EUR</CURDEF>
<INVACCTFROM><ACCTID>U1234567</ACCTID></INVACCTFROM>
<INVTRANLIST>
<BUYSTOCK><INVBUY><INVTRAN><FITID>100</FITID><DTTRADE>20220922141625.000[-4:EDT]</DTTRADE></INVTRAN>
<SECID><UNIQUEID>S1</UNIQUEID></SECID><UNITS>100</UNITS><UNITPRICE>18.22</UNITPRICE><COMMISSION>1.0407</COMMISSION>
<CURRENCY><CURRATE>1.0407</CURRATE><CURSYM>USD</CURSYM></CURRENCY></INVBUY></BUYSTOCK>
<SELLOPT><INVSELL><INVTRAN><FITID>101</FITID><DTTRADE>20221223110909</DTTRADE></INVTRAN>
<SECID><UNIQUEID>O1</UNIQUEID></SECID><UNITS>-2</UNITS><UNITPRICE>0.75</UNITPRICE><COMMISSION>1.26</COMMISSION>
<CURRENCY><CURRATE>0.9</CURRATE><CURSYM>USD</CURSYM></CURRENCY></INVSELL>
<SHPERCTRCT>100</SHPERCTRCT></SELLOPT>
</INVTRANLIST>
</INVSTMTRS></INVSTMTTRNRS></INVSTMTMSGSRSV1>
<SECLISTMSGSRSV1><SECLIST>
<STOCKINFO><SECINFO><SECID><UNIQUEID>S1</UNIQUEID></SECID><SECNAME>ABC INC</SECNAME><TICKER>ABC</TICKER></SECINFO></STOCKINFO>
<OPTINFO><SECINFO><SECID><UNIQUEID>O1</UNIQUEID></SECID><SECNAME>XYZ 20JAN23 145 C</SECNAME><TICKER>XYZ   230120C00145000</TICKER></SECINFO>
<OPTTYPE>CALL</OPTTYPE><STRIKEPRICE>145</STRIKEPRICE><DTEXPIRE>20230120000000.000[-5:EST]</DTEXPIRE><SHPERCTRCT>100</SHPERCTRCT></OPTINFO>
</SECLIST></SECLISTMSGSRSV1>
</OFX>
`

func TestParseQFX(t *testing.T) {
	file, err := ParseQFX(strings.NewReader(qfxSample))
	if err != nil {
		t.Fatal(err)
	}
	if file.Account != "U1234567" || len(file.Trades) != 2 {
		t.Fatalf("%+v", file)
	}

	stock, opt := file.Trades[0], file.Trades[1]
	if stock.Instrument.Symbol != "ABC" || stock.Quantity.Float() != 100 || stock.FxToBase != 1.0407 {
		t.Errorf("stock: %+v", stock)
	}
	if stock.TradedAt.UTC().Format(time.RFC3339) != "2022-09-22T18:16:25Z" {
		t.Errorf("stock time: %v", stock.TradedAt)
	}
	if cash := stock.Cash().Float(); cash != -1823 {
		t.Errorf("stock cash = %v", cash)
	}

	if opt.Quantity.Float() != -2 || opt.Instrument.Multiplier != 100 ||
		opt.Instrument.Symbol != "XYZ   230120C00145000" || opt.FxToBase != 0.9 {
		t.Errorf("option: %+v", opt)
	}
	if opt.Instrument.Right != models.Call || opt.Instrument.Strike.Float() != 145 ||
		opt.Instrument.Expiry.Format(time.DateOnly) != "2023-01-20" {
		t.Errorf("option instrument: %+v", opt.Instrument)
	}
	if cash := opt.Cash().Float(); cash != 148.6 {
		t.Errorf("option cash = %v", cash)
	}
}

func TestImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	file, err := ParseQFX(strings.NewReader(qfxSample))
	if err != nil {
		t.Fatal(err)
	}

	for run, want := range []int{2, 0} { // second import must be a no-op
		got, err := db.ImportTrades(ctx, file.Account, "EUR", file.Trades)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("run %d: inserted %d, want %d", run, got, want)
		}
	}

	all, err := db.GetTrades(ctx, database.TradeQuery{})
	if err != nil || len(all) != 2 {
		t.Fatalf("%d trades, err %v", len(all), err)
	}
	if all[0].Cash() != file.Trades[0].Cash() || !all[1].Instrument.Expiry.Equal(file.Trades[1].Instrument.Expiry) {
		t.Errorf("round trip mismatch: %+v", all[1])
	}

	var name string
	var qty, cash float64
	err = db.QueryRow(`SELECT instrument, quantity, cash FROM trades_readable WHERE type = 'OPT'`).Scan(&name, &qty, &cash)
	if err != nil || name != "XYZ 2023-01-20 145 C" || qty != -2 || cash < 148.5999 || cash > 148.6001 {
		t.Errorf("view: %q %v %v err %v", name, qty, cash, err)
	}

	byFilter, _ := db.GetTrades(ctx, database.TradeQuery{Underlying: "xyz", Year: 2022, Month: 12})
	if len(byFilter) != 1 {
		t.Errorf("filter returned %d", len(byFilter))
	}
}

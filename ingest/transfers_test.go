package ingest

import (
	"fmt"
	"strings"
	"testing"

	"portfolio/models"
)

// transfers as they appear in IB statements, with their securities
const transferSample = `<OFX>
<INVSTMTMSGSRSV1><INVSTMTTRNRS><INVSTMTRS>
<CURDEF>EUR</CURDEF>
<INVACCTFROM><ACCTID>U1</ACCTID></INVACCTFROM>
<INVTRANLIST>
%s
</INVTRANLIST>
</INVSTMTRS></INVSTMTTRNRS></INVSTMTMSGSRSV1>
<SECLISTMSGSRSV1><SECLIST>
<STOCKINFO><SECINFO><SECID><UNIQUEID>654110105</UNIQUEID></SECID><SECNAME>NKLA.OLD</SECNAME><TICKER>NKLA.OLD</TICKER><FIID>425175609</FIID></SECINFO></STOCKINFO>
<STOCKINFO><SECINFO><SECID><UNIQUEID>654110303</UNIQUEID></SECID><SECNAME>NKLA</SECNAME><TICKER>NKLA</TICKER><FIID>711475297</FIID></SECINFO></STOCKINFO>
<STOCKINFO><SECINFO><SECID><UNIQUEID>US46185L1035</UNIQUEID></SECID><SECNAME>IV8</SECNAME><TICKER>IV8</TICKER><FIID>444221340</FIID></SECINFO></STOCKINFO>
<STOCKINFO><SECINFO><SECID><UNIQUEID>46185L103</UNIQUEID></SECID><SECNAME>NVTA</SECNAME><TICKER>NVTA</TICKER><FIID>183455515</FIID></SECINFO></STOCKINFO>
<OPTINFO><SECINFO><SECID><UNIQUEID>682464810</UNIQUEID></SECID><SECNAME>NVDA  240719C00136000 NVDA 19JUL24 136 C</SECNAME><TICKER>NVDA  240719C00136000</TICKER><FIID>682464810</FIID></SECINFO>
<OPTTYPE>CALL</OPTTYPE><STRIKEPRICE>136</STRIKEPRICE><DTEXPIRE>20240719</DTEXPIRE><SHPERCTRCT>100</SHPERCTRCT></OPTINFO>
</SECLIST></SECLISTMSGSRSV1>
</OFX>`

func record(fitid, date, memo, secid string, units float64, action string, price float64) string {
	return fmt.Sprintf(`<TRANSFER><INVTRAN><FITID>%s</FITID><DTTRADE>%s000000.000[-4:EDT]</DTTRADE><MEMO>%s</MEMO></INVTRAN>
<SECID><UNIQUEID>%s</UNIQUEID></SECID><SUBACCTSEC>CASH</SUBACCTSEC><UNITS>%v</UNITS><TFERACTION>%s</TFERACTION><POSTYPE>LONG</POSTYPE><UNITPRICE>%v</UNITPRICE></TRANSFER>`,
		fitid, date, memo, secid, units, action, price)
}

func parseTransfers(t *testing.T, records ...string) *QFXFile {
	t.Helper()
	file, err := ParseQFX(strings.NewReader(fmt.Sprintf(transferSample, strings.Join(records, "\n"))))
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func TestSplitRatio(t *testing.T) {
	for memo, want := range map[string]float64{
		"NVDA(US67066G1040) SPLIT 10 FOR 1 (NVDA  240719C00136000, NVDA 19JUL24 136 C, )": 10,
		"CYN.OLD(US23257B2060) SPLIT 1 FOR 150 (CYN, CYNGN INC, US23257B3050)":            1.0 / 150,
	} {
		if got, ok := splitRatio(memo); !ok || got != want {
			t.Errorf("%q: %v %v, want %v", memo, got, ok, want)
		}
	}
	if _, ok := splitRatio("DELISTED (NVTAQ.OLD)"); ok {
		t.Error("delisting is not a split")
	}
}

func TestOptionSplitScalesInPlace(t *testing.T) {
	file := parseTransfers(t, record("682464810.682464810", "20240607",
		"NVDA(US67066G1040) SPLIT 10 FOR 1 (NVDA  240719C00136000, NVDA 19JUL24 136 C, )", "682464810", -9, "OUT", 0))

	if len(file.Actions) != 1 || len(file.Trades) != 0 || len(file.Skipped) != 0 {
		t.Fatalf("%+v", file)
	}
	a := file.Actions[0]
	if a.Kind != models.ActionScale || a.Ratio != 10 || a.From.ConID != "682464810" || a.From.Type != models.Option {
		t.Errorf("%+v", a)
	}
}

func TestReverseSplitConvertsToNewInstrument(t *testing.T) {
	memo := "NKLA(US6541101050) SPLIT 1 FOR 30 (%s, NIKOLA CORP, US6541103031)"
	file := parseTransfers(t,
		record("US6541103031.711475297", "20240624", fmt.Sprintf(memo, "NKLA"), "654110303", 10, "IN", 0),
		record("US6541101050.425175609", "20240624", fmt.Sprintf(memo, "NKLA.OLD"), "654110105", -300, "OUT", 0),
	)

	if len(file.Actions) != 1 || len(file.Skipped) != 0 {
		t.Fatalf("%+v", file)
	}
	a := file.Actions[0]
	if a.Kind != models.ActionConvert || a.From.ConID != "425175609" || a.To.ConID != "711475297" {
		t.Errorf("%+v", a)
	}
	if want := 10.0 / 300; a.Ratio < want-1e-12 || a.Ratio > want+1e-12 {
		t.Errorf("ratio = %v", a.Ratio)
	}
}

func TestDelistingWritesOffPosition(t *testing.T) {
	file := parseTransfers(t, record("US46185L1035.183455515", "20240814",
		"(US46185L1035) DELISTED (NVTAQ.OLD, INVITAE CORP, US46185L1035)", "46185L103", -15000, "OUT", 0))

	if len(file.Trades) != 1 || len(file.Actions) != 0 {
		t.Fatalf("%+v", file)
	}
	tr := file.Trades[0]
	if tr.Kind != models.KindExpire || tr.Effect != models.EffectClose || tr.Quantity.Float() != -15000 || tr.Price != 0 || tr.Instrument.ConID != "183455515" {
		t.Errorf("%+v", tr)
	}
}

func TestTransferInOpensPositionAtGivenPrice(t *testing.T) {
	file := parseTransfers(t, record("444221340", "20221004",
		"FOP Transfer In From Account 01 586 062 01", "US46185L1035", 2500, "IN", 2.404))

	if len(file.Trades) != 1 {
		t.Fatalf("%+v", file)
	}
	tr := file.Trades[0]
	if tr.Quantity.Float() != 2500 || tr.Price.Float() != 2.404 || tr.Effect != models.EffectOpen || tr.Memo == "" {
		t.Errorf("%+v", tr)
	}
}

func TestInterDepotTransferKeepsUnits(t *testing.T) {
	memo := "InterDepot Transfer In From Account U1"
	file := parseTransfers(t,
		record("444221340", "20221101", memo, "US46185L1035", -2500, "OUT", 2.578), // same FITID as the earlier IN
		record("183455515", "20221101", memo, "46185L103", 2500, "IN", 2.646),
	)

	if len(file.Actions) != 1 || len(file.Trades) != 0 {
		t.Fatalf("%+v", file)
	}
	a := file.Actions[0]
	if a.Kind != models.ActionConvert || a.Ratio != 1 || a.From.Symbol != "IV8" || a.To.Symbol != "NVTA" {
		t.Errorf("%+v", a)
	}
}

func TestUnknownTransfersAreReported(t *testing.T) {
	file := parseTransfers(t,
		record("1", "20250902", "(US912797QM25) TBILL MATURITY (912797QM2, B 09/02/25, US912797QM25)", "912797QM2", -20000, "OUT", 0),
		record("2", "20250903", "SOMETHING NEW", "46185L103", 5, "IN", 0),
	)

	if len(file.Skipped) != 2 || len(file.Trades) != 0 || len(file.Actions) != 0 {
		t.Errorf("%+v", file)
	}
}

func TestTransferSourceIdsAreUnique(t *testing.T) {
	file := parseTransfers(t,
		record("444221340", "20221004", "FOP Transfer In From Account X", "US46185L1035", 2500, "IN", 2.404),
		record("444221340", "20221101", "InterDepot Transfer In From Account U1", "US46185L1035", -2500, "OUT", 2.578),
		record("183455515", "20221101", "InterDepot Transfer In From Account U1", "46185L103", 2500, "IN", 2.646),
	)
	if len(file.Trades) != 1 || len(file.Actions) != 1 || file.Trades[0].SourceID == file.Actions[0].SourceID {
		t.Errorf("%+v", file)
	}
}

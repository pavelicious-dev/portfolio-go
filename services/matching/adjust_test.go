package matching

import (
	"testing"

	"portfolio/models"
)

func action(kind models.ActionKind, day int, from, to models.Instrument, ratio float64) models.CorporateAction {
	return models.CorporateAction{AccountID: 1, Kind: kind, At: day0.AddDate(0, 0, day), From: from, To: to, Ratio: ratio}
}

func TestSplitOfOptionContract(t *testing.T) {
	// short 1 contract at 10.00, a 10 for 1 split, then 10 contracts bought back at 0.50
	trades := []models.Trade{
		trade(option, 0, -1, 10, 0),
		trade(option, 10, 10, 0.5, 0),
	}
	trades = Adjust(trades, []models.CorporateAction{action(models.ActionScale, 5, option, models.Instrument{}, 10)})

	if trades[0].Quantity.Float() != -10 || trades[0].Price.Float() != 1 {
		t.Fatalf("restated trade: %+v", trades[0])
	}
	if trades[1].Quantity.Float() != 10 || trades[1].Price.Float() != 0.5 {
		t.Fatalf("trade after the split changed: %+v", trades[1])
	}

	res := Match(trades)
	if len(res.Unmatched) != 0 || len(res.RoundTrips) != 1 || res.RoundTrips[0].IsOpen() {
		t.Fatalf("%+v", res)
	}
	// 1000 received, 500 paid: the split does not change the profit
	near(t, "pnl", res.RoundTrips[0].RealizedPnL().Float(), 500)
}

func TestAdjustDoesNotModifyInput(t *testing.T) {
	trades := []models.Trade{trade(stock, 0, 100, 10, 0)}
	Adjust(trades, []models.CorporateAction{action(models.ActionScale, 5, stock, models.Instrument{}, 2)})

	if trades[0].Quantity.Float() != 100 {
		t.Errorf("input changed: %+v", trades[0])
	}
}

func TestReverseSplitMovesPositionToNewInstrument(t *testing.T) {
	newStock := models.Instrument{ID: 9, ConID: "N", Type: models.Stock, Symbol: "NEW", Underlying: "NEW", Currency: "USD", Multiplier: 1}

	trades := []models.Trade{
		trade(stock, 0, 200, 9, 0),
		trade(stock, 1, 100, 9, 0),
		trade(newStock, 10, -10, 300, 0),
	}
	trades = Adjust(trades, []models.CorporateAction{action(models.ActionConvert, 5, stock, newStock, 1.0/30)})

	res := Match(trades)
	if len(res.Unmatched) != 0 || len(res.RoundTrips) != 1 {
		t.Fatalf("%+v", res)
	}
	rt := res.RoundTrips[0]
	if rt.IsOpen() || rt.Instrument.Symbol != "NEW" || len(rt.Matches) != 2 {
		t.Errorf("%+v", rt)
	}
	near(t, "pnl", rt.RealizedPnL().Float(), 3000-2700)
}

func TestRoundingDustAfterSplitDoesNotKeepPositionOpen(t *testing.T) {
	// 10 + 10 shares at a 1 for 3 reverse split are 3.333333 + 3.333333 = 6.666666,
	// while the whole position is later sold as 6.666667
	trades := []models.Trade{trade(stock, 0, 10, 9, 0), trade(stock, 1, 10, 9, 0), trade(stock, 10, -6.666667, 40, 0)}
	trades = Adjust(trades, []models.CorporateAction{action(models.ActionScale, 5, stock, models.Instrument{}, 1.0/3)})

	res := Match(trades)
	if len(res.Unmatched) != 0 || len(res.RoundTrips) != 1 || res.RoundTrips[0].IsOpen() {
		t.Errorf("%+v", res)
	}
}

func TestWriteOffClosesPositionAtZero(t *testing.T) {
	writeOff := trade(stock, 10, -100, 0, 0)
	writeOff.Kind = models.KindExpire
	writeOff.Effect = models.EffectClose

	res := Match([]models.Trade{trade(stock, 0, 100, 5, 0), writeOff})
	if len(res.RoundTrips) != 1 || res.RoundTrips[0].IsOpen() {
		t.Fatalf("%+v", res.RoundTrips)
	}
	near(t, "loss", res.RoundTrips[0].RealizedPnL().Float(), -500)
}

func TestActionsOnlyAffectTheirAccountAndEarlierTrades(t *testing.T) {
	other := trade(stock, 0, 100, 10, 0)
	other.AccountID = 2

	trades := Adjust([]models.Trade{other, trade(stock, 6, 100, 10, 0)},
		[]models.CorporateAction{action(models.ActionScale, 5, stock, models.Instrument{}, 2)})

	if trades[0].Quantity.Float() != 100 || trades[1].Quantity.Float() != 100 {
		t.Errorf("%+v", trades)
	}
}

func TestChainedConversions(t *testing.T) {
	b := models.Instrument{ID: 20, Type: models.Stock, Symbol: "B", Underlying: "B", Currency: "USD", Multiplier: 1}
	c := models.Instrument{ID: 30, Type: models.Stock, Symbol: "C", Underlying: "C", Currency: "USD", Multiplier: 1}

	trades := Adjust([]models.Trade{trade(stock, 0, 100, 10, 0)}, []models.CorporateAction{
		action(models.ActionConvert, 20, b, c, 1), // listed first, happens later
		action(models.ActionConvert, 10, stock, b, 1),
	})
	if trades[0].Instrument.Symbol != "C" {
		t.Errorf("%+v", trades[0].Instrument)
	}
}

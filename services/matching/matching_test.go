package matching

import (
	"math"
	"testing"
	"time"

	"portfolio/models"
)

var (
	stock  = models.Instrument{ID: 1, Type: models.Stock, Symbol: "ABC", Underlying: "ABC", Currency: "USD", Multiplier: 1}
	option = models.Instrument{ID: 2, Type: models.Option, Symbol: "ABC   240119C00010000", Underlying: "ABC", Currency: "USD", Multiplier: 100}
	day0   = time.Date(2024, 1, 2, 15, 0, 0, 0, time.UTC)
)

var nextID int64

func trade(in models.Instrument, day int, qty, price, fee float64) models.Trade {
	nextID++
	return models.Trade{
		ID:         nextID,
		AccountID:  1,
		Instrument: in,
		TradedAt:   day0.AddDate(0, 0, day),
		Quantity:   models.FixedFromFloat(qty),
		Price:      models.FixedFromFloat(price),
		Commission: models.FixedFromFloat(fee),
		FxToBase:   1,
	}
}

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestSimpleLong(t *testing.T) {
	res := Match([]models.Trade{trade(stock, 0, 100, 10, 1), trade(stock, 1, -100, 12, 1)})

	if len(res.RoundTrips) != 1 {
		t.Fatalf("%d round trips", len(res.RoundTrips))
	}
	rt := res.RoundTrips[0]
	if rt.IsOpen() || rt.Direction != models.Long || rt.ClosedAt.IsZero() || rt.MaxQuantity.Float() != 100 {
		t.Errorf("%+v", rt)
	}
	near(t, "pnl", rt.RealizedPnL().Float(), 198)
	near(t, "fees", rt.Fees().Float(), 2)
	near(t, "base", rt.RealizedBase(), 198)
}

func TestFIFOPartialClose(t *testing.T) {
	first := trade(stock, 0, 100, 10, 2)
	second := trade(stock, 1, 100, 20, 0)
	sell := trade(stock, 2, -150, 25, 3)

	res := Match([]models.Trade{second, sell, first}) // unordered input

	if len(res.RoundTrips) != 1 {
		t.Fatalf("%d round trips", len(res.RoundTrips))
	}
	rt := res.RoundTrips[0]
	if !rt.IsOpen() || rt.OpenQuantity.Float() != 50 || rt.MaxQuantity.Float() != 200 {
		t.Errorf("open %v max %v", rt.OpenQuantity, rt.MaxQuantity)
	}
	if len(rt.Matches) != 2 {
		t.Fatalf("%d matches", len(rt.Matches))
	}
	// 100 @10 -> 25: 1500 less all of 2 and 2/3 of... 100/150 of the sell's fee 3
	near(t, "m0", rt.Matches[0].PnL.Float(), 1500-2-2)
	// 50 @20 -> 25: 250 less 50/150 of the sell's fee
	near(t, "m1", rt.Matches[1].PnL.Float(), 250-1)
	if rt.Matches[0].OpenTradeID != first.ID || rt.Matches[1].OpenTradeID != second.ID {
		t.Errorf("not first in first out: %+v", rt.Matches)
	}
}

func TestFlipSplitsIntoTwoRoundTrips(t *testing.T) {
	res := Match([]models.Trade{trade(stock, 0, 100, 10, 0), trade(stock, 1, -150, 12, 3)})

	if len(res.RoundTrips) != 2 {
		t.Fatalf("%d round trips", len(res.RoundTrips))
	}
	long, short := res.RoundTrips[0], res.RoundTrips[1]
	if long.IsOpen() || long.Direction != models.Long {
		t.Errorf("long: %+v", long)
	}
	near(t, "long pnl", long.RealizedPnL().Float(), 200-2) // 100/150 of the fee

	if !short.IsOpen() || short.Direction != models.Short || short.OpenQuantity.Float() != 50 || len(short.Matches) != 0 {
		t.Errorf("short: %+v", short)
	}
}

func TestShortOption(t *testing.T) {
	res := Match([]models.Trade{trade(option, 0, -2, 1.0, 1.5), trade(option, 5, 2, 0.4, 1.5)})

	rt := res.RoundTrips[0]
	if rt.Direction != models.Short || rt.IsOpen() {
		t.Fatalf("%+v", rt)
	}
	near(t, "pnl", rt.RealizedPnL().Float(), 120-3) // (1.0-0.4) * 2 * 100
}

func TestReopenCreatesNewRoundTrip(t *testing.T) {
	res := Match([]models.Trade{
		trade(stock, 0, 10, 10, 0), trade(stock, 1, -10, 11, 0),
		trade(stock, 2, 10, 20, 0), trade(stock, 3, -10, 18, 0),
	})
	if len(res.RoundTrips) != 2 {
		t.Fatalf("%d round trips", len(res.RoundTrips))
	}
	near(t, "first", res.RoundTrips[0].RealizedPnL().Float(), 10)
	near(t, "second", res.RoundTrips[1].RealizedPnL().Float(), -20)
	if !res.RoundTrips[1].OpenedAt.After(res.RoundTrips[0].ClosedAt) {
		t.Error("round trips overlap")
	}
}

func TestClosingTradeWithoutOpening(t *testing.T) {
	orphan := trade(stock, 0, 100, 10, 1)
	orphan.Effect = models.EffectClose

	res := Match([]models.Trade{orphan})
	if len(res.RoundTrips) != 0 || len(res.Unmatched) != 1 || res.Unmatched[0].Quantity.Float() != 100 {
		t.Errorf("%+v", res)
	}
}

func TestFlaggedCloseThatCrossesZeroOpensTheOtherSide(t *testing.T) {
	// long 4 contracts, sold 2 + 3 with both fills flagged "to close", the
	// leftover short contract expires worthless (bought back at 0)
	sell1 := trade(option, 1, -2, 0.8, 0)
	sell1.Effect = models.EffectClose
	sell2 := trade(option, 1, -3, 0.85, 0)
	sell2.Effect = models.EffectClose
	expiry := trade(option, 1, 1, 0, 0)
	expiry.TradedAt = expiry.TradedAt.Add(time.Hour)
	expiry.Effect = models.EffectClose

	res := Match([]models.Trade{trade(option, 0, 4, 2, 0), sell1, sell2, expiry})

	if len(res.Unmatched) != 0 {
		t.Fatalf("unmatched: %+v", res.Unmatched)
	}
	if len(res.RoundTrips) != 2 || res.RoundTrips[0].Direction != models.Long || res.RoundTrips[1].Direction != models.Short {
		t.Fatalf("%+v", res.RoundTrips)
	}
	for _, rt := range res.RoundTrips {
		if rt.IsOpen() {
			t.Errorf("still open: %+v", rt)
		}
	}
	// long: 2 sold at 0.8 and 2 at 0.85 against 4 bought at 2.0
	near(t, "long pnl", res.RoundTrips[0].RealizedPnL().Float(), (0.8-2)*2*100+(0.85-2)*2*100)
	// short: 1 sold at 0.85, bought back at 0
	near(t, "short pnl", res.RoundTrips[1].RealizedPnL().Float(), 0.85*100)
}

func TestClosingTradeWithoutAnyPositionStaysUnmatched(t *testing.T) {
	sell := trade(stock, 1, -50, 12, 0)
	sell.Effect = models.EffectClose

	res := Match([]models.Trade{sell})
	if len(res.RoundTrips) != 0 || len(res.Unmatched) != 1 || res.Unmatched[0].Quantity.Float() != 50 {
		t.Errorf("%+v", res)
	}

	// a "close" in the direction of the open position is inconsistent as well
	buy := trade(stock, 2, 10, 12, 0)
	buy.Effect = models.EffectClose
	res = Match([]models.Trade{trade(stock, 0, 100, 10, 0), buy})
	if len(res.Unmatched) != 1 || res.Unmatched[0].Quantity.Float() != 10 || !res.RoundTrips[0].IsOpen() || res.RoundTrips[0].OpenQuantity.Float() != 100 {
		t.Errorf("%+v", res)
	}
}

func TestBaseCurrencyUsesRateOfEachLeg(t *testing.T) {
	buy := trade(stock, 0, 10, 100, 0)
	buy.FxToBase = 0.9
	sell := trade(stock, 1, -10, 110, 0)
	sell.FxToBase = 0.8

	rt := Match([]models.Trade{buy, sell}).RoundTrips[0]
	near(t, "pnl usd", rt.RealizedPnL().Float(), 100)
	near(t, "pnl base", rt.RealizedBase(), -1000*0.9+1100*0.8)
}

func TestInstrumentsAndAccountsAreSeparate(t *testing.T) {
	other := trade(stock, 1, -10, 11, 0)
	other.AccountID = 2

	res := Match([]models.Trade{trade(stock, 0, 10, 10, 0), other, trade(option, 0, 1, 1, 0)})
	if len(res.RoundTrips) != 3 {
		t.Fatalf("%d round trips", len(res.RoundTrips))
	}
	for _, rt := range res.RoundTrips {
		if len(rt.Matches) != 0 {
			t.Errorf("unexpected match: %+v", rt)
		}
	}
}

func TestSameTimestampOpenThenCloseThenUnflagged(t *testing.T) {
	at := func(tr models.Trade, effect models.Effect) models.Trade {
		tr.TradedAt = day0.AddDate(0, 0, 10)
		tr.Effect = effect
		return tr
	}

	// short opened earlier; at one timestamp a new short opens, the old one is
	// covered (flagged) and a long opens through an unflagged exercise buy
	trades := []models.Trade{
		trade(stock, 0, -100, 100, 0),
		at(trade(stock, 0, 100, 90, 0), ""), // unflagged buy, listed first on purpose
		at(trade(stock, 0, 100, 95, 0), models.EffectClose),
		at(trade(stock, 0, -100, 105, 0), models.EffectOpen),
	}
	res := Match(trades)

	if len(res.Unmatched) != 0 {
		t.Fatalf("unmatched: %+v", res.Unmatched)
	}
	if len(res.RoundTrips) != 1 || res.RoundTrips[0].IsOpen() || len(res.RoundTrips[0].Matches) != 2 {
		t.Fatalf("%+v", res.RoundTrips)
	}
	// FIFO: old short covered by the flagged buy @95, new short by the unflagged buy @90
	m := res.RoundTrips[0].Matches
	near(t, "first match", m[0].PnL.Float(), (100-95)*100)
	near(t, "second match", m[1].PnL.Float(), (105-90)*100)
}

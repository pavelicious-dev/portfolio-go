package reports

import (
	"math"
	"testing"
	"time"

	"portfolio/models"
)

var (
	inst = models.Instrument{ID: 1, Type: models.Stock, Symbol: "ABC", Underlying: "ABC", Currency: "USD", Multiplier: 1}
	day  = func(d int) time.Time { return time.Date(2024, 1, 1+d, 15, 0, 0, 0, time.UTC) }
)

func trade(id int64, d int, qty, price float64) models.Trade {
	return models.Trade{
		ID: id, AccountID: 1, Instrument: inst, TradedAt: day(d),
		Quantity: models.FixedFromFloat(qty), Price: models.FixedFromFloat(price), FxToBase: 1,
	}
}

func TestOpenPositionsUsesRemainingLots(t *testing.T) {
	trades := []models.Trade{trade(1, 0, 100, 10), trade(2, 1, 100, 20), trade(3, 2, -150, 25)}
	trips := []models.RoundTrip{{
		AccountID: 1, Instrument: inst, Direction: models.Long, OpenedAt: day(0),
		MaxQuantity: models.FixedFromFloat(200), OpenQuantity: models.FixedFromFloat(50),
		Matches: []models.LotMatch{
			{OpenTradeID: 1, CloseTradeID: 3, Quantity: models.FixedFromFloat(100), PnLBase: 1500},
			{OpenTradeID: 2, CloseTradeID: 3, Quantity: models.FixedFromFloat(50), PnLBase: 250},
		},
	}}

	got := OpenPositions(trips, trades, "")
	if len(got) != 1 {
		t.Fatalf("%d positions", len(got))
	}
	p := got[0]
	// 50 of the second lot remain, bought at 20
	if p.Quantity.Float() != 50 || math.Abs(p.AvgPrice-20) > 1e-9 || math.Abs(p.Cost-1000) > 1e-9 || p.Realized != 1750 {
		t.Errorf("%+v", p)
	}
	if p.DaysOpen(day(10)) != 10 {
		t.Errorf("days open = %d", p.DaysOpen(day(10)))
	}

	if len(OpenPositions(trips, trades, "xyz")) != 0 || len(OpenPositions(trips, trades, "abc")) != 1 {
		t.Error("symbol filter")
	}
}

func TestDaysToExpiry(t *testing.T) {
	p := OpenPosition{Instrument: models.Instrument{Expiry: time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)}}
	if d, ok := p.DaysToExpiry(day(9)); !ok || d != 10 {
		t.Errorf("days = %d, %v", d, ok)
	}
	if _, ok := (OpenPosition{}).DaysToExpiry(day(0)); ok {
		t.Error("stock has no expiry")
	}
}

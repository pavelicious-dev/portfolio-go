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

func closedTrip(id int64, ticker string, opened, closed time.Time, pnl float64) models.RoundTrip {
	in := models.Instrument{ID: id, Type: models.Stock, Symbol: ticker, Underlying: ticker, Currency: "USD", Multiplier: 1}
	return models.RoundTrip{
		ID: id, Instrument: in, Direction: models.Long, OpenedAt: opened, ClosedAt: closed,
		Matches: []models.LotMatch{{OpenedAt: opened, ClosedAt: closed, PnLBase: pnl}},
	}
}

func TestParsePeriod(t *testing.T) {
	for in, want := range map[string]Period{"": {}, "2024": {Year: 2024}, "2024-03": {Year: 2024, Month: 3}} {
		if got, err := ParsePeriod(in); err != nil || got != want {
			t.Errorf("%q: %+v %v", in, got, err)
		}
	}
	for _, bad := range []string{"abc", "2024-13", "2024-00", "24", "2024-3-1"} {
		if _, err := ParsePeriod(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestPeriodContainsUsesReportingZone(t *testing.T) {
	// 01:00 UTC on 1 Feb is still 31 Jan in New York
	late := time.Date(2024, 2, 1, 1, 0, 0, 0, time.UTC)
	if !(Period{2024, 1}).Contains(late) || (Period{2024, 2}).Contains(late) {
		t.Error("month boundary")
	}
	if !(Period{}).Contains(late) || !(Period{Year: 2024}).Contains(late) || (Period{Year: 2023}).Contains(late) {
		t.Error("year and all-time")
	}
}

func TestSummarize(t *testing.T) {
	d := func(m time.Month, day int) time.Time { return time.Date(2024, m, day, 15, 0, 0, 0, time.UTC) }

	option := closedTrip(2, "ABC", d(1, 5), d(2, 5), -50)
	option.Instrument.Type = models.Option
	option.Instrument.Symbol = "ABC 2024-02-16 10 C"

	open := models.RoundTrip{ID: 9, Instrument: models.Instrument{ID: 9, Type: models.Stock, Symbol: "ABC", Underlying: "ABC"},
		OpenedAt: d(3, 1), OpenQuantity: models.FixedFromFloat(10)}

	trips := []models.RoundTrip{
		closedTrip(1, "ABC", d(1, 2), d(1, 12), 200), // +200, 10 days
		option, // -50, 31 days
		closedTrip(3, "ABC", d(2, 1), d(2, 11), 100),  // +100, 10 days
		closedTrip(4, "XYZ", d(1, 2), d(1, 12), 9999), // other ticker
		open,
	}

	s := Summarize(trips, "abc", Period{})
	if s.Realized != 250 || s.Stocks != 300 || s.Options != -50 || s.Trips != 3 || s.Wins != 2 || s.Losses != 1 || s.OpenTrips != 1 || s.Contracts != 3 {
		t.Errorf("%+v", s)
	}
	if s.GrossWin != 300 || s.GrossLoss != -50 || s.ProfitFactor() != 6 || s.AvgWin() != 150 || s.AvgLoss() != -50 {
		t.Errorf("%+v", s)
	}
	if w := s.WinRate(); w < 0.666 || w > 0.667 {
		t.Errorf("win rate %v", w)
	}
	if s.AvgDays < 17 || s.AvgDays > 17.1 { // (10 + 31 + 10) / 3
		t.Errorf("avg days %v", s.AvgDays)
	}
	if len(s.Best) != 2 || s.Best[0].PnL != 200 || len(s.Worst) != 1 || s.Worst[0].PnL != -50 {
		t.Errorf("best %+v worst %+v", s.Best, s.Worst)
	}
	if len(s.Months) != 2 || s.Months[0].Month != "2024-01" || s.Months[0].PnL() != 200 || s.Months[1].PnL() != 50 {
		t.Errorf("months %+v", s.Months)
	}

	feb := Summarize(trips, "ABC", Period{2024, 2})
	if feb.Realized != 50 || feb.Trips != 2 || feb.Wins != 1 || feb.Losses != 1 {
		t.Errorf("february: %+v", feb)
	}
	if none := Summarize(trips, "ABC", Period{Year: 2023}); none.Realized != 0 || none.Trips != 0 {
		t.Errorf("2023: %+v", none)
	}
}

func TestSummarizeAllTickers(t *testing.T) {
	d := func(m time.Month, day int) time.Time { return time.Date(2024, m, day, 15, 0, 0, 0, time.UTC) }

	option := closedTrip(2, "XYZ", d(2, 1), d(2, 20), -30)
	option.Instrument.Type = models.Option

	trips := []models.RoundTrip{
		closedTrip(1, "ABC", d(1, 2), d(1, 12), 200),
		option,
		closedTrip(3, "XYZ", d(2, 1), d(2, 11), 100),
		closedTrip(4, "DEF", d(1, 2), d(1, 12), -80),
		closedTrip(5, "ABC", d(1, 2), time.Date(2023, 12, 12, 15, 0, 0, 0, time.UTC), 999), // other year
	}

	s := Summarize(trips, "", Period{Year: 2024})
	if s.Ticker != "" || s.Realized != 190 || s.Stocks != 220 || s.Options != -30 || s.Trips != 4 || s.Wins != 2 || s.Losses != 2 {
		t.Errorf("%+v", s)
	}

	// ranked by result: ABC 200, XYZ 70, DEF -80
	if len(s.Tickers) != 3 || s.Tickers[0].Asset != "ABC" || s.Tickers[1].Total != 70 || s.Tickers[2].Asset != "DEF" {
		t.Errorf("tickers: %+v", s.Tickers)
	}

	// per month: January 120 stocks, February 100 stocks and -30 options
	if len(s.Months) != 2 || s.Months[0].Stocks != 120 || s.Months[0].PnL() != 120 ||
		s.Months[1].Stocks != 100 || s.Months[1].Options != -30 || s.Months[1].PnL() != 70 {
		t.Errorf("months: %+v", s.Months)
	}

	all := Summarize(trips, "", Period{})
	if all.Realized != 190+999 {
		t.Errorf("all time = %v", all.Realized)
	}
	if jan := Summarize(trips, "", Period{2024, 1}); jan.Realized != 120 || jan.Trips != 2 {
		t.Errorf("january: %+v", jan)
	}
}

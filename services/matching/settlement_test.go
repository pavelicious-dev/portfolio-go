package matching

import (
	"testing"
	"time"

	"portfolio/models"
)

func indexOption(id int64, root string, right models.Right, strike float64, expiry time.Time) models.Instrument {
	return models.Instrument{
		ID: id, Type: models.Option, Symbol: root + " option", Underlying: root, Currency: "USD", Multiplier: 100,
		Expiry: expiry, Strike: models.FixedFromFloat(strike), Right: right,
	}
}

func at(date string, hour, min int) time.Time {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		panic(err)
	}
	// local New York time, as IB books them
	return time.Date(d.Year(), d.Month(), d.Day(), hour, min, 0, 0, newYork)
}

func fill(in models.Instrument, when time.Time, qty, price float64) models.Trade {
	nextID++
	return models.Trade{
		ID: nextID, AccountID: 1, Instrument: in, TradedAt: when, Kind: models.KindTrade,
		Quantity: models.FixedFromFloat(qty), Price: models.FixedFromFloat(price), FxToBase: 1,
	}
}

func price(day string, index string, value float64) models.SettlementPrice {
	d, _ := time.Parse(time.DateOnly, day)
	return models.SettlementPrice{Index: index, Day: d, Price: value}
}

// the four legs of the SPX position that expired on 2025-09-19, as IB booked them
func boxSpread() []models.Trade {
	expiry := time.Date(2025, 9, 19, 0, 0, 0, 0, time.UTC)
	c6150 := indexOption(1, "SPX", models.Call, 6150, expiry)
	c6350 := indexOption(2, "SPX", models.Call, 6350, expiry)
	p6150 := indexOption(3, "SPX", models.Put, 6150, expiry)
	p6350 := indexOption(4, "SPX", models.Put, 6350, expiry)

	open := at("2025-08-01", 12, 14)
	end := at("2025-09-19", 16, 20)
	return []models.Trade{
		fill(c6150, open, -1, 201.723913), fill(c6350, open, 1, 87.213769),
		fill(p6150, open, 1, 80.698195), fill(p6350, open, -1, 137.621015),
		fill(c6150, end, 1, 0), fill(c6350, end, -1, 0),
		fill(p6150, end, -1, 0), fill(p6350, end, 1, 0),
	}
}

func totalPnL(res models.MatchResult) float64 {
	var sum float64
	for _, rt := range res.RoundTrips {
		sum += rt.RealizedPnL().Float()
	}
	return sum
}

func TestSettleBoxSpread(t *testing.T) {
	trades := boxSpread()

	// without a settlement price every leg expires worthless: a profit of the whole premium
	unsettled, report := Settle(trades, nil)
	near(t, "unsettled", totalPnL(Match(unsettled)), 171.432964*100)
	if len(report.Missing) != 1 || report.Missing[0].Index != "SET" || report.Missing[0].Trades != 4 ||
		report.Missing[0].Day.Format(time.DateOnly) != "2025-09-19" || report.MissingTrades() != 4 {
		t.Errorf("missing: %+v", report.Missing)
	}

	// a box spread is worth its width at expiry, whatever the index settles at
	for _, level := range []float64{5900, 6200, 6664.36, 7000} {
		settled, report := Settle(trades, []models.SettlementPrice{price("2025-09-19", "SET", level)})
		res := Match(settled)

		near(t, "box at settlement", totalPnL(res), (171.432964-200)*100)
		if len(report.Missing) != 0 || len(report.Indexes) != 1 || report.Indexes[0].Expiries != 4 {
			t.Errorf("level %v: %+v", level, report)
		}
		if len(res.Unmatched) != 0 {
			t.Errorf("unmatched: %+v", res.Unmatched)
		}
	}

	// the input stays as it was
	for _, tr := range trades[4:] {
		if tr.Price != 0 {
			t.Errorf("input modified: %+v", tr)
		}
	}
}

func TestSettleOneLeg(t *testing.T) {
	expiry := time.Date(2024, 1, 19, 0, 0, 0, 0, time.UTC)
	call := indexOption(1, "SPXW", models.Call, 4780, expiry)
	put := indexOption(2, "SPXW", models.Put, 4780, expiry)

	trades := []models.Trade{
		fill(call, at("2024-01-17", 10, 57), -1, 2.99), // short call, expires in the money
		fill(call, at("2024-01-19", 16, 20), 1, 0),
		fill(put, at("2024-01-17", 10, 57), -1, 4.0), // short put, expires worthless
		fill(put, at("2024-01-19", 16, 20), 1, 0),
	}
	settled, report := Settle(trades, []models.SettlementPrice{price("2024-01-19", "SPX", 4839.81)})

	res := Match(settled)
	near(t, "total", totalPnL(res), (2.99-59.81)*100+4.0*100)
	if report.Indexes[0].Expiries != 2 || report.Indexes[0].InTheMoney != 1 {
		t.Errorf("%+v", report.Indexes)
	}
	// paying 59.81 points on a short call is a cash outflow of 5981 (the rate is 1 here)
	near(t, "settlement cash", report.Indexes[0].Cash, -5981)
}

func TestSettleXSPUsesATenthOfSPX(t *testing.T) {
	expiry := time.Date(2025, 3, 3, 0, 0, 0, 0, time.UTC)
	put := indexOption(1, "XSP", models.Put, 600, expiry)

	trades := []models.Trade{fill(put, at("2025-03-03", 10, 0), 1, 1.0), fill(put, at("2025-03-03", 16, 20), -1, 0)}
	settled, _ := Settle(trades, []models.SettlementPrice{price("2025-03-03", "SPX", 5849)}) // XSP 584.9

	near(t, "long put worth 15.1 points", totalPnL(Match(settled)), (15.1-1.0)*100)

	// an explicit XSP price wins over the derived one
	settled, _ = Settle(trades, []models.SettlementPrice{price("2025-03-03", "SPX", 5849), price("2025-03-03", "XSP", 590)})
	near(t, "explicit XSP price", totalPnL(Match(settled)), (10.0-1.0)*100)
}

func TestSettleLeavesOtherExpiriesAlone(t *testing.T) {
	expiry := time.Date(2024, 1, 19, 0, 0, 0, 0, time.UTC)
	spx := indexOption(1, "SPXW", models.Call, 4700, expiry)
	equity := indexOption(2, "AAPL", models.Call, 150, expiry) // physically settled: assignment shows up as a stock trade
	prices := []models.SettlementPrice{price("2024-01-19", "SPX", 4839.81)}

	cases := map[string]models.Trade{
		"equity option":        fill(equity, at("2024-01-19", 16, 20), 1, 0),
		"traded before expiry": fill(spx, at("2024-01-18", 16, 20), 1, 0),
		"priced trade":         fill(spx, at("2024-01-19", 10, 0), 1, 1.5),
		"stock":                fill(models.Instrument{ID: 3, Type: models.Stock, Symbol: "SPXW", Underlying: "SPXW", Multiplier: 1}, at("2024-01-19", 16, 20), 1, 0),
	}
	for name, tr := range cases {
		got, report := Settle([]models.Trade{tr}, prices)
		if got[0].Price != tr.Price || len(report.Indexes) != 0 || len(report.Missing) != 0 {
			t.Errorf("%s: %+v %+v", name, got[0], report)
		}
	}

	assigned := fill(spx, at("2024-01-19", 16, 20), 1, 0)
	assigned.Kind = models.KindAssign
	if got, _ := Settle([]models.Trade{assigned}, prices); got[0].Price != 0 {
		t.Errorf("assignments keep their price: %+v", got[0])
	}
}

func TestSettleUsesEasternDateNearMidnightUTC(t *testing.T) {
	// 16:20 in New York is already the next day in UTC in winter and summer alike
	expiry := time.Date(2024, 7, 5, 0, 0, 0, 0, time.UTC)
	call := indexOption(1, "SPXW", models.Call, 5500, expiry)

	trade := fill(call, at("2024-07-05", 20, 0), 1, 0) // 20:00 EDT is 00:00 UTC on 6 July
	if trade.TradedAt.UTC().Format(time.DateOnly) != "2024-07-06" {
		t.Fatal("test setup")
	}
	settled, _ := Settle([]models.Trade{trade}, []models.SettlementPrice{price("2024-07-05", "SPX", 5600)})
	near(t, "priced on the Eastern day", settled[0].Price.Float(), 100)
}

func TestSettlingIndex(t *testing.T) {
	for root, want := range map[string]string{"SPX": "SET", "SPXW": "SPX", "XSP": "XSP", "RUTW": "RUT", "NDXP": "NDX"} {
		if got, ok := SettlingIndex(root); !ok || got != want {
			t.Errorf("%s: %q %v", root, got, ok)
		}
	}
	for _, root := range []string{"AAPL", "SPY", "VIX"} {
		if _, ok := SettlingIndex(root); ok {
			t.Errorf("%s is not settled in cash by this program", root)
		}
	}
}

func TestMonthlySPXSettlesOnTheOpenWeeklySPXWOnTheClose(t *testing.T) {
	expiry := time.Date(2024, 1, 19, 0, 0, 0, 0, time.UTC)
	monthly := indexOption(1, "SPX", models.Call, 4780, expiry)
	weekly := indexOption(2, "SPXW", models.Call, 4780, expiry)

	trades := []models.Trade{
		fill(monthly, at("2024-01-19", 16, 20), 1, 0),
		fill(weekly, at("2024-01-19", 16, 20), 1, 0),
	}
	// the opening value (SOQ) of that Friday differs from its close
	settled, report := Settle(trades, []models.SettlementPrice{
		price("2024-01-19", "SET", 4799.62),
		price("2024-01-19", "SPX", 4839.81),
	})

	near(t, "monthly SPX", settled[0].Price.Float(), 4799.62-4780)
	near(t, "SPXW", settled[1].Price.Float(), 4839.81-4780)
	if len(report.Indexes) != 2 || len(report.Missing) != 0 {
		t.Errorf("%+v", report)
	}

	// the close is not a stand-in for the opening value: the monthly options are reported
	settled, report = Settle(trades[:1], []models.SettlementPrice{price("2024-01-19", "SPX", 4839.81)})
	if settled[0].Price != 0 || len(report.Missing) != 1 || report.Missing[0].Index != "SET" {
		t.Errorf("%+v %+v", settled[0], report)
	}
}

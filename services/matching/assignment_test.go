package matching

import (
	"testing"
	"time"

	"portfolio/models"
)

var (
	putExpiry = time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	shares    = models.Instrument{ID: 10, Type: models.Stock, Symbol: "XYZ", Underlying: "XYZ", Currency: "USD", Multiplier: 1}
	put10     = indexOption(11, "XYZ", models.Put, 10, putExpiry)
	call10    = indexOption(12, "XYZ", models.Call, 10, putExpiry)
)

func withFee(t models.Trade, fee float64) models.Trade {
	t.Commission = models.FixedFromFloat(fee)
	return t
}

// pnlOf is the realized result of the round trips of one instrument.
func pnlOf(res models.MatchResult, in models.Instrument) float64 {
	var sum float64
	for _, rt := range res.RoundTrips {
		if rt.Instrument.ID == in.ID {
			sum += rt.RealizedPnL().Float()
		}
	}
	return sum
}

func TestAssignedShortPutRollsPremiumIntoShares(t *testing.T) {
	end := at("2024-03-15", 16, 20)
	trades := []models.Trade{
		withFee(fill(put10, at("2024-03-01", 10, 0), -1, 2), 1), // 199 received
		fill(put10, end, 1, 0),
		fill(shares, end, 100, 10),
		withFee(fill(shares, at("2024-03-20", 10, 0), -100, 12), 1),
	}

	res := Match(RollPremiums(trades))

	near(t, "option", pnlOf(res, put10), 0)
	// 1200 - 1 received, 1000 - 199 paid
	near(t, "shares", pnlOf(res, shares), 398)
	near(t, "total", totalPnL(res), 398)
	for _, rt := range res.RoundTrips {
		if rt.Instrument.ID == put10.ID && rt.IsOpen() {
			t.Error("the assigned put is still open")
		}
	}
	if trades[1].Kind != models.KindTrade || trades[2].Price.Float() != 10 {
		t.Error("the input was modified")
	}
}

func TestExercisedLongPutRollsPremiumIntoShares(t *testing.T) {
	end := at("2024-03-15", 16, 20)
	trades := []models.Trade{
		withFee(fill(put10, at("2024-03-01", 10, 0), 1, 3), 1), // 301 paid
		fill(put10, end, -1, 0),
		fill(shares, end, -100, 10), // sold at the strike
		fill(shares, at("2024-03-15", 17, 0), 100, 9),
	}

	res := Match(RollPremiums(trades))

	near(t, "option", pnlOf(res, put10), 0)
	near(t, "shares", pnlOf(res, shares), 100-301)
}

func TestAssignedShortCallAddsPremiumToProceeds(t *testing.T) {
	end := at("2024-03-15", 16, 20)
	trades := []models.Trade{
		fill(shares, at("2024-03-01", 10, 0), 100, 8),
		fill(call10, at("2024-03-02", 10, 0), -1, 0.5), // 50 received
		fill(call10, end, 1, 0),
		fill(shares, end, -100, 10), // called away
	}

	res := Match(RollPremiums(trades))

	near(t, "option", pnlOf(res, call10), 0)
	near(t, "shares", pnlOf(res, shares), 200+50)
}

func TestRollPremiumsNeedsMatchingShares(t *testing.T) {
	end := at("2024-03-15", 16, 20)
	open := withFee(fill(put10, at("2024-03-01", 10, 0), -1, 2), 1)

	for name, stockTrade := range map[string]models.Trade{
		"other time":  fill(shares, at("2024-03-15", 16, 21), 100, 10),
		"other size":  fill(shares, end, 50, 10),
		"other side":  fill(shares, end, -100, 10),
		"other price": fill(shares, end, 100, 12),
	} {
		trades := []models.Trade{open, fill(put10, end, 1, 0), stockTrade}
		res := Match(RollPremiums(trades))
		near(t, name, pnlOf(res, put10), 199) // expired worthless
	}
}

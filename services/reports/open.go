package reports

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"portfolio/models"
)

// OpenPosition is the not yet closed part of an open round trip.
type OpenPosition struct {
	Instrument models.Instrument
	Direction  models.Direction
	Quantity   models.Fixed // absolute, still open
	OpenedAt   time.Time    // of the round trip
	AvgPrice   float64      // of the open lots, instrument currency
	Cost       float64      // price x quantity x multiplier of the open lots, instrument currency
	Realized   float64      // base currency, from parts of the round trip already closed
}

// DaysOpen counts from the first opening trade until now.
func (p OpenPosition) DaysOpen(now time.Time) int {
	return int(now.Sub(p.OpenedAt).Hours() / 24)
}

// DaysToExpiry is negative once an option is past expiry; ok is false for instruments without one.
func (p OpenPosition) DaysToExpiry(now time.Time) (days int, ok bool) {
	if p.Instrument.Expiry.IsZero() {
		return 0, false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return int(p.Instrument.Expiry.Sub(today).Hours() / 24), true
}

// OpenPositions lists the open round trips with their remaining lots, built
// from the round trips and the trades they were matched from. An empty symbol
// matches every underlying.
func OpenPositions(trips []models.RoundTrip, trades []models.Trade, symbol string) []OpenPosition {
	symbol = strings.ToUpper(symbol)

	// quantity of each opening trade that was already closed
	closed := map[int64]models.Fixed{}
	for _, rt := range trips {
		for _, m := range rt.Matches {
			closed[m.OpenTradeID] += m.Quantity
		}
	}

	var result []OpenPosition
	for _, rt := range trips {
		if !rt.IsOpen() || (symbol != "" && rt.Instrument.Underlying != symbol) {
			continue
		}

		pos := OpenPosition{
			Instrument: rt.Instrument,
			Direction:  rt.Direction,
			Quantity:   rt.OpenQuantity,
			OpenedAt:   rt.OpenedAt,
			Realized:   rt.RealizedBase(),
		}

		var units float64
		for _, t := range trades {
			if t.AccountID != rt.AccountID || t.Instrument.ID != rt.Instrument.ID ||
				t.TradedAt.Before(rt.OpenedAt) || (t.Quantity > 0) != (rt.Direction == models.Long) {
				continue
			}
			remaining := (t.Quantity.Abs() - closed[t.ID]).Float()
			if remaining <= 0 {
				continue
			}
			units += remaining
			pos.Cost += remaining * t.Price.Float() * float64(t.Instrument.Multiplier)
		}
		if units > 0 {
			pos.AvgPrice = pos.Cost / units / float64(max(rt.Instrument.Multiplier, 1))
		}

		result = append(result, pos)
	}

	slices.SortFunc(result, func(a, b OpenPosition) int {
		return cmp.Or(
			cmp.Compare(a.Instrument.Underlying, b.Instrument.Underlying),
			a.Instrument.Expiry.Compare(b.Instrument.Expiry),
			cmp.Compare(a.Instrument.Strike, b.Instrument.Strike),
			a.OpenedAt.Compare(b.OpenedAt),
		)
	})
	return result
}

// Package matching pairs opening and closing trades first-in-first-out and
// groups them into round trips (flat to flat) per account and instrument.
package matching

import (
	"cmp"
	"slices"

	"portfolio/models"
)

// dust is the largest quantity (0.0001 units) treated as zero after rounding.
const dust = models.Fixed(100)

// lot is the not yet closed part of an opening trade.
type lot struct {
	trade     models.Trade
	remaining models.Fixed // absolute
}

func Match(trades []models.Trade) models.MatchResult {
	groups := map[groupKey][]models.Trade{}
	for _, t := range trades {
		if t.Quantity == 0 {
			continue
		}
		key := keyOf(t)
		groups[key] = append(groups[key], t)
	}

	var result models.MatchResult
	for _, group := range groups {
		slices.SortStableFunc(group, func(a, b models.Trade) int {
			return cmp.Or(
				a.TradedAt.Compare(b.TradedAt),
				cmp.Compare(effectRank(a.Effect), effectRank(b.Effect)),
				cmp.Compare(a.ID, b.ID),
			)
		})
		matchGroup(group, &result)
	}

	slices.SortFunc(result.RoundTrips, func(a, b models.RoundTrip) int {
		return cmp.Or(
			a.OpenedAt.Compare(b.OpenedAt),
			cmp.Compare(a.Instrument.Symbol, b.Instrument.Symbol),
			cmp.Compare(a.AccountID, b.AccountID),
		)
	})
	slices.SortFunc(result.Unmatched, func(a, b models.Unmatched) int {
		return cmp.Or(a.Trade.TradedAt.Compare(b.Trade.TradedAt), cmp.Compare(a.Trade.ID, b.Trade.ID))
	})
	return result
}

// matchGroup handles the chronologically sorted trades of one instrument.
func matchGroup(trades []models.Trade, result *models.MatchResult) {
	var queue []lot
	var current *models.RoundTrip

	for _, t := range trades {
		direction := models.Long
		if t.Quantity < 0 {
			direction = models.Short
		}
		rest := t.Quantity.Abs() // part of the trade not yet allocated

		crossing := current != nil && current.Direction != direction
		if crossing {
			for rest > 0 && len(queue) > 0 {
				open := &queue[0]
				n := min(rest, open.remaining)

				current.Matches = append(current.Matches, newMatch(open.trade, t, n))
				current.OpenQuantity -= n
				open.remaining -= n
				rest -= n

				if open.remaining == 0 {
					queue = queue[1:]
				}
			}

			// split adjustments leave rounding dust, which is not a position
			if rest <= dust {
				rest = 0
			}
			var left models.Fixed
			for _, l := range queue {
				left += l.remaining
			}
			if left <= dust {
				queue = nil
				current.OpenQuantity = 0
			}

			if len(queue) == 0 {
				current.ClosedAt = t.TradedAt
				result.RoundTrips = append(result.RoundTrips, *current)
				current = nil
			}
		}

		// A closing trade without a position to close has no opening trade. One that
		// is larger than the position it closes is different: IB flags every fill of an
		// order with the same effect, even when the last one crosses zero, so what is
		// left over opens a position in the other direction.
		if rest > 0 && t.Effect == models.EffectClose && !crossing {
			result.Unmatched = append(result.Unmatched, models.Unmatched{Trade: t, Quantity: rest})
			rest = 0
		}

		if rest > 0 {
			if current == nil {
				current = &models.RoundTrip{
					AccountID:  t.AccountID,
					Instrument: t.Instrument,
					Direction:  direction,
					OpenedAt:   t.TradedAt,
				}
			}
			queue = append(queue, lot{trade: t, remaining: rest})
			current.OpenQuantity += rest
			current.MaxQuantity = max(current.MaxQuantity, current.OpenQuantity)
		}
	}

	if current != nil {
		result.RoundTrips = append(result.RoundTrips, *current)
	}
}

// newMatch closes n units of the opening trade with the closing trade.
func newMatch(open, close models.Trade, n models.Fixed) models.LotMatch {
	openCash, openFee := legCash(open, n)
	closeCash, closeFee := legCash(close, n)

	pnl := openCash + closeCash
	if close.Kind == models.KindAssign || close.Kind == models.KindExercise {
		// the premium went into the cost of the delivered shares, see RollPremiums
		pnl = 0
	}

	return models.LotMatch{
		OpenTradeID:  open.ID,
		CloseTradeID: close.ID,
		OpenedAt:     open.TradedAt,
		ClosedAt:     close.TradedAt,
		Quantity:     n,
		PnL:          models.FixedFromFloat(pnl),
		Fees:         models.FixedFromFloat(openFee + closeFee),
		// like IB: the profit in the trade currency, converted at the rate of the closing day.
		// The currency move of the money tied up in between is not trading profit.
		PnLBase: pnl * close.Rate(),
	}
}

// legCash is the cash flow of n units of a trade, net of its pro-rated
// commission, and that commission, in instrument currency.
func legCash(t models.Trade, n models.Fixed) (cash, fee float64) {
	share := n.Float() / t.Quantity.Abs().Float()
	fee = t.Commission.Float() * share

	gross := n.Float() * t.Price.Float() * float64(t.Instrument.Multiplier)
	if t.Quantity > 0 {
		gross = -gross
	}
	return gross - fee, fee
}

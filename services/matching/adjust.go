package matching

import (
	"cmp"
	"context"
	"slices"

	"portfolio/database"
	"portfolio/models"
)

// Adjust restates trades for corporate actions, as if the new unit size had
// always been in place: trades before an action get their quantity multiplied
// and their price divided by its ratio, and are moved to the new instrument for
// a conversion. Cost and profit of a position stay the same. The input is not modified.
func Adjust(trades []models.Trade, actions []models.CorporateAction) []models.Trade {
	adjusted := slices.Clone(trades)

	sorted := slices.SortedStableFunc(slices.Values(actions), func(a, b models.CorporateAction) int {
		return cmp.Or(a.At.Compare(b.At), cmp.Compare(a.ID, b.ID))
	})

	// oldest action first, so that chains (A to B, later B to C) work out
	for _, a := range sorted {
		if a.Ratio <= 0 {
			continue
		}
		for i, t := range adjusted {
			if t.AccountID != a.AccountID || !t.TradedAt.Before(a.At) || !t.Instrument.Same(a.From) {
				continue
			}
			t.Quantity = models.FixedFromFloat(t.Quantity.Float() * a.Ratio)
			t.Price = models.FixedFromFloat(t.Price.Float() / a.Ratio)
			if a.Kind == models.ActionConvert {
				t.Instrument = a.To
			}
			adjusted[i] = t
		}
	}
	return adjusted
}

// LoadTrades reads all trades and restates them for the stored corporate actions.
func LoadTrades(ctx context.Context, db *database.DB) ([]models.Trade, error) {
	trades, err := db.GetTrades(ctx, database.TradeQuery{})
	if err != nil {
		return nil, err
	}
	actions, err := db.GetCorporateActions(ctx)
	if err != nil {
		return nil, err
	}
	return Adjust(trades, actions), nil
}

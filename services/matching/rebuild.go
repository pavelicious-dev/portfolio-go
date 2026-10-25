package matching

import (
	"context"

	"portfolio/database"
	"portfolio/models"
)

// LoadTrades reads all trades, restated for the stored corporate actions and
// with the expiries of cash-settled index options priced from the given settlement prices.
func LoadTrades(ctx context.Context, db *database.DB, prices []models.SettlementPrice) ([]models.Trade, error) {
	trades, _, err := LoadSettledTrades(ctx, db, prices)
	return trades, err
}

// LoadSettledTrades is LoadTrades together with what the settlement of index options found.
func LoadSettledTrades(ctx context.Context, db *database.DB, prices []models.SettlementPrice) ([]models.Trade, SettlementReport, error) {
	trades, err := db.GetTrades(ctx, database.TradeQuery{})
	if err != nil {
		return nil, SettlementReport{}, err
	}
	actions, err := db.GetCorporateActions(ctx)
	if err != nil {
		return nil, SettlementReport{}, err
	}

	settled, report := Settle(Adjust(trades, actions), prices)
	return settled, report, nil
}

// Rebuild recomputes all round trips from the stored trades and corporate actions
// and the given settlement prices.
func Rebuild(ctx context.Context, db *database.DB, prices []models.SettlementPrice) error {
	trades, err := LoadTrades(ctx, db, prices)
	if err != nil {
		return err
	}
	return db.ReplaceMatching(ctx, Match(trades))
}

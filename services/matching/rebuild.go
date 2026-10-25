package matching

import (
	"context"

	"portfolio/database"
)

// Rebuild recomputes all round trips from the stored trades.
func Rebuild(ctx context.Context, db *database.DB) error {
	trades, err := db.GetTrades(ctx, database.TradeQuery{})
	if err != nil {
		return err
	}
	return db.ReplaceMatching(ctx, Match(trades))
}

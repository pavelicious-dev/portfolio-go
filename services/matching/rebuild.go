package matching

import (
	"context"

	"portfolio/database"
)

// Rebuild recomputes all round trips from the stored trades and corporate actions.
func Rebuild(ctx context.Context, db *database.DB) error {
	trades, err := LoadTrades(ctx, db)
	if err != nil {
		return err
	}
	return db.ReplaceMatching(ctx, Match(trades))
}

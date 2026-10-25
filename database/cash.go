package database

import (
	"context"

	"portfolio/models"
)

// GetCashEvents returns all cash bookings, oldest first.
func (db *DB) GetCashEvents(ctx context.Context) ([]models.CashEvent, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, account_id, source_id, category, happened_at, symbol, amount, currency, fx_to_base, memo
FROM cash_events ORDER BY happened_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []models.CashEvent{}
	for rows.Next() {
		var e models.CashEvent
		var at string
		var amount int64

		if err := rows.Scan(&e.ID, &e.AccountID, &e.SourceID, &e.Category, &at, &e.Symbol, &amount, &e.Currency, &e.FxToBase, &e.Memo); err != nil {
			return nil, err
		}
		if e.At, err = parseTime(at); err != nil {
			return nil, err
		}
		e.Amount = models.Fixed(amount)
		events = append(events, e)
	}
	return events, rows.Err()
}

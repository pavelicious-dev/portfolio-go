package database

import (
	"context"
	"time"

	"portfolio/models"
)

// GetCorporateActions returns all splits and conversions, oldest first.
func (db *DB) GetCorporateActions(ctx context.Context) ([]models.CorporateAction, error) {
	rows, err := db.QueryContext(ctx, `
SELECT c.id, c.account_id, c.source_id, c.kind, c.happened_at, c.ratio, c.memo,
       f.id, f.conid, f.type, f.symbol, f.underlying, f.currency, f.multiplier, f.expiry, f.strike, f.right,
       COALESCE(t.id, 0), COALESCE(t.conid, ''), COALESCE(t.type, ''), COALESCE(t.symbol, ''), COALESCE(t.underlying, ''),
       COALESCE(t.currency, ''), COALESCE(t.multiplier, 1), COALESCE(t.expiry, ''), COALESCE(t.strike, 0), COALESCE(t.right, '')
FROM corporate_actions c
JOIN instruments f ON f.id = c.instrument_id
LEFT JOIN instruments t ON t.id = c.to_instrument_id
ORDER BY c.happened_at, c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	actions := []models.CorporateAction{}
	for rows.Next() {
		var a models.CorporateAction
		var at, fromExpiry, toExpiry string
		var fromStrike, toStrike int64

		err := rows.Scan(&a.ID, &a.AccountID, &a.SourceID, &a.Kind, &at, &a.Ratio, &a.Memo,
			&a.From.ID, &a.From.ConID, &a.From.Type, &a.From.Symbol, &a.From.Underlying, &a.From.Currency,
			&a.From.Multiplier, &fromExpiry, &fromStrike, &a.From.Right,
			&a.To.ID, &a.To.ConID, &a.To.Type, &a.To.Symbol, &a.To.Underlying, &a.To.Currency,
			&a.To.Multiplier, &toExpiry, &toStrike, &a.To.Right)
		if err != nil {
			return nil, err
		}

		if a.At, err = parseTime(at); err != nil {
			return nil, err
		}
		if a.From.Expiry, err = parseExpiry(fromExpiry); err != nil {
			return nil, err
		}
		if a.To.Expiry, err = parseExpiry(toExpiry); err != nil {
			return nil, err
		}
		a.From.Strike, a.To.Strike = models.Fixed(fromStrike), models.Fixed(toStrike)

		actions = append(actions, a)
	}
	return actions, rows.Err()
}

func parseExpiry(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.DateOnly, s)
}

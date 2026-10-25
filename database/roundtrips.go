package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"portfolio/models"
)

// ReplaceMatching stores a matcher result, replacing all previous round trips.
func (db *DB) ReplaceMatching(ctx context.Context, result models.MatchResult) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{"lot_matches", "unmatched_closes", "round_trips"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}

	for _, rt := range result.RoundTrips {
		closedAt := ""
		if !rt.ClosedAt.IsZero() {
			closedAt = formatTime(rt.ClosedAt)
		}

		res, err := tx.ExecContext(ctx, `
INSERT INTO round_trips (account_id, instrument_id, direction, opened_at, closed_at, max_quantity, open_quantity)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			rt.AccountID, rt.Instrument.ID, rt.Direction, formatTime(rt.OpenedAt), closedAt,
			int64(rt.MaxQuantity), int64(rt.OpenQuantity))
		if err != nil {
			return fmt.Errorf("round trip %s: %w", rt.Instrument.Symbol, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}

		for _, m := range rt.Matches {
			_, err := tx.ExecContext(ctx, `
INSERT INTO lot_matches (round_trip_id, open_trade_id, close_trade_id, opened_at, closed_at, quantity, pnl, fees, pnl_base)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, m.OpenTradeID, m.CloseTradeID, formatTime(m.OpenedAt), formatTime(m.ClosedAt),
				int64(m.Quantity), int64(m.PnL), int64(m.Fees), m.PnLBase)
			if err != nil {
				return fmt.Errorf("match in %s: %w", rt.Instrument.Symbol, err)
			}
		}
	}

	for _, u := range result.Unmatched {
		_, err := tx.ExecContext(ctx, `
INSERT INTO unmatched_closes (trade_id, quantity) VALUES (?, ?)
`,
			u.Trade.ID, int64(u.Quantity))
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

type RoundTripQuery struct {
	Underlying string // exact match, case-insensitive
}

// GetRoundTrips returns round trips with their matches, ordered by opening time.
func (db *DB) GetRoundTrips(ctx context.Context, q RoundTripQuery) ([]models.RoundTrip, error) {
	var where []string
	var args []any
	if q.Underlying != "" {
		where = append(where, "i.underlying = ?")
		args = append(args, strings.ToUpper(q.Underlying))
	}
	filter := ""
	if len(where) > 0 {
		filter = " WHERE " + strings.Join(where, " AND ")
	}

	rows, err := db.QueryContext(ctx, `
SELECT r.id, r.account_id, r.direction, r.opened_at, r.closed_at, r.max_quantity, r.open_quantity,
       i.id, i.conid, i.type, i.symbol, i.underlying, i.currency, i.multiplier, i.expiry, i.strike, i.right
FROM round_trips r JOIN instruments i ON i.id = r.instrument_id`+filter+`
ORDER BY r.opened_at, r.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trips := []models.RoundTrip{}
	byID := map[int64]int{}
	for rows.Next() {
		var rt models.RoundTrip
		var openedAt, closedAt, expiry string
		var maxQty, openQty, strike int64

		err := rows.Scan(&rt.ID, &rt.AccountID, &rt.Direction, &openedAt, &closedAt, &maxQty, &openQty,
			&rt.Instrument.ID, &rt.Instrument.ConID, &rt.Instrument.Type, &rt.Instrument.Symbol, &rt.Instrument.Underlying,
			&rt.Instrument.Currency, &rt.Instrument.Multiplier, &expiry, &strike, &rt.Instrument.Right)
		if err != nil {
			return nil, err
		}
		if rt.OpenedAt, err = parseTime(openedAt); err != nil {
			return nil, err
		}
		if closedAt != "" {
			if rt.ClosedAt, err = parseTime(closedAt); err != nil {
				return nil, err
			}
		}
		if expiry != "" {
			if rt.Instrument.Expiry, err = time.Parse(time.DateOnly, expiry); err != nil {
				return nil, err
			}
		}
		rt.MaxQuantity, rt.OpenQuantity = models.Fixed(maxQty), models.Fixed(openQty)
		rt.Instrument.Strike = models.Fixed(strike)
		rt.Matches = []models.LotMatch{}

		byID[rt.ID] = len(trips)
		trips = append(trips, rt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	matches, err := db.QueryContext(ctx, `
SELECT round_trip_id, open_trade_id, close_trade_id, opened_at, closed_at, quantity, pnl, fees, pnl_base
FROM lot_matches ORDER BY closed_at, id`)
	if err != nil {
		return nil, err
	}
	defer matches.Close()

	for matches.Next() {
		var tripID, qty, pnl, fees int64
		var m models.LotMatch
		var openedAt, closedAt string

		if err := matches.Scan(&tripID, &m.OpenTradeID, &m.CloseTradeID, &openedAt, &closedAt, &qty, &pnl, &fees, &m.PnLBase); err != nil {
			return nil, err
		}
		idx, ok := byID[tripID]
		if !ok { // filtered out
			continue
		}
		if m.OpenedAt, err = parseTime(openedAt); err != nil {
			return nil, err
		}
		if m.ClosedAt, err = parseTime(closedAt); err != nil {
			return nil, err
		}
		m.Quantity, m.PnL, m.Fees = models.Fixed(qty), models.Fixed(pnl), models.Fixed(fees)
		trips[idx].Matches = append(trips[idx].Matches, m)
	}
	return trips, matches.Err()
}

// CountUnmatched is the number of closing trades with no known opening trade.
func (db *DB) CountUnmatched(ctx context.Context) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM unmatched_closes").Scan(&n)
	return n, err
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

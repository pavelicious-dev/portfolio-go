package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"portfolio/models"
)

type TradeQuery struct {
	Underlying string // exact match, case-insensitive
	Year       int    // 0 = any
	Month      int    // 0 = any
}

func (db *DB) GetTrades(ctx context.Context, q TradeQuery) ([]models.Trade, error) {
	var where []string
	var args []any

	if q.Underlying != "" {
		where = append(where, "i.underlying = ?")
		args = append(args, strings.ToUpper(q.Underlying))
	}
	if q.Year != 0 {
		where = append(where, "strftime('%Y', t.traded_at) = ?")
		args = append(args, fmt.Sprintf("%04d", q.Year))
	}
	if q.Month != 0 {
		where = append(where, "strftime('%m', t.traded_at) = ?")
		args = append(args, fmt.Sprintf("%02d", q.Month))
	}

	query := `
SELECT t.id, t.account_id, t.source, t.source_id, t.kind, t.effect, t.traded_at,
       t.quantity, t.price, t.commission, t.fx_to_base,
       i.id, i.conid, i.type, i.symbol, i.underlying, i.currency, i.multiplier, i.expiry, i.strike, i.right
FROM trades t JOIN instruments i ON i.id = t.instrument_id`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY t.traded_at, t.id"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trades := []models.Trade{}
	for rows.Next() {
		var t models.Trade
		var tradedAt, expiry string
		var qty, price, commission, strike int64

		err := rows.Scan(&t.ID, &t.AccountID, &t.Source, &t.SourceID, &t.Kind, &t.Effect, &tradedAt,
			&qty, &price, &commission, &t.FxToBase,
			&t.Instrument.ID, &t.Instrument.ConID, &t.Instrument.Type, &t.Instrument.Symbol, &t.Instrument.Underlying,
			&t.Instrument.Currency, &t.Instrument.Multiplier, &expiry, &strike, &t.Instrument.Right)
		if err != nil {
			return nil, err
		}

		if t.TradedAt, err = time.Parse(time.RFC3339, tradedAt); err != nil {
			return nil, fmt.Errorf("trade %d: %w", t.ID, err)
		}
		if expiry != "" {
			if t.Instrument.Expiry, err = time.Parse(time.DateOnly, expiry); err != nil {
				return nil, fmt.Errorf("trade %d: %w", t.ID, err)
			}
		}
		t.Quantity, t.Price, t.Commission = models.Fixed(qty), models.Fixed(price), models.Fixed(commission)
		t.Instrument.Strike = models.Fixed(strike)

		trades = append(trades, t)
	}
	return trades, rows.Err()
}

// ImportTrades stores trades for the given broker account in one transaction.
// Trades already present (same account and source id) are skipped.
// It returns the number of newly inserted trades.
func (db *DB) ImportTrades(ctx context.Context, brokerID, baseCcy string, trades []models.Trade) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var accountID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO accounts (broker_id, base_ccy) VALUES (?, ?)
ON CONFLICT (broker_id) DO UPDATE SET broker_id = broker_id
RETURNING id`, brokerID, baseCcy).Scan(&accountID)
	if err != nil {
		return 0, fmt.Errorf("account %s: %w", brokerID, err)
	}

	instruments := map[string]int64{}
	inserted := 0

	for _, t := range trades {
		in := t.Instrument
		key := string(in.Type) + "|" + in.Symbol + "|" + in.Currency
		if in.ConID != "" {
			key = in.ConID
		}

		instrumentID, ok := instruments[key]
		if !ok {
			instrumentID, err = upsertInstrument(ctx, tx, in)
			if err != nil {
				return 0, fmt.Errorf("instrument %s: %w", in.Symbol, err)
			}
			instruments[key] = instrumentID
		}

		kind := t.Kind
		if kind == "" {
			kind = models.KindTrade
		}

		res, err := tx.ExecContext(ctx, `
INSERT INTO trades (account_id, instrument_id, source, source_id, kind, effect, traded_at, quantity, price, commission, fx_to_base)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (account_id, source_id) DO NOTHING`,
			accountID, instrumentID, t.Source, t.SourceID, kind, string(t.Effect), t.TradedAt.UTC().Format(time.RFC3339),
			int64(t.Quantity), int64(t.Price), int64(t.Commission), t.FxToBase)
		if err != nil {
			return 0, fmt.Errorf("trade %s: %w", t.SourceID, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
			continue
		}

		// already imported: fill in the effect for trades stored before it existed
		_, err = tx.ExecContext(ctx, `
UPDATE trades SET effect = ? WHERE account_id = ? AND source_id = ? AND effect = '' AND ? <> ''`,
			string(t.Effect), accountID, t.SourceID, string(t.Effect))
		if err != nil {
			return 0, fmt.Errorf("trade %s: %w", t.SourceID, err)
		}
	}

	return inserted, tx.Commit()
}

// upsertInstrument finds an instrument by broker contract id, then by symbol,
// and creates it if unknown.
func upsertInstrument(ctx context.Context, tx *sql.Tx, in models.Instrument) (int64, error) {
	var id int64

	if in.ConID != "" {
		err := tx.QueryRowContext(ctx, `SELECT id FROM instruments WHERE conid = ?`, in.ConID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}

	expiry := ""
	if !in.Expiry.IsZero() {
		expiry = in.Expiry.Format(time.DateOnly)
	}
	err := tx.QueryRowContext(ctx, `
INSERT INTO instruments (conid, type, symbol, underlying, currency, multiplier, expiry, strike, right)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (type, symbol, currency) DO UPDATE SET conid = CASE WHEN conid = '' THEN excluded.conid ELSE conid END
RETURNING id`,
		in.ConID, in.Type, in.Symbol, in.Underlying, in.Currency, in.Multiplier, expiry, int64(in.Strike), string(in.Right)).Scan(&id)
	return id, err
}

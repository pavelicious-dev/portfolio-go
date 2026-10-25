-- open/close flag reported by the broker ('' = not reported)
ALTER TABLE trades ADD COLUMN effect TEXT NOT NULL DEFAULT '' CHECK (effect IN ('', 'OPEN', 'CLOSE'));

-- Derived from trades by the FIFO matcher; rebuilt completely after each import.
CREATE TABLE round_trips (
  id            INTEGER PRIMARY KEY,
  account_id    INTEGER NOT NULL REFERENCES accounts (id),
  instrument_id INTEGER NOT NULL REFERENCES instruments (id),
  direction     TEXT NOT NULL CHECK (direction IN ('LONG', 'SHORT')),
  opened_at     TEXT NOT NULL,
  closed_at     TEXT NOT NULL DEFAULT '', -- '' while open
  max_quantity  INTEGER NOT NULL,
  open_quantity INTEGER NOT NULL          -- 0 once closed
);
CREATE INDEX round_trips_by_instrument ON round_trips (instrument_id, opened_at);

CREATE TABLE lot_matches (
  id             INTEGER PRIMARY KEY,
  round_trip_id  INTEGER NOT NULL REFERENCES round_trips (id) ON DELETE CASCADE,
  open_trade_id  INTEGER NOT NULL REFERENCES trades (id),
  close_trade_id INTEGER NOT NULL REFERENCES trades (id),
  opened_at      TEXT NOT NULL,
  closed_at      TEXT NOT NULL,
  quantity       INTEGER NOT NULL,
  pnl            INTEGER NOT NULL, -- instrument currency, net of fees
  fees           INTEGER NOT NULL,
  pnl_base       REAL NOT NULL
);
CREATE INDEX lot_matches_by_round_trip ON lot_matches (round_trip_id);
CREATE INDEX lot_matches_by_close ON lot_matches (closed_at);

-- Closing trades without a known opening trade (earlier statements not imported)
CREATE TABLE unmatched_closes (
  trade_id INTEGER PRIMARY KEY REFERENCES trades (id),
  quantity INTEGER NOT NULL
);

CREATE VIEW round_trips_readable AS
SELECT
  r.id,
  a.broker_id AS account,
  CASE WHEN i.type = 'OPT' AND i.expiry <> ''
       THEN i.underlying || ' ' || i.expiry || ' ' || printf('%g', i.strike / 1e6) || ' ' || i.right
       ELSE i.symbol END AS instrument,
  i.type,
  i.underlying,
  i.currency,
  r.direction,
  CASE WHEN r.open_quantity = 0 THEN 'CLOSED' ELSE 'OPEN' END AS status,
  datetime(r.opened_at) AS opened_at,
  CASE WHEN r.closed_at = '' THEN NULL ELSE datetime(r.closed_at) END AS closed_at,
  r.max_quantity  / 1e6 AS max_quantity,
  r.open_quantity / 1e6 AS open_quantity,
  COALESCE(SUM(m.pnl), 0)  / 1e6 AS realized_pnl,
  COALESCE(SUM(m.fees), 0) / 1e6 AS fees,
  COALESCE(SUM(m.pnl_base), 0)   AS realized_base
FROM round_trips r
JOIN instruments i ON i.id = r.instrument_id
JOIN accounts a ON a.id = r.account_id
LEFT JOIN lot_matches m ON m.round_trip_id = r.id
GROUP BY r.id;

-- trades_readable gains the effect column
DROP VIEW trades_readable;
CREATE VIEW trades_readable AS
SELECT
  t.id,
  datetime(t.traded_at)                       AS traded_at,
  a.broker_id                                 AS account,
  CASE WHEN i.type = 'OPT' AND i.expiry <> ''
       THEN i.underlying || ' ' || i.expiry || ' ' || printf('%g', i.strike / 1e6) || ' ' || i.right
       ELSE i.symbol END                      AS instrument,
  i.type,
  i.underlying,
  i.currency,
  t.kind,
  t.effect,
  t.quantity   / 1e6                          AS quantity,
  t.price      / 1e6                          AS price,
  i.multiplier,
  t.commission / 1e6                          AS commission,
  -t.quantity / 1e6 * t.price / 1e6 * i.multiplier - t.commission / 1e6 AS cash,
  t.fx_to_base,
  (-t.quantity / 1e6 * t.price / 1e6 * i.multiplier - t.commission / 1e6)
    * CASE WHEN t.fx_to_base > 0 THEN t.fx_to_base ELSE 1 END           AS cash_base,
  t.source,
  t.source_id
FROM trades t
JOIN instruments i ON i.id = t.instrument_id
JOIN accounts a ON a.id = t.account_id;

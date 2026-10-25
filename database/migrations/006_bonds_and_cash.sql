-- Allow T-bills (BOND) as an instrument type. SQLite cannot change a CHECK
-- constraint, so the table is rebuilt; views are dropped and recreated around it.
-- (The migration runner turns foreign keys off and verifies them afterwards.)
DROP VIEW trades_readable;
DROP VIEW round_trips_readable;
DROP VIEW corporate_actions_readable;

CREATE TABLE instruments_new (
  id         INTEGER PRIMARY KEY,
  type       TEXT NOT NULL CHECK (type IN ('STK','OPT','FUT','FOP','CFD','BOND')),
  symbol     TEXT NOT NULL,
  underlying TEXT NOT NULL,
  currency   TEXT NOT NULL,
  multiplier INTEGER NOT NULL DEFAULT 1,
  expiry     TEXT NOT NULL DEFAULT '',
  strike     INTEGER NOT NULL DEFAULT 0,
  right      TEXT NOT NULL DEFAULT '' CHECK (right IN ('','C','P')),
  conid      TEXT NOT NULL DEFAULT '',
  UNIQUE (type, symbol, currency)
);
INSERT INTO instruments_new (id, type, symbol, underlying, currency, multiplier, expiry, strike, right, conid)
SELECT id, type, symbol, underlying, currency, multiplier, expiry, strike, right, conid FROM instruments;
DROP TABLE instruments;
ALTER TABLE instruments_new RENAME TO instruments;
CREATE INDEX instruments_by_underlying ON instruments (underlying);
CREATE UNIQUE INDEX instruments_by_conid ON instruments (conid) WHERE conid <> '';

-- Money that moves without a trade: deposits, dividends, interest, taxes, fees.
-- Amount is a fixed-point integer (x 1e6) in the given currency.
CREATE TABLE cash_events (
  id          INTEGER PRIMARY KEY,
  account_id  INTEGER NOT NULL REFERENCES accounts (id),
  source_id   TEXT NOT NULL,
  category    TEXT NOT NULL CHECK (category IN (
                'DEPOSIT','WITHDRAWAL','DIVIDEND','DIVIDEND_TAX','INTEREST','BORROW_FEE',
                'INTEREST_TAX','DATA_FEE','FEE','FX_CONVERSION')),
  happened_at TEXT NOT NULL, -- UTC, RFC3339
  symbol      TEXT NOT NULL DEFAULT '',
  amount      INTEGER NOT NULL,
  currency    TEXT NOT NULL,
  fx_to_base  REAL NOT NULL DEFAULT 0,
  memo        TEXT NOT NULL DEFAULT '',
  UNIQUE (account_id, source_id)
);
CREATE INDEX cash_events_by_time ON cash_events (happened_at);

CREATE VIEW cash_events_readable AS
SELECT
  e.id,
  datetime(e.happened_at) AS happened_at,
  a.broker_id             AS account,
  e.category,
  e.symbol,
  e.amount / 1e6          AS amount,
  e.currency,
  e.fx_to_base,
  e.amount / 1e6 * CASE WHEN e.fx_to_base > 0 THEN e.fx_to_base ELSE 1 END AS amount_base,
  e.memo
FROM cash_events e
JOIN accounts a ON a.id = e.account_id;

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

CREATE VIEW corporate_actions_readable AS
SELECT
  c.id,
  datetime(c.happened_at) AS happened_at,
  a.broker_id             AS account,
  c.kind,
  f.symbol                AS instrument,
  t.symbol                AS to_instrument,
  c.ratio,
  c.memo
FROM corporate_actions c
JOIN accounts a ON a.id = c.account_id
JOIN instruments f ON f.id = c.instrument_id
LEFT JOIN instruments t ON t.id = c.to_instrument_id;

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
  t.memo,
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

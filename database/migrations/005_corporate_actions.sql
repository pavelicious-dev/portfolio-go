-- Broker note on a trade, used for transfers, and splits and other corporate
-- actions that change the size of a position without a trade.
ALTER TABLE trades ADD COLUMN memo TEXT NOT NULL DEFAULT '';

CREATE TABLE corporate_actions (
  id               INTEGER PRIMARY KEY,
  account_id       INTEGER NOT NULL REFERENCES accounts (id),
  source_id        TEXT NOT NULL,
  kind             TEXT NOT NULL CHECK (kind IN ('SCALE', 'CONVERT')),
  happened_at      TEXT NOT NULL, -- UTC, RFC3339
  instrument_id    INTEGER NOT NULL REFERENCES instruments (id),
  to_instrument_id INTEGER REFERENCES instruments (id), -- CONVERT only
  ratio            REAL NOT NULL, -- new units per old unit
  memo             TEXT NOT NULL DEFAULT '',
  UNIQUE (account_id, source_id)
);

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

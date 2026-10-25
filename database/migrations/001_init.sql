CREATE TABLE accounts (
  id        INTEGER PRIMARY KEY,
  broker_id TEXT NOT NULL UNIQUE,
  base_ccy  TEXT NOT NULL
);

CREATE TABLE instruments (
  id         INTEGER PRIMARY KEY,
  type       TEXT NOT NULL CHECK (type IN ('STK','OPT','FUT','FOP','CFD')),
  symbol     TEXT NOT NULL,
  underlying TEXT NOT NULL,
  currency   TEXT NOT NULL,
  multiplier INTEGER NOT NULL DEFAULT 1,
  expiry     TEXT NOT NULL DEFAULT '',
  strike     INTEGER NOT NULL DEFAULT 0,
  right      TEXT NOT NULL DEFAULT '' CHECK (right IN ('','C','P')),
  UNIQUE (type, symbol, currency)
);
CREATE INDEX instruments_by_underlying ON instruments (underlying);

-- Imported fills. Quantity, price and commission are fixed-point integers (x 1e6).
CREATE TABLE trades (
  id            INTEGER PRIMARY KEY,
  account_id    INTEGER NOT NULL REFERENCES accounts (id),
  instrument_id INTEGER NOT NULL REFERENCES instruments (id),
  source        TEXT NOT NULL,
  source_id     TEXT NOT NULL,
  kind          TEXT NOT NULL DEFAULT 'TRADE' CHECK (kind IN ('TRADE','EXPIRE','ASSIGN','EXERCISE')),
  traded_at     TEXT NOT NULL, -- UTC, RFC3339
  quantity      INTEGER NOT NULL,
  price         INTEGER NOT NULL,
  commission    INTEGER NOT NULL,
  fx_to_base    REAL NOT NULL DEFAULT 0,
  UNIQUE (account_id, source_id)
);
CREATE INDEX trades_by_instrument ON trades (instrument_id, traded_at);
CREATE INDEX trades_by_time ON trades (traded_at);

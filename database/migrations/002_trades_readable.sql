-- Read-only view with decimal values instead of the x 1e6 fixed-point integers,
-- and the instrument name instead of its id.
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

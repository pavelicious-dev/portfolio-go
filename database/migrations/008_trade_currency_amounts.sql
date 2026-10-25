-- Prices and cash amounts of QFX records are stated in the account's base
-- currency; fx_to_base is the rate they were converted at. Earlier versions
-- stored them as they were and treated them as amounts in the trade currency.
-- Divided by the rate they are what the model expects: prices in the currency of
-- the instrument (297.0297 at a rate of 1.0001 is 297.00).
UPDATE trades
SET price = CAST(ROUND(price * 1.0 / fx_to_base) AS INTEGER)
WHERE fx_to_base > 0 AND fx_to_base <> 1;

UPDATE cash_events
SET amount = CAST(ROUND(amount * 1.0 / fx_to_base) AS INTEGER)
WHERE fx_to_base > 0 AND fx_to_base <> 1;

-- round trips were derived from the old prices; the next import rebuilds them
DELETE FROM lot_matches;
DELETE FROM unmatched_closes;
DELETE FROM round_trips;

-- an early build kept settlement prices here; they are read from the downloaded files now
DROP TABLE IF EXISTS settlement_prices;

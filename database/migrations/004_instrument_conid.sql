-- Broker contract id: identifies a contract even if its ticker or strike is
-- adjusted later (dividends, splits). Databases imported before this column
-- existed should be rebuilt by re-importing the statements.
ALTER TABLE instruments ADD COLUMN conid TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX instruments_by_conid ON instruments (conid) WHERE conid <> '';

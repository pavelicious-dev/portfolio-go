package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// oldDatabase builds a database as an older version of the program left it:
// only the migrations up to the given version applied, and some linked data.
func oldDatabase(t *testing.T, path string, version int) {
	t.Helper()

	raw, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		v, _ := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if v > version {
			continue
		}
		script, _ := migrationFiles.ReadFile("migrations/" + name)
		if _, err := raw.Exec(string(script)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := raw.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		t.Fatal(err)
	}

	for _, stmt := range []string{
		`INSERT INTO accounts (id, broker_id, base_ccy) VALUES (1, 'U1', 'EUR')`,
		`INSERT INTO instruments (id, conid, type, symbol, underlying, currency, multiplier) VALUES (1, '100', 'STK', 'ABC', 'ABC', 'USD', 1)`,
		`INSERT INTO instruments (id, conid, type, symbol, underlying, currency, multiplier) VALUES (2, '200', 'STK', 'ABD', 'ABD', 'USD', 1)`,
		`INSERT INTO trades (id, account_id, instrument_id, source, source_id, traded_at, quantity, price, commission)
		   VALUES (1, 1, 1, 'qfx', 't1', '2024-01-02T15:00:00Z', 100000000, 10000000, 1000000)`,
		`INSERT INTO trades (id, account_id, instrument_id, source, source_id, traded_at, quantity, price, commission)
		   VALUES (2, 1, 1, 'qfx', 't2', '2024-01-03T15:00:00Z', -100000000, 12000000, 1000000)`,
		`INSERT INTO round_trips (id, account_id, instrument_id, direction, opened_at, closed_at, max_quantity, open_quantity)
		   VALUES (1, 1, 1, 'LONG', '2024-01-02T15:00:00Z', '2024-01-03T15:00:00Z', 100000000, 0)`,
		`INSERT INTO lot_matches (round_trip_id, open_trade_id, close_trade_id, opened_at, closed_at, quantity, pnl, fees, pnl_base)
		   VALUES (1, 1, 2, '2024-01-02T15:00:00Z', '2024-01-03T15:00:00Z', 100000000, 198000000, 2000000, 180)`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	if version >= 5 {
		_, err := raw.Exec(`INSERT INTO corporate_actions (account_id, source_id, kind, happened_at, instrument_id, to_instrument_id, ratio)
		   VALUES (1, 'a1', 'CONVERT', '2024-01-02T20:00:00Z', 1, 2, 0.5)`)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpgradeKeepsDataAndAllowsBonds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	oldDatabase(t, path, 5)

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	count := func(query string) int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return n
	}

	for table, want := range map[string]int{
		"accounts": 1, "instruments": 2, "trades": 2, "round_trips": 1, "lot_matches": 1, "corporate_actions": 1,
	} {
		if got := count("SELECT COUNT(*) FROM " + table); got != want {
			t.Errorf("%s: %d rows, want %d", table, got, want)
		}
	}

	// the views work again and see the old data
	if got := count("SELECT COUNT(*) FROM trades_readable"); got != 2 {
		t.Errorf("trades_readable: %d", got)
	}
	if got := count("SELECT COUNT(*) FROM round_trips_readable"); got != 1 {
		t.Errorf("round_trips_readable: %d", got)
	}
	if got := count("SELECT COUNT(*) FROM corporate_actions_readable"); got != 1 {
		t.Errorf("corporate_actions_readable: %d", got)
	}
	if got := count("SELECT COUNT(*) FROM cash_events_readable"); got != 0 {
		t.Errorf("cash_events_readable: %d", got)
	}

	// foreign keys hold and are enforced again after the migration
	if got := count("SELECT COUNT(*) FROM pragma_foreign_key_check"); got != 0 {
		t.Errorf("%d foreign key violations", got)
	}
	if got := count("PRAGMA foreign_keys"); got != 1 {
		t.Error("foreign keys are not enforced after migrating")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO trades (account_id, instrument_id, source, source_id, traded_at, quantity, price, commission)
		VALUES (1, 999, 'qfx', 'bad', '2024-01-01T00:00:00Z', 1, 1, 0)`); err == nil {
		t.Error("a trade of a missing instrument was accepted")
	}

	// the new instrument type is accepted, an invented one is not
	if _, err := db.ExecContext(ctx, `INSERT INTO instruments (type, symbol, underlying, currency) VALUES ('BOND', '912797PG6', '912797PG6', 'EUR')`); err != nil {
		t.Errorf("bond: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO instruments (type, symbol, underlying, currency) VALUES ('XXX', 'X', 'X', 'EUR')`); err == nil {
		t.Error("an unknown instrument type was accepted")
	}

	// the conid stays unique
	if _, err := db.ExecContext(ctx, `INSERT INTO instruments (conid, type, symbol, underlying, currency) VALUES ('100', 'STK', 'OTHER', 'OTHER', 'USD')`); err == nil {
		t.Error("a duplicate conid was accepted")
	}

	// opening it again changes nothing
	db.Close()
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	again.Close()
}

func TestFreshDatabaseHasAllTables(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version < 6 {
		t.Errorf("version %d, %v", version, err)
	}
	for _, table := range []string{"cash_events", "corporate_actions", "round_trips", "lot_matches", "unmatched_closes"} {
		if _, err := db.Exec("SELECT 1 FROM " + table + " LIMIT 1"); err != nil {
			t.Errorf("%s: %v", table, err)
		}
	}
}

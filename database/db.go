package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type DB struct {
	*sql.DB
}

// Open opens (and creates) the SQLite database at path and applies pending migrations.
func Open(path string) (*DB, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// a single writer avoids SQLITE_BUSY; reads are cheap at this size
	sqlDB.SetMaxOpenConns(1)

	db := &DB{sqlDB}
	if err := db.migrate(context.Background()); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return db, nil
}

// migrate applies migrations/NNN_name.sql files newer than PRAGMA user_version.
func (db *DB) migrate(ctx context.Context) error {
	var current int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("bad migration name %q", name)
		}
		if version <= current {
			continue
		}

		script, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(script)); err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"portfolio/database"
)

type Options struct {
	BaseCurrency string // reporting currency of newly created accounts
}

// ImportFolder imports every .qfx file in folder.
func ImportFolder(ctx context.Context, db *database.DB, folder string, opts Options) error {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".qfx" {
			continue
		}
		if err := importQFX(ctx, db, filepath.Join(folder, entry.Name()), opts); err != nil {
			return fmt.Errorf("%s: %w", entry.Name(), err)
		}
	}
	return nil
}

func importQFX(ctx context.Context, db *database.DB, path string, opts Options) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	file, err := ParseQFX(f)
	if err != nil {
		return err
	}

	inserted := 0
	if len(file.Trades) > 0 {
		if inserted, err = db.ImportTrades(ctx, file.Account, opts.BaseCurrency, file.Trades); err != nil {
			return err
		}
	}
	fmt.Printf("%s: %d trades parsed, %d new (account %s)\n", filepath.Base(path), len(file.Trades), inserted, file.Account)
	return nil
}

package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

	result := database.ImportResult{}
	if len(file.Trades) > 0 || len(file.Actions) > 0 || len(file.CashEvents) > 0 {
		data := database.ImportData{Trades: file.Trades, Actions: file.Actions, CashEvents: file.CashEvents}
		if result, err = db.Import(ctx, file.Account, opts.BaseCurrency, data); err != nil {
			return err
		}
	}

	fmt.Printf("%s: %d trades parsed, %d new (account %s)", filepath.Base(path), len(file.Trades), result.Trades, file.Account)
	if len(file.Actions) > 0 {
		fmt.Printf("; %d corporate actions, %d new", len(file.Actions), result.Actions)
	}
	if len(file.CashEvents) > 0 {
		fmt.Printf("; %d cash events, %d new", len(file.CashEvents), result.CashEvents)
	}
	fmt.Println()

	for _, s := range file.Skipped {
		fmt.Printf("  skipped: %s\n", s)
	}
	kinds := make([]string, 0, len(file.Ignored))
	for kind := range file.Ignored {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	for _, kind := range kinds {
		fmt.Printf("  not imported yet: %d %s\n", file.Ignored[kind], kind)
	}
	return nil
}

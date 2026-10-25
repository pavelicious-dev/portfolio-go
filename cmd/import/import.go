package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"portfolio/config"
	"portfolio/database"
	"portfolio/ingest"
	"portfolio/services/matching"
	"portfolio/services/settlements"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	folder := config.StatementsDir()
	switch len(os.Args) {
	case 1:
	case 2:
		folder = os.Args[1]
	default:
		log.Fatalf("usage: import [folder with .qfx files, default %s]", folder)
	}

	ctx := context.Background()
	db, err := database.Open(config.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Printf("importing statements from %s\n", folder)
	err = ingest.ImportFolder(ctx, db, folder, ingest.Options{
		BaseCurrency: config.BaseCurrency(),
	})
	if err != nil {
		log.Fatal(err)
	}

	prices, err := settlements.LoadDir(config.CboeDir())
	if err != nil {
		log.Fatal(err)
	}
	if err := matching.Rebuild(ctx, db, prices); err != nil {
		log.Fatalf("match trades: %v", err)
	}
	trips, err := db.GetRoundTrips(ctx, database.RoundTripQuery{})
	if err != nil {
		log.Fatal(err)
	}
	unmatched, _ := db.CountUnmatched(ctx)
	fmt.Printf("matched %d round trips, %d closing trades without opening trade\n", len(trips), unmatched)

	if _, settlement, err := matching.LoadSettledTrades(ctx, db, prices); err == nil && len(settlement.Missing) > 0 {
		fmt.Printf("note: %d trades of cash-settled index options expired without a known settlement price and count as worthless (run: go run ./cmd/settlements, then see: report settlements)\n", settlement.MissingTrades())
	}
}

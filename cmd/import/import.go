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

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	if len(os.Args) != 2 {
		log.Fatal("usage: import <folder with .qfx files>")
	}

	ctx := context.Background()
	db, err := database.Open(config.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = ingest.ImportFolder(ctx, db, os.Args[1], ingest.Options{
		BaseCurrency: config.BaseCurrency(),
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := matching.Rebuild(ctx, db); err != nil {
		log.Fatalf("match trades: %v", err)
	}
	trips, err := db.GetRoundTrips(ctx, database.RoundTripQuery{})
	if err != nil {
		log.Fatal(err)
	}
	unmatched, _ := db.CountUnmatched(ctx)
	fmt.Printf("matched %d round trips, %d closing trades without opening trade\n", len(trips), unmatched)
}

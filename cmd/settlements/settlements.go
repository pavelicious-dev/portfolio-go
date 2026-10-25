// Command settlements downloads the index levels that cash-settled options expire at.
//
// IB's QFX statements book SPX, SPXW, XSP and RUT options that expire in the money
// at a price of 0 and contain no cash settlement. The index history files that
// Cboe publishes are stored in data/cboe, and the round trips are rebuilt with them.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"portfolio/config"
	"portfolio/database"
	"portfolio/services/matching"
	"portfolio/services/settlements"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	dir := config.CboeDir()
	fmt.Printf("downloading index history from Cboe into %s\n", dir)

	client := &http.Client{Timeout: 60 * time.Second}
	files, err := settlements.Download(client, dir)
	for _, file := range files {
		fmt.Printf("  stored %s\n", file)
	}
	if err != nil {
		log.Fatalf("download: %v", err)
	}

	// the prices change the value of expiries, so recompute the round trips
	ctx := context.Background()
	db, err := database.Open(config.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	prices, err := settlements.LoadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	if err := matching.Rebuild(ctx, db, prices); err != nil {
		log.Fatalf("match trades: %v", err)
	}
	_, report, err := matching.LoadSettledTrades(ctx, db, prices)
	if err != nil {
		log.Fatal(err)
	}
	for _, line := range report.Lines(10) {
		fmt.Println(line)
	}
}

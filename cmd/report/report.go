package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"portfolio/config"
	"portfolio/database"
	"portfolio/services/reports"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	if len(os.Args) < 2 {
		log.Fatal("usage: report monthly | report assets [YYYY-MM]")
	}

	ctx := context.Background()
	db, err := database.Open(config.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	trips, err := db.GetRoundTrips(ctx, database.RoundTripQuery{})
	if err != nil {
		log.Fatal(err)
	}

	switch os.Args[1] {
	case "monthly":
		for _, s := range reports.MonthlySummary(trips) {
			fmt.Printf("%s - Stocks: %+9.2f€,\tOptions: %+9.2f€,\tOther: %+9.2f€,\tTotal: %+9.2f€\n",
				s.Month, s.Stocks, s.Options, s.Other, s.Total())
		}
	case "assets":
		month := ""
		if len(os.Args) > 2 {
			month = os.Args[2]
		}
		for _, s := range reports.AssetSummaries(trips, month) {
			fmt.Printf("%-6s %+9.2f€\n", s.Asset, s.Total)
		}
	default:
		log.Fatalf("unknown report %q", os.Args[1])
	}

	if n, err := db.CountUnmatched(ctx); err == nil && n > 0 {
		fmt.Printf("\nnote: %d closing trades have no opening trade in the database (missing statements, or corporate actions such as splits) and are not included\n", n)
	}
}

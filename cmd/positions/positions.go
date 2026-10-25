package main

import (
	"context"
	"log"
	"os"

	"portfolio/config"
	"portfolio/database"
	"portfolio/services/positions"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	if len(os.Args) < 2 {
		log.Fatal("usage: positions monthly|asset [month|asset]")
	}

	db, err := database.Open(config.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	trades, err := db.GetTrades(context.Background(), database.TradeQuery{})
	if err != nil {
		log.Fatal(err)
	}
	allPositions, keys := positions.GetPositions(trades)

	groupBy := os.Args[1]

	monthOrAsset := ""
	if len(os.Args) > 2 {
		monthOrAsset = os.Args[2]
	}

	switch groupBy {
	case "monthly":
		positions.GetMonthlySummary(allPositions, keys, monthOrAsset)
	case "asset":
		positions.GetAssetSummary(allPositions, keys, monthOrAsset)
	}
}

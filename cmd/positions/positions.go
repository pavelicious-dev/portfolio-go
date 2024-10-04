package main

import (
	"os"
	"portfolio/database"
	"portfolio/services/positions"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	database.Connect()
	defer database.Close()

	trades, _ := database.GetTrades(database.TradeQuery{})
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

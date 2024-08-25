package main

import (
	"fmt"
	"portfolio/database"
	"portfolio/models"
	"portfolio/services"
	"sort"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	database.Connect()
	defer database.Close()

	trades, _ := database.GetTrades(database.TradeQuery{})
	positions := services.GetPositions(trades)

	summary := summarizePositions(positions)

	keys := make([]string, 0, len(summary))
	for k := range summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		sum := summary[k]
		fmt.Printf("%s - Stocks: %+9.2f€,\tOptions: %+9.2f€,\tTotal: %+9.2f€\n", k, sum.SumStocks, sum.SumOptions, sum.SumTotal())
	}
}

func summarizePositions(positions map[string]*models.Position) map[string]*MonthlySummary {
	summary := make(map[string]*MonthlySummary)

	keys := make([]string, 0, len(positions))
	for k := range positions {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return positions[keys[i]].OpenedAt.Before(positions[keys[j]].OpenedAt)
	})

	for _, k := range keys {
		pos := positions[k]

		if pos.IsOpen() {
			continue
		}

		var changeStocks = 0.0
		var changeOptions = 0.0

		if pos.IsOptionPosition() {
			changeOptions = pos.Value
		} else {
			changeStocks = pos.Value
		}

		month := fmt.Sprintf("%d-%02d", pos.ClosedAt.Year(), pos.ClosedAt.Month())
		sum, ok := summary[month]

		if ok {
			sum.SumStocks += changeStocks
			sum.SumOptions += changeOptions
		} else {
			summary[month] = &MonthlySummary{
				SumStocks:  changeStocks,
				SumOptions: changeOptions,
			}
		}
	}
	return summary
}

type MonthlySummary struct {
	SumStocks  float64
	SumOptions float64
}

func (m MonthlySummary) SumTotal() float64 {
	return m.SumStocks + m.SumOptions
}

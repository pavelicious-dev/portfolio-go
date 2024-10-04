package positions

import (
	"fmt"
	"maps"
	"portfolio/models"
	"slices"
)

func GetMonthlySummary(positions map[string]*models.Position, keys []string, month string) {
	if month == "" {
		summary := summarizePositions(positions, keys)
		for _, month := range slices.Sorted(maps.Keys(summary)) {
			data := summary[month]
			fmt.Printf("%s - Stocks: %+9.2f€,\tOptions: %+9.2f€,\tTotal: %+9.2f€\n", month, data.SumStocks, data.SumOptions, data.SumTotal())
		}
	} else {
		summary := summarizeMonth(positions, keys, month)
		for _, asset := range slices.Sorted(maps.Keys(summary)) {
			data := summary[asset]
			fmt.Printf("%-5s - %+9.2f€\n", data.Asset, data.Total)
		}
	}
}

func summarizeMonth(positions map[string]*models.Position, keys []string, month string) map[string]*AssetSummary {
	assets := make(map[string]*AssetSummary)

	for _, k := range keys {
		pos := positions[k]

		if pos.IsOpen() {
			continue
		}

		posMonth := fmt.Sprintf("%d-%02d", pos.ClosedAt().Year(), pos.ClosedAt().Month())
		if posMonth != month {
			continue
		}

		assetName := pos.GetAssetName()
		realized, _ := pos.GetProfit()

		summary, ok := assets[assetName]
		if ok {
			summary.Total += realized
		} else {
			assets[assetName] = &AssetSummary{
				Asset: assetName,
				Total: realized,
			}
		}
	}
	return assets
}

func summarizePositions(positions map[string]*models.Position, keys []string) map[string]*MonthlySummary {
	summary := make(map[string]*MonthlySummary)

	for _, k := range keys {
		pos := positions[k]

		if pos.IsOpen() {
			continue
		}

		var changeStocks = 0.0
		var changeOptions = 0.0

		realized, _ := pos.GetProfit()

		if pos.IsOptionPosition() {
			changeOptions = realized
		} else {
			changeStocks = realized
		}

		month := fmt.Sprintf("%d-%02d", pos.ClosedAt().Year(), pos.ClosedAt().Month())
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

type AssetSummary struct {
	Asset string
	Total float64
}

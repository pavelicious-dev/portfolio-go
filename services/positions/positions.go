package positions

import (
	"maps"
	"portfolio/models"
	"slices"
)

func GetPositions(trades []models.Trade) (map[string]*models.Position, []string) {
	positions := make(map[string]*models.Position)

	for _, trade := range trades {
		existing, ok := positions[trade.Asset]
		var pos *models.Position

		if ok {
			pos = existing
		} else {
			pos = &models.Position{Asset: trade.Asset}
			positions[trade.Asset] = pos
		}
		pos.AddTrade(trade)
	}

	keys := slices.SortedStableFunc(maps.Keys(positions), func(a, b string) int {
		return positions[a].OpenedAt().Compare(positions[b].OpenedAt())
	})

	return positions, keys
}

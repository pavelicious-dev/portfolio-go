package positions

import (
	"maps"
	"portfolio/models"
	"slices"
)

func GetPositions(trades []models.Trade) (map[string]*models.Position, []string) {
	positions := make(map[string]*models.Position)

	for _, trade := range trades {
		name := trade.Instrument.Name()
		pos, ok := positions[name]

		if !ok {
			pos = &models.Position{Instrument: trade.Instrument}
			positions[name] = pos
		}
		pos.AddTrade(trade)
	}

	keys := slices.SortedStableFunc(maps.Keys(positions), func(a, b string) int {
		return positions[a].OpenedAt().Compare(positions[b].OpenedAt())
	})

	return positions, keys
}

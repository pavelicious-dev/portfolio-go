package services

import (
	"portfolio/models"
)

func GetPositions(trades []models.Trade) map[string]*models.Position {
	positions := make(map[string]*models.Position)

	for _, trade := range trades {
		pos, ok := positions[trade.Asset]

		if ok {
			pos.Units += trade.Units
			pos.Value += trade.Total

			if pos.Units == 0 {
				pos.ClosedAt = trade.Date
			}
		} else {
			positions[trade.Asset] = &models.Position{
				Asset:    trade.Asset,
				OpenedAt: trade.Date,
				Units:    trade.Units,
				Value:    trade.Total,
			}
		}
	}
	return positions
}

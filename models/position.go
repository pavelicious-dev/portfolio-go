package models

import (
	"strings"
	"time"
)

type Position struct {
	Asset string

	openedAt   time.Time
	closedAt   time.Time
	units      float64
	realized   float64
	unrealized float64
}

func (p *Position) AddTrade(trade Trade) {
	if p.openedAt.IsZero() {
		p.openedAt = trade.Date
	}

	p.units += trade.Units

	if p.units == 0 {
		p.closedAt = trade.Date
		p.realized += p.unrealized + trade.Total
		p.unrealized = 0
	} else {
		p.closedAt = time.Time{}
		p.unrealized += trade.Total
	}
}

func (p Position) OpenedAt() time.Time {
	return p.openedAt
}

func (p Position) ClosedAt() time.Time {
	return p.closedAt
}

func (p Position) IsOpen() bool {
	return p.closedAt.IsZero()
}

func (p Position) DaysOpen() int {
	if p.IsOpen() {
		return int(time.Since(p.openedAt).Hours() / 24)
	}
	return int(p.closedAt.Sub(p.openedAt).Hours() / 24)
}

func (p Position) IsOptionPosition() bool {
	return len(strings.Fields(p.Asset)) == 4
}

func (p Position) GetAssetName() string {
	if p.IsOptionPosition() {
		return strings.Fields(p.Asset)[0]
	}
	return p.Asset
}

func (p Position) GetProfit() (float64, float64) {
	return p.realized, p.unrealized
}

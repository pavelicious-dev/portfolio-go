package models

import (
	"time"
)

// Position accumulates all trades of one instrument.
// TODO: replaced by per round trip matching.
type Position struct {
	Instrument Instrument

	openedAt   time.Time
	closedAt   time.Time
	units      Fixed
	realized   float64
	unrealized float64
}

func (p *Position) AddTrade(trade Trade) {
	if p.openedAt.IsZero() {
		p.openedAt = trade.TradedAt
	}

	p.units += trade.Quantity

	if p.units == 0 {
		p.closedAt = trade.TradedAt
		p.realized += p.unrealized + trade.CashBase()
		p.unrealized = 0
	} else {
		p.closedAt = time.Time{}
		p.unrealized += trade.CashBase()
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
	return p.Instrument.IsOption()
}

func (p Position) GetAssetName() string {
	return p.Instrument.Underlying
}

func (p Position) GetProfit() (float64, float64) {
	return p.realized, p.unrealized
}

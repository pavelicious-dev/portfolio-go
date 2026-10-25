package models

import "time"

type Direction string

const (
	Long  Direction = "LONG"
	Short Direction = "SHORT"
)

// RoundTrip is a sequence of trades in one instrument from flat to flat.
type RoundTrip struct {
	ID           int64      `json:"id"`
	AccountID    int64      `json:"accountId"`
	Instrument   Instrument `json:"instrument"`
	Direction    Direction  `json:"direction"`
	OpenedAt     time.Time  `json:"openedAt"`
	ClosedAt     time.Time  `json:"closedAt,omitzero"` // zero while still open
	MaxQuantity  Fixed      `json:"maxQuantity"`       // largest absolute size held
	OpenQuantity Fixed      `json:"openQuantity"`      // absolute size still open
	Matches      []LotMatch `json:"matches"`
}

// LotMatch is a part of an opening trade closed by (a part of) a closing trade.
type LotMatch struct {
	OpenTradeID  int64     `json:"openTradeId"`
	CloseTradeID int64     `json:"closeTradeId"`
	OpenedAt     time.Time `json:"openedAt"`
	ClosedAt     time.Time `json:"closedAt"`
	Quantity     Fixed     `json:"quantity"` // absolute
	PnL          Fixed     `json:"pnl"`      // instrument currency, net of fees
	Fees         Fixed     `json:"fees"`
	PnLBase      float64   `json:"pnlBase"` // each leg converted at its own trade rate
}

func (r RoundTrip) IsOpen() bool {
	return r.OpenQuantity != 0
}

// RealizedPnL sums the matches closed so far, in instrument currency.
func (r RoundTrip) RealizedPnL() Fixed {
	var sum Fixed
	for _, m := range r.Matches {
		sum += m.PnL
	}
	return sum
}

func (r RoundTrip) RealizedBase() float64 {
	var sum float64
	for _, m := range r.Matches {
		sum += m.PnLBase
	}
	return sum
}

func (r RoundTrip) Fees() Fixed {
	var sum Fixed
	for _, m := range r.Matches {
		sum += m.Fees
	}
	return sum
}

// Unmatched is (part of) a closing trade for which no opening trade was found,
// typically because the statement that opened the position is not imported.
type Unmatched struct {
	Trade    Trade `json:"trade"`
	Quantity Fixed `json:"quantity"` // absolute
}

type MatchResult struct {
	RoundTrips []RoundTrip
	Unmatched  []Unmatched
}

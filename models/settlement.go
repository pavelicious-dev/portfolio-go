package models

import "time"

// SettlementPrice is the value an index was settled at on a day. Cash-settled
// index options (SPX, SPXW, XSP, ...) pay the difference to the strike in cash
// at expiry, which IB's QFX export does not contain.
type SettlementPrice struct {
	Index  string    `json:"index"` // SPX, XSP, RUT, ...
	Day    time.Time `json:"day"`
	Price  float64   `json:"price"`
	Source string    `json:"source"`
}

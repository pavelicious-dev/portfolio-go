package models

import (
	"fmt"
	"time"
)

type InstrumentType string

const (
	Stock        InstrumentType = "STK"
	Option       InstrumentType = "OPT"
	Future       InstrumentType = "FUT"
	FutureOption InstrumentType = "FOP"
	CFD          InstrumentType = "CFD"
)

type Right string

const (
	Call Right = "C"
	Put  Right = "P"
)

// Instrument is a tradable contract. Option fields are zero for other types.
type Instrument struct {
	ID         int64          `json:"id"`
	Type       InstrumentType `json:"type"`
	Symbol     string         `json:"symbol"` // unique per type+currency; OCC symbol for options
	Underlying string         `json:"underlying"`
	Currency   string         `json:"currency"`
	Multiplier int64          `json:"multiplier"`
	Expiry     time.Time      `json:"expiry,omitzero"`
	Strike     Fixed          `json:"strike,omitempty"`
	Right      Right          `json:"right,omitempty"`
}

func (i Instrument) IsOption() bool {
	return i.Type == Option || i.Type == FutureOption
}

// Name is a human readable identifier, e.g. "AAPL 2023-01-20 145 C".
func (i Instrument) Name() string {
	if i.Type == Option && !i.Expiry.IsZero() {
		return fmt.Sprintf("%s %s %s %s", i.Underlying, i.Expiry.Format(time.DateOnly), i.Strike, i.Right)
	}
	return i.Symbol
}

// OCCSymbol builds the 21 character OCC option symbol, e.g. "AAPL  230120C00145000".
func OCCSymbol(underlying string, expiry time.Time, right Right, strike Fixed) string {
	return fmt.Sprintf("%-6s%s%s%08d", underlying, expiry.Format("060102"), right, int64(strike.Float()*1000+0.5))
}

package models

import "time"

type TradeKind string

const (
	KindTrade    TradeKind = "TRADE"
	KindExpire   TradeKind = "EXPIRE"
	KindAssign   TradeKind = "ASSIGN"
	KindExercise TradeKind = "EXERCISE"
)

// Trade is one imported fill. It is immutable once imported.
type Trade struct {
	ID         int64      `json:"id"`
	AccountID  int64      `json:"accountId"`
	Instrument Instrument `json:"instrument"`
	Source     string     `json:"source"`   // import format, "qfx"
	SourceID   string     `json:"sourceId"` // broker transaction id, unique per account
	Kind       TradeKind  `json:"kind"`
	TradedAt   time.Time  `json:"tradedAt"`
	Quantity   Fixed      `json:"quantity"`   // signed: buy > 0, sell < 0
	Price      Fixed      `json:"price"`      // per unit, in instrument currency
	Commission Fixed      `json:"commission"` // positive = cost, in instrument currency
	FxToBase   float64    `json:"fxToBase"`   // base currency per 1 unit of instrument currency, 0 = unknown
}

// Cash is the signed cash flow in instrument currency, net of commission:
// buys are negative, sells positive.
func (t Trade) Cash() Fixed {
	gross := -t.Quantity.Float() * t.Price.Float() * float64(t.Instrument.Multiplier)
	return FixedFromFloat(gross) - t.Commission
}

// CashBase is Cash converted to the account's base currency.
// Falls back to the unconverted amount if the rate is unknown.
func (t Trade) CashBase() float64 {
	if t.FxToBase <= 0 {
		return t.Cash().Float()
	}
	return t.Cash().Float() * t.FxToBase
}

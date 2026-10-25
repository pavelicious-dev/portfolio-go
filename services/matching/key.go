package matching

import "portfolio/models"

// groupKey identifies what trades are matched against each other.
type groupKey struct {
	account    int64
	instrument int64 // database id; zero for trades not read from the database
	kind       models.InstrumentType
	symbol     string
	currency   string
}

func keyOf(t models.Trade) groupKey {
	if t.Instrument.ID != 0 {
		return groupKey{account: t.AccountID, instrument: t.Instrument.ID}
	}
	return groupKey{account: t.AccountID, kind: t.Instrument.Type, symbol: t.Instrument.Symbol, currency: t.Instrument.Currency}
}

// effectRank orders trades with identical timestamps. IB books assignments,
// exercises and expiries at one timestamp (16:20): opening legs go first, then
// flagged closing legs, and unflagged legs (the stock side of an exercise)
// last, so they cannot take over a close meant for an older position.
func effectRank(e models.Effect) int {
	switch e {
	case models.EffectOpen:
		return 0
	case models.EffectClose:
		return 1
	}
	return 2
}

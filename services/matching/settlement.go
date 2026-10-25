package matching

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	_ "time/tzdata"

	"portfolio/models"
)

// settlingIndex maps the root symbol of a cash-settled option to the index
// whose settlement price decides its value at expiry. Monthly SPX options settle
// on the opening value of the day (Cboe's "SET"), the weekly and daily ones on
// the close.
var settlingIndex = map[string]string{
	"SPX": "SET", "SPXW": "SPX",
	"XSP": "XSP",
	"RUT": "RUT", "RUTW": "RUT",
	"NDX": "NDX", "NDXP": "NDX",
}

// SettlingIndex reports which index settles the options of a root symbol, if they are cash-settled.
func SettlingIndex(root string) (string, bool) {
	index, ok := settlingIndex[root]
	return index, ok
}

var newYork = func() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return loc
}()

// MissingSettlement is a day on which cash-settled options expired and no settlement price is known.
type MissingSettlement struct {
	Index  string
	Day    time.Time
	Trades int
}

// IndexSettlement sums up the settled expiries of one index.
type IndexSettlement struct {
	Index      string
	Expiries   int     // expiry trades that got a settlement price
	InTheMoney int     // of those, trades settled for a non-zero amount
	Cash       float64 // settlement cash in base currency
}

type SettlementReport struct {
	Indexes []IndexSettlement
	Missing []MissingSettlement
}

// Settle prices the expiries of cash-settled index options.
//
// IB books these at expiry like worthless options, at a price of 0, and the QFX
// export contains no cash settlement. An option that expired in the money is worth
// its intrinsic value against the settlement price of the index, which is what the
// closing trade gets as its price. Expiries on days without a known settlement
// price are left at 0 and reported. The input is not modified.
func Settle(trades []models.Trade, prices []models.SettlementPrice) ([]models.Trade, SettlementReport) {
	known := map[string]float64{}
	for _, p := range prices {
		known[p.Index+"|"+p.Day.Format(time.DateOnly)] = p.Price
	}
	lookup := func(index, day string) (float64, bool) {
		if price, ok := known[index+"|"+day]; ok {
			return price, true
		}
		if index == "XSP" { // XSP is a tenth of SPX
			if price, ok := known["SPX|"+day]; ok {
				return price / 10, true
			}
		}
		return 0, false
	}

	settled := slices.Clone(trades)
	totals := map[string]*IndexSettlement{}
	missing := map[string]*MissingSettlement{}

	for i, t := range settled {
		index, ok := settlingIndex[t.Instrument.Underlying]
		if !ok || t.Instrument.Type != models.Option || t.Price != 0 || t.Kind == models.KindAssign || t.Kind == models.KindExercise {
			continue
		}
		if t.Instrument.Expiry.IsZero() || t.TradedAt.In(newYork).Format(time.DateOnly) != t.Instrument.Expiry.Format(time.DateOnly) {
			continue
		}

		day := t.Instrument.Expiry.Format(time.DateOnly)
		level, found := lookup(index, day)
		if !found {
			m, ok := missing[index+"|"+day]
			if !ok {
				m = &MissingSettlement{Index: index, Day: t.Instrument.Expiry}
				missing[index+"|"+day] = m
			}
			m.Trades++
			continue
		}

		value := intrinsic(t.Instrument, level)
		t.Price = models.FixedFromFloat(value)
		settled[i] = t

		s, ok := totals[index]
		if !ok {
			s = &IndexSettlement{Index: index}
			totals[index] = s
		}
		s.Expiries++
		if value > 0 {
			s.InTheMoney++
			s.Cash += t.CashBase()
		}
	}

	var report SettlementReport
	for _, s := range totals {
		report.Indexes = append(report.Indexes, *s)
	}
	slices.SortFunc(report.Indexes, func(a, b IndexSettlement) int { return cmp.Compare(a.Index, b.Index) })
	for _, m := range missing {
		report.Missing = append(report.Missing, *m)
	}
	slices.SortFunc(report.Missing, func(a, b MissingSettlement) int {
		return cmp.Or(cmp.Compare(a.Index, b.Index), a.Day.Compare(b.Day))
	})
	return settled, report
}

// intrinsic is the value of an option per unit at the given level of its underlying.
func intrinsic(in models.Instrument, level float64) float64 {
	strike := in.Strike.Float()
	switch in.Right {
	case models.Call:
		return max(level-strike, 0)
	case models.Put:
		return max(strike-level, 0)
	}
	return 0
}

// MissingTrades is the number of expiry trades that could not be priced.
func (r SettlementReport) MissingTrades() int {
	n := 0
	for _, m := range r.Missing {
		n += m.Trades
	}
	return n
}

// Lines describes the report for output: what was priced and the days that are missing.
// At most limit missing days are listed, all of them if limit is 0.
func (r SettlementReport) Lines(limit int) []string {
	var lines []string
	for _, s := range r.Indexes {
		lines = append(lines, fmt.Sprintf("%-4s %5d expiries priced, %4d in the money, settlement cash %+.2f€", s.Index, s.Expiries, s.InTheMoney, s.Cash))
	}

	if len(r.Missing) > 0 {
		lines = append(lines, fmt.Sprintf("%d expiry days with %d trades have no settlement price and count as worthless:", len(r.Missing), r.MissingTrades()))
		for i, m := range r.Missing {
			if limit > 0 && i == limit {
				lines = append(lines, fmt.Sprintf("  ... and %d more", len(r.Missing)-limit))
				break
			}
			lines = append(lines, fmt.Sprintf("  %-4s %s  %d trades", m.Index, m.Day.Format(time.DateOnly), m.Trades))
		}
	}
	return lines
}

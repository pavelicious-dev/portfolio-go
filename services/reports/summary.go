package reports

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"portfolio/models"
)

// Period selects a year or a month. The zero value covers all time.
type Period struct {
	Year  int
	Month int // 0 = whole year
}

// ParsePeriod reads "", "2024" or "2024-03".
func ParsePeriod(s string) (Period, error) {
	if s == "" {
		return Period{}, nil
	}

	yearPart, monthPart, hasMonth := strings.Cut(s, "-")
	year, err := strconv.Atoi(yearPart)
	if err != nil || year < 1990 || year > 2200 {
		return Period{}, fmt.Errorf("invalid period %q, use YYYY or YYYY-MM", s)
	}
	if !hasMonth {
		return Period{Year: year}, nil
	}

	month, err := strconv.Atoi(monthPart)
	if err != nil || month < 1 || month > 12 {
		return Period{}, fmt.Errorf("invalid period %q, use YYYY or YYYY-MM", s)
	}
	return Period{Year: year, Month: month}, nil
}

func (p Period) String() string {
	switch {
	case p.Year == 0:
		return "all time"
	case p.Month == 0:
		return strconv.Itoa(p.Year)
	}
	return fmt.Sprintf("%d-%02d", p.Year, p.Month)
}

// Contains is evaluated in the reporting time zone, like all monthly figures.
func (p Period) Contains(t time.Time) bool {
	if p.Year == 0 {
		return true
	}
	t = t.In(location)
	return t.Year() == p.Year && (p.Month == 0 || int(t.Month()) == p.Month)
}

// TripResult is a closed round trip with its realized result in base currency.
type TripResult struct {
	Instrument string
	Direction  models.Direction
	Quantity   models.Fixed // largest size held
	OpenedAt   time.Time
	ClosedAt   time.Time
	PnL        float64 // base currency
	Currency   string  // of the instrument
	PnLLocal   float64 // in Currency
}

func newTripResult(rt models.RoundTrip) TripResult {
	return TripResult{
		Instrument: rt.Instrument.Name(),
		Direction:  rt.Direction,
		Quantity:   rt.MaxQuantity,
		OpenedAt:   rt.OpenedAt,
		ClosedAt:   rt.ClosedAt,
		PnL:        rt.RealizedBase(),
		Currency:   rt.Instrument.Currency,
		PnLLocal:   rt.RealizedPnL().Float(),
	}
}

func (r TripResult) Days() int {
	return int(r.ClosedAt.Sub(r.OpenedAt).Hours() / 24)
}

// MonthTotal is the realized result of one month.
type MonthTotal struct {
	Month   string
	Stocks  float64
	Options float64
	Other   float64
}

func (m MonthTotal) PnL() float64 {
	return m.Stocks + m.Options + m.Other
}

// Summary describes the realized results of one underlying, or of all of them, in base currency.
type Summary struct {
	Ticker string // empty for all underlyings
	Period Period

	// Realized is booked per lot on the day it was closed, like the monthly report
	Realized float64
	Stocks   float64
	Options  float64
	Other    float64
	Months   []MonthTotal
	Tickers  []AssetSummary // realized result per underlying, best first

	// the statistics cover round trips that were completely closed in the period
	Trips     int
	Wins      int
	Losses    int
	GrossWin  float64
	GrossLoss float64 // negative
	AvgDays   float64
	Best      []TripResult // best first
	Worst     []TripResult // worst first
	OpenTrips int          // round trips still open, regardless of the period
	Contracts int          // different contracts with a lot closed in the period
}

func (s Summary) WinRate() float64 {
	if s.Wins+s.Losses == 0 {
		return 0
	}
	return float64(s.Wins) / float64(s.Wins+s.Losses)
}

func (s Summary) AvgWin() float64 {
	if s.Wins == 0 {
		return 0
	}
	return s.GrossWin / float64(s.Wins)
}

func (s Summary) AvgLoss() float64 {
	if s.Losses == 0 {
		return 0
	}
	return s.GrossLoss / float64(s.Losses)
}

// ProfitFactor is gross profit over gross loss; zero if there were no losses.
func (s Summary) ProfitFactor() float64 {
	if s.GrossLoss == 0 {
		return 0
	}
	return s.GrossWin / -s.GrossLoss
}

const topTrips = 3

// Summarize summarizes one underlying over the given period; an empty ticker covers all of them.
func Summarize(trips []models.RoundTrip, ticker string, period Period) Summary {
	s := Summary{Ticker: strings.ToUpper(ticker), Period: period}

	months := map[string]*MonthTotal{}
	tickers := map[string]float64{}
	contracts := map[int64]bool{}
	var results []TripResult
	var days float64

	for _, rt := range trips {
		if s.Ticker != "" && rt.Instrument.Underlying != s.Ticker {
			continue
		}

		for _, m := range rt.Matches {
			if !period.Contains(m.ClosedAt) {
				continue
			}
			s.Realized += m.PnLBase
			tickers[rt.Instrument.Underlying] += m.PnLBase
			contracts[rt.Instrument.ID] = true

			key := Month(m.ClosedAt)
			month, ok := months[key]
			if !ok {
				month = &MonthTotal{Month: key}
				months[key] = month
			}

			switch rt.Instrument.Type {
			case models.Stock:
				s.Stocks += m.PnLBase
				month.Stocks += m.PnLBase
			case models.Option, models.FutureOption:
				s.Options += m.PnLBase
				month.Options += m.PnLBase
			default:
				s.Other += m.PnLBase
				month.Other += m.PnLBase
			}
		}

		if rt.IsOpen() {
			s.OpenTrips++
			continue
		}
		if len(rt.Matches) == 0 || !period.Contains(rt.ClosedAt) {
			continue
		}

		result := newTripResult(rt)
		pnl := result.PnL
		results = append(results, result)
		days += float64(result.Days())

		switch {
		case pnl > 0:
			s.Wins++
			s.GrossWin += pnl
		case pnl < 0:
			s.Losses++
			s.GrossLoss += pnl
		}
	}

	s.Trips = len(results)
	s.Contracts = len(contracts)
	if s.Trips > 0 {
		s.AvgDays = days / float64(s.Trips)
	}

	for _, m := range months {
		s.Months = append(s.Months, *m)
	}
	slices.SortFunc(s.Months, func(a, b MonthTotal) int { return cmp.Compare(a.Month, b.Month) })

	for ticker, total := range tickers {
		s.Tickers = append(s.Tickers, AssetSummary{ticker, total})
	}
	slices.SortFunc(s.Tickers, func(a, b AssetSummary) int {
		return cmp.Or(cmp.Compare(b.Total, a.Total), cmp.Compare(a.Asset, b.Asset))
	})

	slices.SortStableFunc(results, func(a, b TripResult) int { return cmp.Compare(b.PnL, a.PnL) })
	s.Best = results[:min(topTrips, len(results))]
	for i := len(results) - 1; i >= 0 && len(s.Worst) < topTrips; i-- {
		s.Worst = append(s.Worst, results[i])
	}
	// only list a result once, and only if it really is a win or loss
	s.Best = slices.DeleteFunc(slices.Clone(s.Best), func(r TripResult) bool { return r.PnL <= 0 })
	s.Worst = slices.DeleteFunc(s.Worst, func(r TripResult) bool { return r.PnL >= 0 })

	return s
}

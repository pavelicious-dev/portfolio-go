package reports

import (
	"cmp"
	"slices"
	"strings"

	"portfolio/models"
)

// CashTotals adds up cash bookings in base currency. Deposits and withdrawals
// are not income; everything else is, with costs negative.
type CashTotals struct {
	Deposits    float64
	Withdrawals float64 // negative
	Dividends   float64
	DividendTax float64 // negative
	Interest    float64
	BorrowFees  float64 // negative
	InterestTax float64 // negative
	DataFees    float64 // net of refunds
	Fees        float64
}

// Income is everything earned or paid besides trading: dividends, interest, taxes and fees.
func (c CashTotals) Income() float64 {
	return c.Dividends + c.DividendTax + c.Interest + c.BorrowFees + c.InterestTax + c.DataFees + c.Fees
}

func (c CashTotals) NetDeposits() float64 {
	return c.Deposits + c.Withdrawals
}

func (c *CashTotals) add(e models.CashEvent) {
	amount := e.AmountBase()

	switch e.Category {
	case models.CashDeposit:
		c.Deposits += amount
	case models.CashWithdrawal:
		c.Withdrawals += amount
	case models.CashDividend:
		c.Dividends += amount
	case models.CashDividendTax:
		c.DividendTax += amount
	case models.CashInterest:
		c.Interest += amount
	case models.CashBorrowFee:
		c.BorrowFees += amount
	case models.CashInterestTax:
		c.InterestTax += amount
	case models.CashDataFee:
		c.DataFees += amount
	case models.CashFee:
		c.Fees += amount
	}
	// currency conversions move money between currencies and change nothing here
}

type CashMonth struct {
	Month string
	CashTotals
}

type CashSummary struct {
	Ticker string
	Period Period
	CashTotals
	Months []CashMonth
}

// SummarizeCash adds up the bookings of a period, in base currency. A ticker
// restricts it to the bookings of that underlying, such as its dividends.
func SummarizeCash(events []models.CashEvent, ticker string, period Period) CashSummary {
	s := CashSummary{Ticker: strings.ToUpper(ticker), Period: period}
	months := map[string]*CashMonth{}

	for _, e := range events {
		if !period.Contains(e.At) || (s.Ticker != "" && e.Symbol != s.Ticker) {
			continue
		}
		s.add(e)

		key := Month(e.At)
		m, ok := months[key]
		if !ok {
			m = &CashMonth{Month: key}
			months[key] = m
		}
		m.add(e)
	}

	for _, m := range months {
		s.Months = append(s.Months, *m)
	}
	slices.SortFunc(s.Months, func(a, b CashMonth) int { return cmp.Compare(a.Month, b.Month) })
	return s
}

// OverviewMonth combines the trading result of a month with its income.
type OverviewMonth struct {
	MonthTotal         // trading: stocks, options and other
	Income     float64 // dividends, interest, taxes and fees
}

// Total is the trading result plus income.
func (m OverviewMonth) Total() float64 {
	return m.MonthTotal.PnL() + m.Income
}

// MergeMonths joins trading and cash figures by month, oldest first.
func MergeMonths(trading []MonthTotal, cash []CashMonth) []OverviewMonth {
	months := map[string]*OverviewMonth{}
	get := func(month string) *OverviewMonth {
		m, ok := months[month]
		if !ok {
			m = &OverviewMonth{MonthTotal: MonthTotal{Month: month}}
			months[month] = m
		}
		return m
	}

	for _, t := range trading {
		get(t.Month).MonthTotal = t
	}
	for _, c := range cash {
		get(c.Month).Income = c.Income()
	}

	result := make([]OverviewMonth, 0, len(months))
	for _, m := range months {
		result = append(result, *m)
	}
	slices.SortFunc(result, func(a, b OverviewMonth) int { return cmp.Compare(a.Month, b.Month) })
	return result
}

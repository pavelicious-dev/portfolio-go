package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"portfolio/config"
	"portfolio/database"
	"portfolio/models"
	"portfolio/services/matching"
	"portfolio/services/reports"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	if len(os.Args) < 2 {
		log.Fatal("usage: report monthly | report assets [YYYY-MM] | report open [SYMBOL] | report unmatched [SYMBOL] | report ticker SYMBOL [YYYY|YYYY-MM] | report summary [YYYY|YYYY-MM] | report cash [YYYY|YYYY-MM]")
	}

	ctx := context.Background()
	db, err := database.Open(config.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	trips, err := db.GetRoundTrips(ctx, database.RoundTripQuery{})
	if err != nil {
		log.Fatal(err)
	}

	events, err := db.GetCashEvents(ctx)
	if err != nil {
		log.Fatal(err)
	}

	switch os.Args[1] {
	case "monthly":
		all := reports.Summarize(trips, "", reports.Period{})
		cash := reports.SummarizeCash(events, "", reports.Period{})
		printMonths(reports.MergeMonths(all.Months, cash.Months))
	case "assets":
		month := ""
		if len(os.Args) > 2 {
			month = os.Args[2]
		}
		for _, s := range reports.AssetSummaries(trips, month) {
			fmt.Printf("%-6s %+9.2f€\n", s.Asset, s.Total)
		}
	case "open":
		symbol := ""
		if len(os.Args) > 2 {
			symbol = os.Args[2]
		}
		trades, err := matching.LoadTrades(ctx, db)
		if err != nil {
			log.Fatal(err)
		}
		asOf := time.Time{}
		for _, t := range trades {
			if t.TradedAt.After(asOf) {
				asOf = t.TradedAt
			}
		}
		printOpen(reports.OpenPositions(trips, trades, symbol), asOf)
	case "ticker":
		if len(os.Args) < 3 {
			log.Fatal("usage: report ticker SYMBOL [YYYY | YYYY-MM]")
		}
		period := reports.Period{}
		if len(os.Args) > 3 {
			var err error
			if period, err = reports.ParsePeriod(os.Args[3]); err != nil {
				log.Fatal(err)
			}
		}
		printTicker(trips, reports.Summarize(trips, os.Args[2], period), reports.SummarizeCash(events, os.Args[2], period))
	case "summary":
		period := reports.Period{}
		if len(os.Args) > 2 {
			var err error
			if period, err = reports.ParsePeriod(os.Args[2]); err != nil {
				log.Fatal(err)
			}
		}
		printSummary(reports.Summarize(trips, "", period), reports.SummarizeCash(events, "", period))
	case "cash":
		period := reports.Period{}
		if len(os.Args) > 2 {
			var err error
			if period, err = reports.ParsePeriod(os.Args[2]); err != nil {
				log.Fatal(err)
			}
		}
		printCash(reports.SummarizeCash(events, "", period))
		return // trades are not part of this report
	case "unmatched":
		symbol := ""
		if len(os.Args) > 2 {
			symbol = strings.ToUpper(os.Args[2])
		}
		unmatched, err := db.GetUnmatched(ctx)
		if err != nil {
			log.Fatal(err)
		}
		printUnmatched(unmatched, symbol)
		return // the trailing note would repeat this list
	default:
		log.Fatalf("unknown report %q", os.Args[1])
	}

	if n, err := db.CountUnmatched(ctx); err == nil && n > 0 {
		fmt.Printf("\nnote: %d closing trades have no opening trade in the database (missing statements, or corporate actions such as splits) and are not included\n", n)
	}
}

// printOpen lists positions as of the given date, normally that of the last imported trade.
func printOpen(positions []reports.OpenPosition, asOf time.Time) {
	if len(positions) == 0 {
		fmt.Println("no open positions")
		return
	}

	fmt.Printf("open positions as of %s (last imported trade)\n\n", asOf.Format(time.DateOnly))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "Instrument\tSide\tQty\tAvg price\tCost\tCcy\tOpened\tDays\tExpiry\tRealized so far\t")

	for _, p := range positions {
		sign := ""
		if p.Direction == models.Short {
			sign = "-"
		}

		expiry := "-"
		if days, ok := p.DaysToExpiry(asOf); ok {
			expiry = fmt.Sprintf("%s (%dd)", p.Instrument.Expiry.Format(time.DateOnly), days)
			if days < 0 {
				expiry = fmt.Sprintf("%s EXPIRED?", p.Instrument.Expiry.Format(time.DateOnly))
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s%s\t%.4f\t%.2f\t%s\t%s\t%d\t%s\t%+.2f€\t\n",
			p.Instrument.Name(), p.Direction, sign, p.Quantity, p.AvgPrice, p.Cost, p.Instrument.Currency,
			p.OpenedAt.Format(time.DateOnly), p.DaysOpen(asOf), expiry, p.Realized)
	}
	w.Flush()
}

// printUnmatched lists closing trades for which no opening trade was found.
func printUnmatched(unmatched []models.Unmatched, symbol string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "Date\tInstrument\tSide\tTrade qty\tUnmatched\tPrice\tCcy\tFlag\t")

	count := 0
	for _, u := range unmatched {
		t := u.Trade
		if symbol != "" && t.Instrument.Underlying != symbol {
			continue
		}
		count++

		side := "BUY"
		if t.Quantity < 0 {
			side = "SELL"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%.4f\t%s\t%s\t\n",
			t.TradedAt.Format(time.DateOnly), t.Instrument.Name(), side, t.Quantity.Abs(), u.Quantity,
			t.Price.Float(), t.Instrument.Currency, t.Effect)
	}

	if count == 0 {
		fmt.Println("no unmatched closing trades")
		return
	}
	w.Flush()
	fmt.Printf("\n%d closing trades have no opening trade: the opening statement is not imported, or the position\n"+
		"came from a corporate action (split, symbol change) that the QFX files do not contain.\n", count)
}

func printTotals(s reports.Summary) {
	fmt.Printf("Realized P&L     %+10.2f€   (stocks %+.2f€, options %+.2f€, other %+.2f€)\n", s.Realized, s.Stocks, s.Options, s.Other)
}

func printTripStats(s reports.Summary) {
	if s.Trips == 0 {
		fmt.Println("\nno round trips were closed in this period")
		return
	}

	fmt.Printf("\nClosed round trips %8d   (%d wins, %d losses)\n", s.Trips, s.Wins, s.Losses)
	fmt.Printf("Win rate         %9.1f%%\n", s.WinRate()*100)
	fmt.Printf("Average win      %+10.2f€\n", s.AvgWin())
	fmt.Printf("Average loss     %+10.2f€\n", s.AvgLoss())
	if s.Losses > 0 {
		fmt.Printf("Profit factor    %10.2f\n", s.ProfitFactor())
	}
	fmt.Printf("Average holding  %7.1f days\n", s.AvgDays)

	if len(s.Best) > 0 || len(s.Worst) > 0 {
		printTrips("Best trades", s.Best)
		printTrips("Worst trades", s.Worst)
	}
}

const topTickers = 5

func printTickers(title string, tickers []reports.AssetSummary, include func(float64) bool) {
	var lines []string
	for _, t := range tickers {
		if len(lines) == topTickers {
			break
		}
		if include(t.Total) {
			lines = append(lines, fmt.Sprintf("  %+10.2f€  %s", t.Total, t.Asset))
		}
	}
	if len(lines) == 0 {
		return
	}
	fmt.Printf("\n%s\n%s\n", title, strings.Join(lines, "\n"))
}

func reversed(tickers []reports.AssetSummary) []reports.AssetSummary {
	out := slices.Clone(tickers)
	slices.Reverse(out)
	return out
}

func printTrips(title string, trips []reports.TripResult) {
	if len(trips) == 0 {
		return
	}
	fmt.Printf("\n%s\n", title)
	for _, t := range trips {
		fmt.Printf("  %+10.2f€  %-26s %-5s %s to %s (%dd)\n", t.PnL, t.Instrument, t.Direction,
			t.OpenedAt.Format(time.DateOnly), t.ClosedAt.Format(time.DateOnly), t.Days())
	}
}

// printTicker prints the summary of one underlying.
func printTicker(trips []models.RoundTrip, s reports.Summary, cash reports.CashSummary) {
	known := false
	for _, rt := range trips {
		if rt.Instrument.Underlying == s.Ticker {
			known = true
			break
		}
	}
	if !known {
		fmt.Printf("no trades found for %s\n", s.Ticker)
		return
	}

	fmt.Printf("%s - %s\n\n", s.Ticker, s.Period)
	printTotals(s)
	if cash.Dividends != 0 || cash.DividendTax != 0 || cash.Fees != 0 {
		fmt.Printf("Dividends        %+10.2f€   (tax %+.2f€, fees %+.2f€)\n", cash.Dividends, cash.DividendTax, cash.Fees)
		fmt.Printf("Total with income %9.2f€\n", s.Realized+cash.Income())
	}
	fmt.Printf("Contracts traded %10d\n", s.Contracts)
	fmt.Printf("Open round trips %10d\n", s.OpenTrips)
	printTripStats(s)

	if s.Period.Month == 0 && len(s.Months) > 0 {
		fmt.Println("\nBy month")
		for _, m := range s.Months {
			fmt.Printf("  %s %+10.2f€\n", m.Month, m.PnL())
		}
	}
}

// printSummary prints the summary of everything traded and booked in a period.
func printSummary(s reports.Summary, cash reports.CashSummary) {
	fmt.Printf("Summary - %s\n\n", s.Period)
	printTotals(s)
	fmt.Printf("Income and costs %+10.2f€   (dividends %+.2f€, interest %+.2f€, taxes %+.2f€, fees %+.2f€)\n",
		cash.Income(), cash.Dividends, cash.Interest+cash.BorrowFees,
		cash.DividendTax+cash.InterestTax, cash.DataFees+cash.Fees)
	fmt.Printf("Total result     %+10.2f€\n", s.Realized+cash.Income())
	fmt.Printf("Net deposits     %+10.2f€\n", cash.NetDeposits())
	fmt.Printf("Underlyings      %10d\n", len(s.Tickers))
	fmt.Printf("Contracts traded %10d\n", s.Contracts)
	fmt.Printf("Open round trips %10d\n", s.OpenTrips)
	printTripStats(s)

	if s.Period.Month == 0 {
		months := reports.MergeMonths(s.Months, cash.Months)
		if len(months) > 0 {
			fmt.Println("\nBy month")
			printMonths(months)
		}
	}

	printTickers("Best underlyings", s.Tickers, func(total float64) bool { return total > 0 })
	printTickers("Worst underlyings", reversed(s.Tickers), func(total float64) bool { return total < 0 })
}

// printMonths prints the trading result and income of each month and the total.
func printMonths(months []reports.OverviewMonth) {
	var total reports.OverviewMonth
	line := func(label string, m reports.OverviewMonth) {
		fmt.Printf("%s - Stocks: %+9.2f€,\tOptions: %+9.2f€,\tOther: %+9.2f€,\tIncome: %+9.2f€,\tTotal: %+9.2f€\n",
			label, m.Stocks, m.Options, m.Other, m.Income, m.Total())
	}

	for _, m := range months {
		line(m.Month, m)
		total.Stocks += m.Stocks
		total.Options += m.Options
		total.Other += m.Other
		total.Income += m.Income
	}
	if len(months) > 1 {
		line("Total  ", total)
	}
}

// printCash prints deposits, income and costs of a period.
func printCash(c reports.CashSummary) {
	fmt.Printf("Cash - %s\n\n", c.Period)
	fmt.Printf("Deposits            %+10.2f€\n", c.Deposits)
	fmt.Printf("Withdrawals         %+10.2f€\n", c.Withdrawals)
	fmt.Printf("Net deposits        %+10.2f€\n\n", c.NetDeposits())

	fmt.Printf("Dividends           %+10.2f€\n", c.Dividends)
	fmt.Printf("Dividend tax        %+10.2f€\n", c.DividendTax)
	fmt.Printf("Interest            %+10.2f€\n", c.Interest)
	fmt.Printf("Borrow fees         %+10.2f€\n", c.BorrowFees)
	fmt.Printf("Interest tax        %+10.2f€\n", c.InterestTax)
	fmt.Printf("Market data fees    %+10.2f€   (net of refunds)\n", c.DataFees)
	fmt.Printf("Other fees          %+10.2f€\n", c.Fees)
	fmt.Printf("Income and costs    %+10.2f€\n", c.Income())

	if c.Period.Month != 0 || len(c.Months) == 0 {
		return
	}

	fmt.Println("\nBy month")
	for _, m := range c.Months {
		fmt.Printf("  %s  Deposits: %+10.2f€  Dividends: %+9.2f€  Interest: %+8.2f€  Fees: %+8.2f€  Net: %+9.2f€\n",
			m.Month, m.NetDeposits(), m.Dividends+m.DividendTax, m.Interest+m.BorrowFees+m.InterestTax, m.DataFees+m.Fees, m.Income())
	}
}

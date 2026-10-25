package main

import (
	"context"
	"fmt"
	"log"
	"os"
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
		log.Fatal("usage: report monthly | report assets [YYYY-MM] | report open [SYMBOL] | report unmatched [SYMBOL]")
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

	switch os.Args[1] {
	case "monthly":
		for _, s := range reports.MonthlySummary(trips) {
			fmt.Printf("%s - Stocks: %+9.2f€,\tOptions: %+9.2f€,\tOther: %+9.2f€,\tTotal: %+9.2f€\n",
				s.Month, s.Stocks, s.Options, s.Other, s.Total())
		}
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

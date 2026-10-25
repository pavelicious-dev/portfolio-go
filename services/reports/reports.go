// Package reports aggregates realized profit and loss from round trips.
package reports

import (
	"cmp"
	"slices"
	"time"

	_ "time/tzdata"

	"portfolio/models"
)

// Realized P&L is booked on the day a lot is closed, in US Eastern time like
// the exchanges, so that late trades on the last day of a month stay in it.
var location = mustLoad("America/New_York")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// Month is "YYYY-MM".
func Month(t time.Time) string {
	return t.In(location).Format("2006-01")
}

// AssetSummary is the realized P&L of one underlying in base currency, net of fees.
type AssetSummary struct {
	Asset string
	Total float64
}

// AssetSummaries lists realized P&L per underlying, best first. An empty month
// ("YYYY-MM") covers all time.
func AssetSummaries(trips []models.RoundTrip, month string) []AssetSummary {
	assets := map[string]float64{}

	for _, rt := range trips {
		for _, m := range rt.Matches {
			if month == "" || Month(m.ClosedAt) == month {
				assets[rt.Instrument.Underlying] += m.PnLBase
			}
		}
	}

	result := make([]AssetSummary, 0, len(assets))
	for asset, total := range assets {
		result = append(result, AssetSummary{asset, total})
	}
	slices.SortFunc(result, func(a, b AssetSummary) int {
		return cmp.Or(cmp.Compare(b.Total, a.Total), cmp.Compare(a.Asset, b.Asset))
	})
	return result
}

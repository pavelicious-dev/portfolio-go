package reports

import (
	"cmp"
	"slices"
	"strings"

	"portfolio/models"
)

// ClosedTrips lists the round trips that were completely closed in the period, oldest first, with
// their realized result in base currency. An empty ticker covers all underlyings.
//
// A round trip counts in the period it was closed in, as a whole, like the trip statistics of
// Summarize; the monthly figures instead book each lot in the month it was closed.
func ClosedTrips(trips []models.RoundTrip, ticker string, period Period) []TripResult {
	ticker = strings.ToUpper(ticker)

	var results []TripResult
	for _, rt := range trips {
		if ticker != "" && rt.Instrument.Underlying != ticker {
			continue
		}
		if rt.IsOpen() || len(rt.Matches) == 0 || !period.Contains(rt.ClosedAt) {
			continue
		}
		results = append(results, newTripResult(rt))
	}

	slices.SortStableFunc(results, func(a, b TripResult) int { return cmp.Compare(a.ClosedAt.Unix(), b.ClosedAt.Unix()) })
	return results
}

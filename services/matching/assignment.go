package matching

import (
	"math"
	"time"

	"portfolio/models"
)

// deliveryKey identifies the trades of one underlying at one moment. For option
// closes it also holds the contract's strike and right.
type deliveryKey struct {
	account    int64
	underlying string
	at         time.Time
	strike     models.Fixed
	right      models.Right
}

// RollPremiums moves the result of options that ended in the delivery of shares
// into those shares, like IB does: an assigned or exercised option realizes nothing,
// its premium and fees become part of the cost of the stock it produced and are
// realized when that stock is sold.
//
// The QFX export does not flag assignments and exercises, they show up as a close
// at price 0 and a stock trade at the strike, both at the same time. A close is
// taken for one if a stock trade of the underlying at that time has the size of
// the contracts and a price at the strike. The close is then marked as assigned
// (a short option was closed) or exercised (a long one), and the stock trade is
// restated by the premium. The input is not modified.
func RollPremiums(trades []models.Trade) []models.Trade {
	stocks := map[deliveryKey][]int{}
	closes := map[deliveryKey][]int{}

	for i, t := range trades {
		in := t.Instrument
		switch {
		case in.Type == models.Stock:
			key := deliveryKey{account: t.AccountID, underlying: in.Underlying, at: t.TradedAt}
			stocks[key] = append(stocks[key], i)
		case in.Type == models.Option && t.ID != 0 && t.Price == 0 && t.Quantity != 0 && in.Strike > 0 &&
			(in.Right == models.Call || in.Right == models.Put) && t.Kind != models.KindAssign && t.Kind != models.KindExercise:
			key := deliveryKey{t.AccountID, in.Underlying, t.TradedAt, in.Strike, in.Right}
			closes[key] = append(closes[key], i)
		}
	}

	delivered := map[deliveryKey]int{} // option closes -> index of the stock trade they delivered
	used := map[int]bool{}
	for key, idx := range closes {
		first := trades[idx[0]].Instrument

		var contracts models.Fixed
		for _, i := range idx {
			contracts += trades[i].Quantity
		}
		// a put delivers (or takes) shares in the direction of the option trade, a call opposite to it
		shares := contracts * models.Fixed(first.Multiplier)
		if key.right == models.Call {
			shares = -shares
		}

		for _, j := range stocks[deliveryKey{account: key.account, underlying: key.underlying, at: key.at}] {
			s := trades[j]
			if used[j] || s.Quantity != shares || s.Instrument.Currency != first.Currency {
				continue
			}
			// the price in the file is the base currency one divided by a rounded rate
			if math.Abs(s.Price.Float()-key.strike.Float()) > 0.002*key.strike.Float()+0.01 {
				continue
			}
			delivered[key] = j
			used[j] = true
			break
		}
	}
	if len(delivered) == 0 {
		return trades
	}

	// the premium is what the options closed by the delivery made in their own matching
	stockOf := map[int64]int{} // option close trade ID -> stock trade index
	for key, j := range delivered {
		for _, i := range closes[key] {
			stockOf[trades[i].ID] = j
		}
	}
	premium := map[int]float64{}
	for _, rt := range Match(trades).RoundTrips {
		for _, m := range rt.Matches {
			if j, ok := stockOf[m.CloseTradeID]; ok {
				premium[j] += m.PnL.Float()
			}
		}
	}

	rolled := append([]models.Trade(nil), trades...)
	for key := range delivered {
		for _, i := range closes[key] {
			rolled[i].Kind = models.KindAssign // closing a short option
			if rolled[i].Quantity < 0 {
				rolled[i].Kind = models.KindExercise // closing a long one
			}
		}
	}
	for j, sum := range premium {
		rolled[j].Price = models.FixedFromFloat(rolled[j].Price.Float() - sum/rolled[j].Quantity.Float())
	}
	return rolled
}

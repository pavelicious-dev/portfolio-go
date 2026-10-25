package ingest

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"portfolio/models"
)

// transfer is a TRANSFER record: a change of a position that is not a trade.
// IB uses it for splits, delistings and moves between accounts.
type transfer struct {
	fitid      string
	at         time.Time
	memo       string
	instrument models.Instrument
	units      models.Fixed // signed change of the position
	price      models.Fixed
	out        bool
}

var splitRe = regexp.MustCompile(`SPLIT\s+([\d.]+)\s+FOR\s+([\d.]+)`)

// splitRatio reads "SPLIT 10 FOR 1" as 10 and "SPLIT 1 FOR 30" as 1/30.
func splitRatio(memo string) (float64, bool) {
	m := splitRe.FindStringSubmatch(strings.ToUpper(memo))
	if m == nil {
		return 0, false
	}
	newUnits, err1 := strconv.ParseFloat(m[1], 64)
	oldUnits, err2 := strconv.ParseFloat(m[2], 64)
	if err1 != nil || err2 != nil || newUnits <= 0 || oldUnits <= 0 {
		return 0, false
	}
	return newUnits / oldUnits, true
}

func (t transfer) direction() string {
	if t.out {
		return "OUT"
	}
	return "IN"
}

func (t transfer) sourceID() string {
	// FITIDs repeat between records: an IN and a later OUT may share one
	return fmt.Sprintf("%s|%s|%s", t.fitid, t.direction(), t.at.Format("20060102"))
}

type groupID struct {
	at     int64 // unix time: time.Time values with separately parsed zones are not comparable
	prefix string
}

// classifyTransfers turns transfer records into
//   - trades: a delisting closes the position at 0, a transfer in from another
//     account opens one at the given price,
//   - corporate actions: splits, which rescale the history of an instrument,
//
// and returns a description of every record it does not understand.
func classifyTransfers(ts []transfer) (trades []models.Trade, actions []models.CorporateAction, skipped []string) {
	var order []groupID
	groups := map[groupID][]transfer{}

	for _, t := range ts {
		memo := strings.ToUpper(t.memo)

		switch {
		case strings.Contains(memo, "SPLIT"), strings.Contains(memo, "INTERDEPOT"):
			// the records of one event share date and the memo up to the details in parentheses
			prefix, _, _ := strings.Cut(t.memo, " (")
			id := groupID{t.at.Unix(), prefix}
			if _, ok := groups[id]; !ok {
				order = append(order, id)
			}
			groups[id] = append(groups[id], t)

		case t.out && strings.Contains(memo, "DELISTED"):
			trades = append(trades, t.trade(models.KindExpire, models.EffectClose, -t.units.Abs(), 0))

		case !t.out && strings.Contains(memo, "TRANSFER IN FROM"):
			trades = append(trades, t.trade(models.KindTrade, models.EffectOpen, t.units.Abs(), t.price))

		default:
			skipped = append(skipped, t.describe())
		}
	}

	for _, id := range order {
		a, s := classifyEvent(groups[id])
		actions = append(actions, a...)
		skipped = append(skipped, s...)
	}
	return trades, actions, skipped
}

// classifyEvent handles the records of one split or re-identification.
func classifyEvent(group []transfer) (actions []models.CorporateAction, skipped []string) {
	ratio, hasSplit := splitRatio(group[0].memo)

	var outs, ins []transfer
	for _, t := range group {
		switch {
		case t.instrument.IsOption():
			// option contracts keep their id, the record only carries the change in size
			if !hasSplit {
				skipped = append(skipped, t.describe())
				continue
			}
			actions = append(actions, t.action(models.ActionScale, models.Instrument{}, ratio))
		case t.out:
			outs = append(outs, t)
		default:
			ins = append(ins, t)
		}
	}

	switch {
	case len(outs) == 1 && len(ins) == 1 && !outs[0].instrument.Same(ins[0].instrument) && outs[0].units != 0:
		// the position moves to a new instrument: take the ratio from the actual units
		r := ins[0].units.Float() / outs[0].units.Abs().Float()
		actions = append(actions, outs[0].action(models.ActionConvert, ins[0].instrument, r))
	case len(outs)+len(ins) == 1 && hasSplit:
		t := append(outs, ins...)[0]
		actions = append(actions, t.action(models.ActionScale, models.Instrument{}, ratio))
	default:
		for _, t := range append(outs, ins...) {
			skipped = append(skipped, t.describe())
		}
	}
	return actions, skipped
}

func (t transfer) trade(kind models.TradeKind, effect models.Effect, quantity, price models.Fixed) models.Trade {
	return models.Trade{
		Instrument: t.instrument,
		Source:     "qfx",
		SourceID:   t.sourceID(),
		Kind:       kind,
		Effect:     effect,
		Memo:       t.memo,
		TradedAt:   t.at,
		Quantity:   quantity,
		Price:      price,
	}
}

func (t transfer) action(kind models.ActionKind, to models.Instrument, ratio float64) models.CorporateAction {
	return models.CorporateAction{
		SourceID: t.sourceID(),
		Kind:     kind,
		At:       t.at,
		From:     t.instrument,
		To:       to,
		Ratio:    ratio,
		Memo:     t.memo,
	}
}

func (t transfer) describe() string {
	return fmt.Sprintf("%s %s %s %s %s", t.at.Format(time.DateOnly), t.direction(), t.units, t.instrument.Name(), t.memo)
}

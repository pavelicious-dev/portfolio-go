package ingest

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"portfolio/models"
)

var maturityRe = regexp.MustCompile(`(\d{2})/(\d{2})/(\d{2})\s*$`)

// maturity reads the date of names like "912797PG6 B 07/31/25".
func maturity(name string) time.Time {
	m := maturityRe.FindStringSubmatch(strings.TrimSpace(name))
	if m == nil {
		return time.Time{}
	}
	t, err := time.Parse("01/02/06", m[0])
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseBonds reads purchases, sales and maturities of T-bills.
//
// IB reports all amounts of these in the account's base currency, although
// the records carry the currency of the bill, and quotes the price per 100
// of face value, except at maturity. So the price is derived from TOTAL,
// which makes the cash flow of the trade equal to the broker's figure.
func parseBonds(stmt invStmt, debts []debtInfo, file *QFXFile) error {
	bonds := map[string]secInfo{}
	for _, d := range debts {
		bonds[d.SECINFO.SECID.UNIQUEID] = d.SECINFO
	}

	add := func(info invInfo, sign int64, reason string) error {
		sec, ok := bonds[info.SecID.UNIQUEID]
		if !ok || sec.TICKER == "" {
			return fmt.Errorf("bond trade %s: unknown security %q", info.INVTRAN.FITID, info.SecID.UNIQUEID)
		}
		at, err := parseOFXTime(info.INVTRAN.DTTRADE)
		if err != nil {
			return fmt.Errorf("bond trade %s: %w", info.INVTRAN.FITID, err)
		}

		units := math.Abs(info.UNITS)
		if units == 0 {
			return fmt.Errorf("bond trade %s: no units", info.INVTRAN.FITID)
		}
		commission := math.Abs(info.COMMISSION)

		// buys: TOTAL = -(gross + commission), sells: TOTAL = gross - commission
		gross := math.Abs(info.TOTAL) - commission
		if sign < 0 {
			gross = info.TOTAL + commission
		}

		kind, effect := models.KindTrade, models.EffectOpen
		if sign < 0 {
			kind, effect = models.KindTrade, models.EffectClose
			if strings.EqualFold(strings.TrimSpace(reason), "MATURITY") {
				kind = models.KindExpire
			}
		}

		file.Trades = append(file.Trades, models.Trade{
			Instrument: models.Instrument{
				ConID:      sec.conid(),
				Type:       models.Bond,
				Symbol:     sec.TICKER,
				Underlying: sec.TICKER,
				Currency:   stmt.CURDEF,
				Multiplier: 1,
				Expiry:     maturity(sec.SECNAME),
			},
			Source:     "qfx",
			SourceID:   info.INVTRAN.FITID,
			Kind:       kind,
			Effect:     effect,
			Memo:       strings.TrimSpace(info.INVTRAN.MEMO),
			TradedAt:   at,
			Quantity:   models.Fixed(sign) * models.FixedFromFloat(units),
			Price:      models.FixedFromFloat(gross / units),
			Commission: models.FixedFromFloat(commission),
			FxToBase:   1,
		})
		return nil
	}

	for _, b := range stmt.INVTRANLIST.BUYDEBT {
		if err := add(b.INVBUY, 1, ""); err != nil {
			return err
		}
	}
	for _, s := range stmt.INVTRANLIST.SELLDEBT {
		if err := add(s.INVSELL, -1, s.SELLREASON); err != nil {
			return err
		}
	}
	return nil
}

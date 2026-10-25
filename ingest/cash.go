package ingest

import (
	"fmt"
	"regexp"
	"strings"

	"portfolio/models"
)

var symbolRe = regexp.MustCompile(`^([A-Z0-9.]+)\(`)

// symbolFromMemo reads the ticker of memos like "BABA(US01609W1027) CASH DIVIDEND ...".
func symbolFromMemo(memo string) string {
	if m := symbolRe.FindStringSubmatch(strings.TrimSpace(memo)); m != nil {
		return m[1]
	}
	return ""
}

// classifyCash decides from the broker's memo what kind of booking this is.
// The order matters: the specific cases come before the general ones.
func classifyCash(memo string, amount float64) (models.CashCategory, bool) {
	m := strings.ToUpper(memo)
	has := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(m, p) {
				return true
			}
		}
		return false
	}

	switch {
	case has("CASH TRADE:"):
		return models.CashFXConversion, true
	case has("ADJUSTMENT:"), has("- FEE"), has("SETTLEMENT FEE"):
		return models.CashFee, true
	case has("DIVIDEND") && has("TAX"):
		return models.CashDividendTax, true
	case has("DIVIDEND"):
		return models.CashDividend, true
	case has("BORROW FEE"):
		return models.CashBorrowFee, true
	case has("WITHHOLDING"):
		return models.CashInterestTax, true
	case has("INT FOR "), has("INTEREST FOR "):
		return models.CashInterest, true
	case has("OPRA"), has("SNAPSHOTVALUE"):
		return models.CashDataFee, true
	case has("DISBURSEMENT"):
		return models.CashWithdrawal, true
	case has("CASH RECEIPTS"), has("ELECTRONIC FUND TRANSFER"):
		if amount < 0 {
			return models.CashWithdrawal, true
		}
		return models.CashDeposit, true
	}
	return "", false
}

// isDerivativeSettlement tells the bookings of futures and CFDs, which are not imported yet:
// daily profit and loss settlements and the commissions of those trades.
func isDerivativeSettlement(memo string) bool {
	m := strings.ToUpper(memo)
	return strings.Contains(m, "P/L; SETLPR") || strings.Contains(m, "TRADE PR:")
}

// parseCash collects the cash bookings of a statement: deposits, dividends,
// interest, taxes and fees.
func parseCash(stmt invStmt, file *QFXFile) error {
	list := stmt.INVTRANLIST

	add := func(fitid, at, memo, source string, amount float64, cur currencyInfo, category models.CashCategory) error {
		when, err := parseOFXTime(at)
		if err != nil {
			return fmt.Errorf("%s %s: %w", source, fitid, err)
		}

		currency, rate := stmt.CURDEF, 1.0
		if cur.CURSYM != "" {
			currency = cur.CURSYM
			if cur.CURRATE > 0 {
				rate = cur.CURRATE
			}
		}

		file.CashEvents = append(file.CashEvents, models.CashEvent{
			SourceID: fitid,
			Category: category,
			At:       when,
			Symbol:   symbolFromMemo(memo),
			Amount:   models.FixedFromFloat(amount),
			Currency: currency,
			FxToBase: rate,
			Memo:     strings.TrimSpace(memo),
		})
		return nil
	}

	skip := func(at, memo string, amount float64) {
		file.Skipped = append(file.Skipped, fmt.Sprintf("%s %+.2f %s", strings.TrimSpace(at)[:min(8, len(strings.TrimSpace(at)))], amount, strings.TrimSpace(memo)))
	}

	for _, b := range list.INVBANKTRAN {
		t := b.STMTTRN
		category, ok := classifyCash(t.MEMO, t.TRNAMT)
		if !ok {
			skip(t.DTPOSTED, t.MEMO, t.TRNAMT)
			continue
		}
		if err := add(t.FITID, t.DTPOSTED, t.MEMO, "bank transaction", t.TRNAMT, t.CURRENCY, category); err != nil {
			return err
		}
	}

	for _, in := range list.INCOME {
		if isDerivativeSettlement(in.INVTRAN.MEMO) {
			file.Ignored["futures and CFD settlements"]++
			continue
		}
		category, ok := classifyCash(in.INVTRAN.MEMO, in.TOTAL)
		if !ok && strings.EqualFold(in.INCOMETYPE, "DIV") {
			category, ok = models.CashDividend, true
		}
		if !ok {
			skip(in.INVTRAN.DTTRADE, in.INVTRAN.MEMO, in.TOTAL)
			continue
		}
		if err := add(in.INVTRAN.FITID, in.INVTRAN.DTTRADE, in.INVTRAN.MEMO, "income", in.TOTAL, in.CURRENCY, category); err != nil {
			return err
		}
	}

	for _, ex := range list.INVEXPENSE {
		if isDerivativeSettlement(ex.INVTRAN.MEMO) {
			file.Ignored["futures and CFD settlements"]++
			continue
		}
		category, ok := classifyCash(ex.INVTRAN.MEMO, ex.TOTAL)
		if !ok {
			skip(ex.INVTRAN.DTTRADE, ex.INVTRAN.MEMO, ex.TOTAL)
			continue
		}
		if err := add(ex.INVTRAN.FITID, ex.INVTRAN.DTTRADE, ex.INVTRAN.MEMO, "expense", ex.TOTAL, ex.CURRENCY, category); err != nil {
			return err
		}
	}

	if n := len(list.BUYOTHER) + len(list.SELLOTHER); n > 0 {
		file.Ignored["futures, CFD and currency trades"] += n
	}
	return nil
}

package ingest

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	"portfolio/models"
)

// QFXFile is the result of parsing one Interactive Brokers .qfx export.
type QFXFile struct {
	Account string // broker account id
	Trades  []models.Trade
	Actions []models.CorporateAction // splits and similar, from TRANSFER records
	Skipped []string                 // records that were not understood

	CashEvents []models.CashEvent // deposits, dividends, interest, taxes, fees
	Ignored    map[string]int     // records of kinds that are not imported yet, by kind
}

func ParseQFX(r io.Reader) (*QFXFile, error) {
	dec := xml.NewDecoder(r)
	dec.Strict = false

	var doc ofx
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse xml: %w", err)
	}

	stmt := doc.INVSTMTMSGSRSV1.INVSTMTTRNRS.INVSTMTRS
	list := stmt.INVTRANLIST
	secs := doc.SECLISTMSGSRSV1.SECLIST

	stocks := make(map[string]secInfo)
	for _, s := range secs.STOCKINFO {
		stocks[s.SECINFO.SECID.UNIQUEID] = s.SECINFO
	}
	options := make(map[string]optInfo)
	for _, o := range secs.OPTINFO {
		options[o.SECINFO.SECID.UNIQUEID] = o
	}

	file := &QFXFile{Account: strings.TrimSpace(stmt.INVACCTFROM.ACCTID), Ignored: map[string]int{}}
	if file.Account == "" {
		return nil, fmt.Errorf("no ACCTID found")
	}

	add := func(info invInfo, sign int64, tradeType string, instrument func(currency string) (models.Instrument, error)) error {
		currency, rate := info.currency(stmt.CURDEF)
		in, err := instrument(currency)
		if err != nil {
			return fmt.Errorf("trade %s: %w", info.INVTRAN.FITID, err)
		}
		tradedAt, err := parseOFXTime(info.INVTRAN.DTTRADE)
		if err != nil {
			return fmt.Errorf("trade %s: %w", info.INVTRAN.FITID, err)
		}
		file.Trades = append(file.Trades, models.Trade{
			Instrument: in,
			Source:     "qfx",
			SourceID:   info.INVTRAN.FITID,
			Kind:       models.KindTrade,
			Effect:     effectOf(tradeType),
			TradedAt:   tradedAt,
			// OFX reports sells with negative units; don't rely on it
			Quantity:   models.Fixed(sign) * models.FixedFromFloat(info.UNITS).Abs(),
			Price:      models.FixedFromFloat(info.UNITPRICE / rate),
			Commission: models.FixedFromFloat(info.commission(rate)),
			FxToBase:   rate,
		})
		return nil
	}

	stockInstrument := func(info invInfo) func(string) (models.Instrument, error) {
		return func(currency string) (models.Instrument, error) {
			sec := stocks[info.SecID.UNIQUEID]
			ticker := sec.TICKER
			if ticker == "" {
				return models.Instrument{}, fmt.Errorf("unknown security %q", info.SecID.UNIQUEID)
			}
			return models.Instrument{ConID: sec.conid(), Type: models.Stock, Symbol: ticker, Underlying: ticker, Currency: currency, Multiplier: 1}, nil
		}
	}
	optionInstrument := func(info invInfo, perContract int64) func(string) (models.Instrument, error) {
		return func(currency string) (models.Instrument, error) {
			opt, ok := options[info.SecID.UNIQUEID]
			if !ok {
				return models.Instrument{}, fmt.Errorf("unknown option %q", info.SecID.UNIQUEID)
			}
			if perContract == 0 {
				perContract = opt.SHPERCTRCT
			}
			return opt.instrument(currency, perContract)
		}
	}

	for _, b := range list.BUYSTOCK {
		if err := add(b.INVBUY, 1, b.BUYTYPE, stockInstrument(b.INVBUY)); err != nil {
			return nil, err
		}
	}
	for _, s := range list.SELLSTOCK {
		if err := add(s.INVSELL, -1, s.SELLTYPE, stockInstrument(s.INVSELL)); err != nil {
			return nil, err
		}
	}
	for _, b := range list.BUYOPT {
		if err := add(b.INVBUY, 1, b.OPTBUYTYPE, optionInstrument(b.INVBUY, b.SHPERCTRCT)); err != nil {
			return nil, err
		}
	}
	for _, s := range list.SELLOPT {
		if err := add(s.INVSELL, -1, s.OPTSELLTYPE, optionInstrument(s.INVSELL, s.SHPERCTRCT)); err != nil {
			return nil, err
		}
	}

	var transfers []transfer
	for _, tr := range list.TRANSFER {
		t := transfer{
			fitid: tr.INVTRAN.FITID,
			memo:  strings.TrimSpace(tr.INVTRAN.MEMO),
			units: models.FixedFromFloat(tr.UNITS),
			price: models.FixedFromFloat(tr.UNITPRICE),
			out:   strings.EqualFold(strings.TrimSpace(tr.TFERACTION), "OUT"),
		}

		var err error
		if t.at, err = parseOFXTime(tr.INVTRAN.DTTRADE); err != nil {
			return nil, fmt.Errorf("transfer %s: %w", t.fitid, err)
		}

		// the records carry no currency, so assume the account's; instruments
		// already known from trades keep theirs when stored
		if opt, ok := options[tr.SECID.UNIQUEID]; ok {
			t.instrument, err = opt.instrument(stmt.CURDEF, opt.SHPERCTRCT)
		} else if sec, ok := stocks[tr.SECID.UNIQUEID]; ok && sec.TICKER != "" {
			t.instrument = models.Instrument{ConID: sec.conid(), Type: models.Stock, Symbol: sec.TICKER, Underlying: sec.TICKER, Currency: stmt.CURDEF, Multiplier: 1}
		} else {
			err = fmt.Errorf("unknown security %q", tr.SECID.UNIQUEID)
		}
		if err != nil {
			file.Skipped = append(file.Skipped, fmt.Sprintf("%s %s: %v", t.at.Format(time.DateOnly), t.memo, err))
			continue
		}
		transfers = append(transfers, t)
	}

	trades, actions, skipped := classifyTransfers(transfers)
	file.Trades = append(file.Trades, trades...)
	file.Actions = actions
	file.Skipped = append(file.Skipped, skipped...)

	if err := parseCash(stmt, file); err != nil {
		return nil, err
	}
	if err := parseBonds(stmt, secs.DEBTINFO, file); err != nil {
		return nil, err
	}

	return file, nil
}

func (o optInfo) instrument(currency string, perContract int64) (models.Instrument, error) {
	expiry, err := parseOFXTime(o.DTEXPIRE)
	if err != nil {
		return models.Instrument{}, err
	}
	// the OFX time is a local timestamp; only the date matters
	expiry = time.Date(expiry.Year(), expiry.Month(), expiry.Day(), 0, 0, 0, 0, time.UTC)

	var right models.Right
	switch strings.ToUpper(o.OPTTYPE) {
	case "CALL":
		right = models.Call
	case "PUT":
		right = models.Put
	default:
		return models.Instrument{}, fmt.Errorf("unknown option type %q", o.OPTTYPE)
	}

	underlying := o.underlying()
	if underlying == "" {
		return models.Instrument{}, fmt.Errorf("no underlying for %q", o.SECINFO.SECNAME)
	}
	strike := models.FixedFromFloat(o.STRIKEPRICE)

	return models.Instrument{
		ConID:      o.SECINFO.conid(),
		Type:       models.Option,
		Symbol:     models.OCCSymbol(underlying, expiry, right, strike),
		Underlying: underlying,
		Currency:   currency,
		Multiplier: perContract,
		Expiry:     expiry,
		Strike:     strike,
		Right:      right,
	}, nil
}

// underlying finds the root symbol: the OCC ticker if present, otherwise the
// third word of the security name (the layout the previous importer relied on).
func (o optInfo) underlying() string {
	if u, _, _, _, ok := parseOCC(o.SECINFO.TICKER); ok {
		return strings.ToUpper(u)
	}
	if words := strings.Fields(o.SECINFO.SECNAME); len(words) > 2 {
		return strings.ToUpper(words[2])
	}
	if words := strings.Fields(o.SECINFO.TICKER); len(words) > 0 {
		return strings.ToUpper(words[0])
	}
	return ""
}

// currency returns the currency a trade was made in and its rate to the base currency.
//
// All amounts of a record (price, commission, total) are stated in the account's
// base currency, converted at that rate; CURSYM only tells what they were converted
// from. Divided by the rate they give the amounts in the trade currency, where
// prices are round numbers again: 297.0297 at a rate of 1.0001 is 297.00.
// Without a CURRENCY element the trade was made in the base currency itself.
func (i invInfo) currency(def string) (string, float64) {
	if i.CURRENCY.CURSYM == "" || i.CURRENCY.CURRATE == 0 {
		return def, 1
	}
	// CURRATE is base currency per unit of CURSYM
	return i.CURRENCY.CURSYM, i.CURRENCY.CURRATE
}

type ofx struct {
	XMLName         xml.Name `xml:"OFX"`
	INVSTMTMSGSRSV1 struct {
		INVSTMTTRNRS struct {
			INVSTMTRS invStmt
		}
	}
	SECLISTMSGSRSV1 struct {
		SECLIST struct {
			STOCKINFO []struct {
				SECINFO secInfo `xml:"SECINFO"`
			}
			OPTINFO  []optInfo
			DEBTINFO []debtInfo
		}
	}
}

type invStmt struct {
	CURDEF      string
	INVACCTFROM struct {
		ACCTID string
	}
	INVTRANLIST struct {
		BUYSTOCK []struct {
			INVBUY  invInfo
			BUYTYPE string
		}
		SELLSTOCK []struct {
			INVSELL  invInfo
			SELLTYPE string
		}
		BUYOPT []struct {
			INVBUY     invInfo
			OPTBUYTYPE string
			SHPERCTRCT int64
		}
		TRANSFER []struct {
			INVTRAN struct {
				FITID   string
				DTTRADE string
				MEMO    string
			}
			SECID      secID
			UNITS      float64
			TFERACTION string
			UNITPRICE  float64
		}
		INVBANKTRAN []struct {
			STMTTRN struct {
				TRNTYPE  string
				DTPOSTED string
				TRNAMT   float64
				FITID    string
				MEMO     string
				CURRENCY currencyInfo
			}
		}
		INCOME []struct {
			INVTRAN    invTran
			SECID      secID
			INCOMETYPE string
			TOTAL      float64
			CURRENCY   currencyInfo
		}
		INVEXPENSE []struct {
			INVTRAN  invTran
			TOTAL    float64
			CURRENCY currencyInfo
		}
		BUYDEBT []struct {
			INVBUY invInfo
		}
		SELLDEBT []struct {
			INVSELL    invInfo
			SELLREASON string
		}
		// futures, CFDs and currencies: not imported yet, only counted
		BUYOTHER []struct {
			INVBUY invInfo
		}
		SELLOTHER []struct {
			INVSELL invInfo
		}
		SELLOPT []struct {
			INVSELL     invInfo
			OPTSELLTYPE string
			SHPERCTRCT  int64
		}
	}
}

type optInfo struct {
	SECINFO     secInfo `xml:"SECINFO"`
	OPTTYPE     string
	STRIKEPRICE float64
	DTEXPIRE    string
	SHPERCTRCT  int64
}

type secInfo struct {
	SECID   secID `xml:"SECID"`
	SECNAME string
	TICKER  string
	FIID    string
}

type secID struct {
	UNIQUEID string
}

type invTran struct {
	FITID   string
	DTTRADE string
	MEMO    string
}

type currencyInfo struct {
	CURRATE float64
	CURSYM  string
}

type invInfo struct {
	INVTRAN    invTran
	SecID      secID `xml:"SECID"`
	UNITS      float64
	UNITPRICE  float64
	COMMISSION float64
	TOTAL      float64
	CURRENCY   currencyInfo
}

// commission returns the commission in the trade currency. It keeps its sign: positive is a
// cost, negative a rebate or correction, and TOTAL = -gross - COMMISSION holds either way.
func (i invInfo) commission(rate float64) float64 {
	return i.COMMISSION / rate
}

// effectOf maps an OFX buy/sell type (BUYTOOPEN, SELLTOCLOSE, BUYTOCOVER,
// SELLSHORT, ...) to the open/close flag; plain BUY and SELL carry none.
func effectOf(tradeType string) models.Effect {
	t := strings.ToUpper(tradeType)
	switch {
	case strings.Contains(t, "CLOSE"), strings.Contains(t, "COVER"):
		return models.EffectClose
	case strings.Contains(t, "OPEN"), strings.Contains(t, "SHORT"):
		return models.EffectOpen
	}
	return ""
}

// conid is the broker's contract id, which stays the same when the ticker or
// strike of a contract is adjusted for corporate actions.
func (s secInfo) conid() string {
	if s.FIID != "" {
		return strings.TrimSpace(s.FIID)
	}
	return ""
}

type debtInfo struct {
	SECINFO secInfo `xml:"SECINFO"`
}

package ingest

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"portfolio/models"
)

// QFXFile is the result of parsing one Interactive Brokers .qfx export.
type QFXFile struct {
	Account string // broker account id
	Trades  []models.Trade
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

	stocks := make(map[string]string)
	for _, s := range secs.STOCKINFO {
		stocks[s.SECINFO.SECID.UNIQUEID] = s.SECINFO.TICKER
	}
	options := make(map[string]optInfo)
	for _, o := range secs.OPTINFO {
		options[o.SECINFO.SECID.UNIQUEID] = o
	}

	file := &QFXFile{Account: strings.TrimSpace(stmt.INVACCTFROM.ACCTID)}
	if file.Account == "" {
		return nil, fmt.Errorf("no ACCTID found")
	}

	add := func(info invInfo, sign int64, instrument func(currency string) (models.Instrument, error)) error {
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
			TradedAt:   tradedAt,
			// OFX reports sells with negative units; don't rely on it
			Quantity:   models.Fixed(sign) * models.FixedFromFloat(info.UNITS).Abs(),
			Price:      models.FixedFromFloat(info.UNITPRICE),
			Commission: models.FixedFromFloat(info.commission(rate)),
			FxToBase:   rate,
		})
		return nil
	}

	stockInstrument := func(info invInfo) func(string) (models.Instrument, error) {
		return func(currency string) (models.Instrument, error) {
			ticker := stocks[info.SecID.UNIQUEID]
			if ticker == "" {
				return models.Instrument{}, fmt.Errorf("unknown security %q", info.SecID.UNIQUEID)
			}
			return models.Instrument{Type: models.Stock, Symbol: ticker, Underlying: ticker, Currency: currency, Multiplier: 1}, nil
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
		if err := add(b.INVBUY, 1, stockInstrument(b.INVBUY)); err != nil {
			return nil, err
		}
	}
	for _, s := range list.SELLSTOCK {
		if err := add(s.INVSELL, -1, stockInstrument(s.INVSELL)); err != nil {
			return nil, err
		}
	}
	for _, b := range list.BUYOPT {
		if err := add(b.INVBUY, 1, optionInstrument(b.INVBUY, b.SHPERCTRCT)); err != nil {
			return nil, err
		}
	}
	for _, s := range list.SELLOPT {
		if err := add(s.INVSELL, -1, optionInstrument(s.INVSELL, s.SHPERCTRCT)); err != nil {
			return nil, err
		}
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

// currency returns the trade currency and its rate to the base currency.
// Without a CURRENCY element OFX amounts are in the account's default currency.
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
			OPTINFO []optInfo
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
			INVBUY invInfo
		}
		SELLSTOCK []struct {
			INVSELL invInfo
		}
		BUYOPT []struct {
			INVBUY     invInfo
			SHPERCTRCT int64
		}
		SELLOPT []struct {
			INVSELL    invInfo
			SHPERCTRCT int64
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
}

type secID struct {
	UNIQUEID string
}

type invInfo struct {
	INVTRAN struct {
		FITID   string
		DTTRADE string
	}
	SecID      secID `xml:"SECID"`
	UNITS      float64
	UNITPRICE  float64
	COMMISSION float64
	CURRENCY   struct {
		CURRATE float64
		CURSYM  string
	}
}

// commission returns the commission in the trade currency. IB reports it in
// the account's base currency while prices are in the trade currency.
func (i invInfo) commission(rate float64) float64 {
	return math.Abs(i.COMMISSION) / rate
}

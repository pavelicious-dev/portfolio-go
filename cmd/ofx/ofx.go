package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"portfolio/database"
	"portfolio/models"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	database.Connect()
	defer database.Close()

	trades := importTrades(os.Args[1])

	err := database.ImportTrades(trades)
	if err != nil {
		panic(err)
	}
}

func importTrades(path string) []models.Trade {
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	bytes, _ := io.ReadAll(file)

	var ofx Ofx

	xml.Unmarshal(bytes, &ofx)

	stocks := make(map[string]string)
	options := make(map[string]string)
	trades := make([]models.Trade, 0)

	for _, stock := range ofx.SECLISTMSGSRSV1.SECLIST.STOCKINFO {
		stocks[stock.SECINFO.SECID.UNIQUEID] = stock.SECINFO.TICKER
	}

	for _, opt := range ofx.SECLISTMSGSRSV1.SECLIST.OPTINFO {
		parts := strings.Fields(opt.SECINFO.SECNAME)
		options[opt.SECINFO.SECID.UNIQUEID] = fmt.Sprintf("%s %s %.2f %s", parts[2], opt.DTEXPIRE, opt.STRIKEPRICE, opt.OPTTYPE)
	}

	for _, info := range ofx.INVSTMTMSGSRSV1.INVSTMTTRNRS.INVSTMTRS.INVTRANLIST.BUYSTOCK {
		trades = append(trades, parseStockTrade(info.INVBUY, stocks))
	}

	for _, info := range ofx.INVSTMTMSGSRSV1.INVSTMTTRNRS.INVSTMTRS.INVTRANLIST.SELLSTOCK {
		trades = append(trades, parseStockTrade(info.INVSELL, stocks))
	}

	for _, info := range ofx.INVSTMTMSGSRSV1.INVSTMTTRNRS.INVSTMTRS.INVTRANLIST.BUYOPT {
		trades = append(trades, parseOptionTrade(info.INVBUY, info.SHPERCTRCT, options))
	}

	for _, info := range ofx.INVSTMTMSGSRSV1.INVSTMTTRNRS.INVSTMTRS.INVTRANLIST.SELLOPT {
		trades = append(trades, parseOptionTrade(info.INVSELL, info.SHPERCTRCT, options))
	}

	return trades
}

func parseDate(date string) time.Time {
	formattedDate := fmt.Sprintf("%s-%s-%sT%s:%s:%s-04:00", date[:4], date[4:6], date[6:8], date[8:10], date[10:12], date[12:14])
	t, _ := time.Parse(time.RFC3339, formattedDate)
	return t
}

func parseStockTrade(stockInfo InvInfo, stockMap map[string]string) models.Trade {
	return models.Trade{
		TransactionId: stockInfo.INVTRAN.FITID,
		Asset:         stockMap[stockInfo.SecID.UNIQUEID],
		Date:          parseDate(stockInfo.INVTRAN.DTTRADE),
		Units:         stockInfo.UNITS,
		UnitPrice:     stockInfo.UNITPRICE,
		Commission:    stockInfo.COMMISSION,
		Total:         stockInfo.TOTAL,
		ExchangeRate:  stockInfo.CURRENCY.CURRATE,
		Multiplier:    1,
		IsOptionTrade: false,
	}
}

func parseOptionTrade(optionInfo InvInfo, multiplier int64, optionMap map[string]string) models.Trade {
	return models.Trade{
		TransactionId: optionInfo.INVTRAN.FITID,
		Asset:         optionMap[optionInfo.SecID.UNIQUEID],
		Date:          parseDate(optionInfo.INVTRAN.DTTRADE),
		Units:         optionInfo.UNITS,
		UnitPrice:     optionInfo.UNITPRICE,
		Commission:    optionInfo.COMMISSION,
		Total:         optionInfo.TOTAL,
		ExchangeRate:  optionInfo.CURRENCY.CURRATE,
		Multiplier:    multiplier,
		IsOptionTrade: true,
	}
}

type Ofx struct {
	XMLName         xml.Name `xml:"OFX"`
	INVSTMTMSGSRSV1 struct {
		INVSTMTTRNRS struct {
			INVSTMTRS struct {
				INVTRANLIST struct {
					DTSTART  string
					DTEND    string
					BUYSTOCK []struct {
						INVBUY InvInfo
					}
					SELLSTOCK []struct {
						INVSELL InvInfo
					}
					BUYOPT []struct {
						INVBUY     InvInfo
						SHPERCTRCT int64
					}
					SELLOPT []struct {
						INVSELL    InvInfo
						SHPERCTRCT int64
					}
				}
			}
		}
	}
	SECLISTMSGSRSV1 struct {
		SECLIST struct {
			STOCKINFO []struct {
				SECINFO SecInfo `xml:"SECINFO"`
			}
			OPTINFO []struct {
				SECINFO     SecInfo `xml:"SECINFO"`
				OPTTYPE     string
				STRIKEPRICE float64
				DTEXPIRE    string
				SHPERCTRCT  int64
			}
		}
	}
}

type SecInfo struct {
	SECID   SecID `xml:"SECID"`
	SECNAME string
	TICKER  string
	FIID    string
}

type SecID struct {
	UNIQUEID     string
	UNIQUEIDTYPE string
}

type InvInfo struct {
	INVTRAN struct {
		FITID   string
		DTTRADE string
	}
	SecID      SecID `xml:"SECID"`
	UNITS      float64
	UNITPRICE  float64
	COMMISSION float64
	TOTAL      float64
	CURRENCY   struct {
		CURRATE float64
		CURSYM  string
	}
}

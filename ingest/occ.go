package ingest

import (
	"strconv"
	"strings"
	"time"

	"portfolio/models"
)

// parseOCC splits an OCC option symbol such as "AAPL  230120C00145000".
func parseOCC(symbol string) (underlying string, expiry time.Time, right models.Right, strike models.Fixed, ok bool) {
	if len(symbol) < 16 {
		return
	}
	tail := symbol[len(symbol)-15:]

	expiry, err := time.Parse("060102", tail[:6])
	if err != nil {
		return
	}
	switch tail[6] {
	case 'C':
		right = models.Call
	case 'P':
		right = models.Put
	default:
		return
	}
	thousandths, err := strconv.ParseInt(tail[7:], 10, 64)
	if err != nil {
		return
	}

	underlying = strings.TrimSpace(symbol[:len(symbol)-15])
	strike = models.Fixed(thousandths * (models.FixedScale / 1000))
	ok = underlying != ""
	return
}

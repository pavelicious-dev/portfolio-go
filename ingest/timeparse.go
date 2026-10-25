package ingest

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "time/tzdata" // IB reports US exchange time; don't depend on the OS zone database
)

var newYork = mustLoad("America/New_York")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// parseOFXTime parses "YYYYMMDD[HHMMSS[.XXX]][[offset:TZ]]". An explicit
// offset in brackets wins, otherwise the time is taken as US Eastern.
func parseOFXTime(s string) (time.Time, error) {
	orig := s
	s = strings.TrimSpace(s)
	loc := newYork

	if i := strings.IndexByte(s, '['); i >= 0 {
		zone := strings.TrimSuffix(s[i+1:], "]")
		s = s[:i]
		offset, name, _ := strings.Cut(zone, ":")
		if hours, err := strconv.ParseFloat(offset, 64); err == nil {
			loc = time.FixedZone(name, int(hours*3600))
		}
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}

	var layout string
	switch len(s) {
	case 14:
		layout = "20060102150405"
	case 8:
		layout = "20060102"
	default:
		return time.Time{}, fmt.Errorf("bad OFX time %q", orig)
	}
	return time.ParseInLocation(layout, s, loc)
}

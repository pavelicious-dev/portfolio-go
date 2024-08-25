package models

import (
	"strings"
	"time"
)

type Position struct {
	Asset    string
	OpenedAt time.Time
	ClosedAt time.Time
	Units    float64
	Value    float64
}

func (p Position) IsOpen() bool {
	return p.ClosedAt.IsZero()
}

func (p Position) DaysOpen() int {
	if p.IsOpen() {
		return int(time.Since(p.OpenedAt).Hours() / 24)
	}
	return int(p.ClosedAt.Sub(p.OpenedAt).Hours() / 24)
}

func (p Position) IsOptionPosition() bool {
	return len(strings.Fields(p.Asset)) == 4
}

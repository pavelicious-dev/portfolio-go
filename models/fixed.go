package models

import (
	"math"
	"strconv"
)

// FixedScale is the number of fixed-point units per 1.0.
const FixedScale = 1_000_000

// Fixed is a decimal number stored as an integer scaled by FixedScale.
// It is used for quantities, prices and amounts so that sums and zero checks are exact.
type Fixed int64

func FixedFromFloat(f float64) Fixed {
	return Fixed(math.Round(f * FixedScale))
}

func (f Fixed) Float() float64 {
	return float64(f) / FixedScale
}

func (f Fixed) Abs() Fixed {
	if f < 0 {
		return -f
	}
	return f
}

func (f Fixed) String() string {
	return strconv.FormatFloat(f.Float(), 'f', -1, 64)
}

func (f Fixed) MarshalJSON() ([]byte, error) {
	return []byte(f.String()), nil
}

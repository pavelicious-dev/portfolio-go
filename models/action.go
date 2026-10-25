package models

import "time"

type ActionKind string

const (
	// ActionScale changes the unit size of an instrument in place, as in a
	// stock split of an option contract that keeps its contract id.
	ActionScale ActionKind = "SCALE"
	// ActionConvert moves a position to another instrument, as in a reverse
	// split that gives the shares a new contract id.
	ActionConvert ActionKind = "CONVERT"
)

// CorporateAction rescales the history of an instrument: trades before At
// are restated as if the new unit size had always been in place.
type CorporateAction struct {
	ID        int64      `json:"id"`
	AccountID int64      `json:"accountId"`
	SourceID  string     `json:"sourceId"`
	Kind      ActionKind `json:"kind"`
	At        time.Time  `json:"at"`
	From      Instrument `json:"from"`
	To        Instrument `json:"to,omitzero"` // ActionConvert only
	Ratio     float64    `json:"ratio"`       // new units per old unit: 10 for a 10 for 1 split
	Memo      string     `json:"memo"`
}

package models

import "time"

// CashCategory classifies money that moves without a trade.
type CashCategory string

const (
	CashDeposit      CashCategory = "DEPOSIT"
	CashWithdrawal   CashCategory = "WITHDRAWAL"
	CashDividend     CashCategory = "DIVIDEND"     // also payments in lieu, which can be negative for shorts
	CashDividendTax  CashCategory = "DIVIDEND_TAX" // withholding on dividends
	CashInterest     CashCategory = "INTEREST"     // credit and debit interest
	CashBorrowFee    CashCategory = "BORROW_FEE"   // fees for borrowed shares
	CashInterestTax  CashCategory = "INTEREST_TAX" // withholding on interest
	CashDataFee      CashCategory = "DATA_FEE"     // market data subscriptions and their refunds
	CashFee          CashCategory = "FEE"          // everything else: ADR fees, settlement fees, commission adjustments
	CashFXConversion CashCategory = "FX_CONVERSION"
)

// CashEvent is a booking of money that is not a trade, such as a deposit,
// a dividend or a fee. It is immutable once imported.
type CashEvent struct {
	ID        int64        `json:"id"`
	AccountID int64        `json:"accountId"`
	SourceID  string       `json:"sourceId"`
	Category  CashCategory `json:"category"`
	At        time.Time    `json:"at"`
	Symbol    string       `json:"symbol,omitempty"` // underlying the booking belongs to, if any
	Amount    Fixed        `json:"amount"`           // signed, in Currency
	Currency  string       `json:"currency"`
	FxToBase  float64      `json:"fxToBase"` // base currency per 1 unit of Currency, 0 = unknown
	Memo      string       `json:"memo"`
}

// AmountBase is the amount converted to the account's base currency.
func (e CashEvent) AmountBase() float64 {
	if e.FxToBase <= 0 {
		return e.Amount.Float()
	}
	return e.Amount.Float() * e.FxToBase
}

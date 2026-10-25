package reports

import (
	"math"
	"testing"
	"time"

	"portfolio/models"
)

func cashEvent(category models.CashCategory, symbol string, d time.Time, amount, rate float64) models.CashEvent {
	return models.CashEvent{Category: category, Symbol: symbol, At: d, Amount: models.FixedFromFloat(amount), Currency: "USD", FxToBase: rate}
}

func TestSummarizeCash(t *testing.T) {
	d := func(m time.Month, day int) time.Time { return time.Date(2024, m, day, 15, 0, 0, 0, time.UTC) }

	events := []models.CashEvent{
		cashEvent(models.CashDeposit, "", d(1, 2), 1000, 1),
		cashEvent(models.CashWithdrawal, "", d(2, 2), -200, 1),
		cashEvent(models.CashDividend, "ABC", d(1, 10), 100, 0.9),    // 90
		cashEvent(models.CashDividendTax, "ABC", d(1, 10), -15, 0.9), // -13.5
		cashEvent(models.CashDividend, "XYZ", d(2, 10), 50, 0.9),     // 45
		cashEvent(models.CashInterest, "", d(1, 31), 10, 0.9),        // 9
		cashEvent(models.CashBorrowFee, "", d(1, 31), -5, 0.9),       // -4.5
		cashEvent(models.CashInterestTax, "", d(1, 31), -2, 0.9),     // -1.8
		cashEvent(models.CashDataFee, "", d(1, 5), -10, 1),
		cashEvent(models.CashDataFee, "", d(1, 6), 9, 1), // refund
		cashEvent(models.CashFee, "ABC", d(2, 3), -3, 1),
		cashEvent(models.CashFXConversion, "", d(1, 3), 5000, 0.9), // moves money, changes no result
		cashEvent(models.CashDividend, "ABC", time.Date(2023, 12, 1, 15, 0, 0, 0, time.UTC), 999, 1),
	}

	near := func(what string, got, want float64) {
		t.Helper()
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("%s = %v, want %v", what, got, want)
		}
	}

	s := SummarizeCash(events, "", Period{Year: 2024})
	if s.Deposits != 1000 || s.Withdrawals != -200 || s.NetDeposits() != 800 {
		t.Errorf("deposits: %+v", s.CashTotals)
	}
	near("dividends", s.Dividends, 135)
	near("dividend tax", s.DividendTax, -13.5)
	near("interest", s.Interest, 9)
	near("borrow fees", s.BorrowFees, -4.5)
	near("interest tax", s.InterestTax, -1.8)
	near("data fees", s.DataFees, -1)
	near("fees", s.Fees, -3)
	near("income", s.Income(), 135-13.5+9-4.5-1.8-1-3)

	if len(s.Months) != 2 || s.Months[0].Month != "2024-01" || s.Months[1].Month != "2024-02" {
		t.Fatalf("months: %+v", s.Months)
	}
	near("january dividends", s.Months[0].Dividends, 90)
	near("february dividends", s.Months[1].Dividends, 45)

	abc := SummarizeCash(events, "abc", Period{Year: 2024})
	near("abc dividends", abc.Dividends, 90)
	near("abc tax", abc.DividendTax, -13.5)
	near("abc fees", abc.Fees, -3)
	if abc.Interest != 0 || abc.Deposits != 0 {
		t.Errorf("a ticker must not include general bookings: %+v", abc.CashTotals)
	}

	if feb := SummarizeCash(events, "", Period{2024, 2}); feb.Withdrawals != -200 || feb.Deposits != 0 {
		t.Errorf("february: %+v", feb.CashTotals)
	}
}

func TestMergeMonths(t *testing.T) {
	trading := []MonthTotal{{Month: "2024-01", Stocks: 100, Options: 50}, {Month: "2024-03", Other: -10}}
	cash := []CashMonth{
		{Month: "2024-02", CashTotals: CashTotals{Dividends: 20, Fees: -5}},
		{Month: "2024-03", CashTotals: CashTotals{Interest: 1}},
	}

	got := MergeMonths(trading, cash)
	if len(got) != 3 || got[0].Month != "2024-01" || got[1].Month != "2024-02" || got[2].Month != "2024-03" {
		t.Fatalf("%+v", got)
	}
	if got[0].Total() != 150 || got[1].Total() != 15 || got[1].Income != 15 || got[2].Total() != -9 {
		t.Errorf("%+v", got)
	}
}

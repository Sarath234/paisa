// internal/agent/reconcile/reconcile_test.go
package reconcile

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/statement"
)

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func TestCompare_allMatch(t *testing.T) {
	result := statement.ParseResult{
		Transactions: []statement.Transaction{
			{Date: date(2026, 5, 1), Description: "SWIGGY", Debit: 500, Credit: 0},
			{Date: date(2026, 5, 2), Description: "UPI CREDIT", Debit: 0, Credit: 1000},
		},
		ClosingBalance: 9000,
		Month:          time.May,
		Year:           2026,
	}
	ledger := []LedgerEntry{
		{Date: date(2026, 5, 1), Description: "Food Swiggy", Amount: -500},
		{Date: date(2026, 5, 2), Description: "UPI credit", Amount: 1000},
	}
	diff := Compare(result, ledger)
	if len(diff.Missing) != 0 {
		t.Errorf("Missing=%d want 0", len(diff.Missing))
	}
	if len(diff.Extra) != 0 {
		t.Errorf("Extra=%d want 0", len(diff.Extra))
	}
}

func TestCompare_missingFromLedger(t *testing.T) {
	result := statement.ParseResult{
		Transactions: []statement.Transaction{
			{Date: date(2026, 5, 1), Description: "SWIGGY", Debit: 500},
			{Date: date(2026, 5, 3), Description: "AMAZON", Debit: 1200},
		},
		ClosingBalance: 8000,
		Month:          time.May,
		Year:           2026,
	}
	ledger := []LedgerEntry{
		{Date: date(2026, 5, 1), Description: "Food Swiggy", Amount: -500},
	}
	diff := Compare(result, ledger)
	if len(diff.Missing) != 1 {
		t.Fatalf("Missing=%d want 1", len(diff.Missing))
	}
	if diff.Missing[0].Description != "AMAZON" {
		t.Errorf("Missing[0].Description=%q want AMAZON", diff.Missing[0].Description)
	}
}

func TestCompare_extraInLedger(t *testing.T) {
	result := statement.ParseResult{
		Transactions: []statement.Transaction{
			{Date: date(2026, 5, 1), Description: "SWIGGY", Debit: 500},
		},
		ClosingBalance: 9500,
		Month:          time.May,
		Year:           2026,
	}
	ledger := []LedgerEntry{
		{Date: date(2026, 5, 1), Description: "Food Swiggy", Amount: -500},
		{Date: date(2026, 5, 5), Description: "Extra phantom", Amount: -200},
	}
	diff := Compare(result, ledger)
	if len(diff.Extra) != 1 {
		t.Fatalf("Extra=%d want 1", len(diff.Extra))
	}
	if diff.Extra[0].Description != "Extra phantom" {
		t.Errorf("Extra[0].Description=%q want 'Extra phantom'", diff.Extra[0].Description)
	}
}

func TestCompare_closingBalance(t *testing.T) {
	result := statement.ParseResult{
		ClosingBalance: 12345.67,
		Month:          time.May,
		Year:           2026,
	}
	diff := Compare(result, nil)
	if diff.StatementClose != 12345.67 {
		t.Errorf("StatementClose=%.2f want 12345.67", diff.StatementClose)
	}
}

func ccTxn(date time.Time, desc string, debit, credit float64) statement.CCTransaction {
	return statement.CCTransaction{Transaction: statement.Transaction{
		Date: date, Description: desc, Debit: debit, Credit: credit,
	}}
}

func TestCompareCCDateWindow(t *testing.T) {
	res := statement.CCResult{
		PeriodEnd: date(2026, 6, 10), TotalDue: 5000,
		Transactions: []statement.CCTransaction{ccTxn(date(2026, 6, 15), "SWIGGY", 450.50, 0)},
	}
	// ledger entry 2 days later (SMS ingest date drift) still matches
	ledger := []LedgerEntry{{Date: date(2026, 6, 17), Description: "Food Swiggy", Amount: -450.50}}
	diff := CompareCC(res, ledger)
	if len(diff.Missing) != 0 || len(diff.Extra) != 0 {
		t.Fatalf("±3d window must match: %+v", diff)
	}
}

func TestCompareCCBeyondWindowIsMissing(t *testing.T) {
	res := statement.CCResult{
		PeriodEnd:    date(2026, 7, 10),
		Transactions: []statement.CCTransaction{ccTxn(date(2026, 6, 15), "SWIGGY", 450.50, 0)},
	}
	ledger := []LedgerEntry{{Date: date(2026, 6, 20), Description: "x", Amount: -450.50}}
	diff := CompareCC(res, ledger)
	if len(diff.Missing) != 1 || len(diff.Extra) != 1 {
		t.Fatalf("5 days apart: no match: %+v", diff)
	}
}

func TestCompareCCAmbiguityConsumesClosest(t *testing.T) {
	// two same-amount statement rows, two ledger entries: pairs resolve by
	// closest date, nothing reported missing/extra
	res := statement.CCResult{
		PeriodEnd: date(2026, 7, 10),
		Transactions: []statement.CCTransaction{
			ccTxn(date(2026, 6, 15), "UBER", 240.00, 0),
			ccTxn(date(2026, 6, 18), "UBER", 240.00, 0),
		},
	}
	ledger := []LedgerEntry{
		{Date: date(2026, 6, 15), Description: "u1", Amount: -240.00},
		{Date: date(2026, 6, 19), Description: "u2", Amount: -240.00},
	}
	diff := CompareCC(res, ledger)
	if len(diff.Missing) != 0 || len(diff.Extra) != 0 {
		t.Fatalf("ambiguity must fully pair: %+v", diff)
	}
}

func TestCompareCCOrderSensitiveStranding(t *testing.T) {
	// Regression: closest-date greedy in statement-list order strands a
	// pairable pair. Ledger X@day0, Y@day3; statement [A@day0, B@day-1].
	// Naive closest-date: A grabs X (gap 0), B's only remaining candidate Y
	// is gap 4 (outside ±3d) → false Missing B + false Extra Y. The valid
	// full pairing is B↔X (gap 1), A↔Y (gap 3).
	res := statement.CCResult{
		PeriodEnd: date(2026, 7, 10),
		Transactions: []statement.CCTransaction{
			ccTxn(date(2026, 6, 10), "A", 100.00, 0), // day 0
			ccTxn(date(2026, 6, 9), "B", 100.00, 0),  // day -1
		},
	}
	ledger := []LedgerEntry{
		{Date: date(2026, 6, 10), Description: "X", Amount: -100.00}, // day 0
		{Date: date(2026, 6, 13), Description: "Y", Amount: -100.00}, // day 3
	}
	diff := CompareCC(res, ledger)
	if len(diff.Missing) != 0 || len(diff.Extra) != 0 {
		t.Fatalf("full pairing exists (B-X, A-Y); must not strand: %+v", diff)
	}
}

func TestCompareCCPaymentCreditMatchesTransfer(t *testing.T) {
	res := statement.CCResult{
		PeriodEnd:    date(2026, 7, 10),
		Transactions: []statement.CCTransaction{ccTxn(date(2026, 6, 25), "PAYMENT RECEIVED", 0, 15000.00)},
	}
	ledger := []LedgerEntry{{Date: date(2026, 6, 25), Description: "CC Payment", Amount: 15000.00}}
	diff := CompareCC(res, ledger)
	if len(diff.Missing) != 0 {
		t.Fatalf("%+v", diff)
	}
}

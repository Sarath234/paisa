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

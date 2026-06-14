// internal/agent/statement/axis_test.go
package statement

import (
	"strings"
	"testing"
	"time"
)

// sampleAxisText is representative extracted-PDF text for an Axis savings statement.
const sampleAxisText = `
Statement for the period from 01-05-2026 to 31-05-2026

Savings account(s)
Account number: XXXXXXXXXX6386

Date         Tran Date   Transaction Particulars                       Chq no.  Withdrawal(Dr)  Deposit(Cr)  Balance

01-05-2026  01-05-2026  UPI/UPICQR/SWIGGY FOOD                                 500.00           0.00    9,24,764.20
05-05-2026  05-05-2026  UPI/P2P/SALARY CREDIT                                  0.00          50,000.00    9,74,764.20
12-05-2026  12-05-2026  IRCTC RAIL BOOKING                                    1,200.00          0.00    9,73,564.20
31-05-2026  31-05-2026  INTEREST CREDITED                                       0.00            120.50    9,73,684.70

Credit Cards
`

func TestAxisParser_Parse(t *testing.T) {
	p := &AxisParser{}
	result, err := p.parseText(sampleAxisText)
	if err != nil {
		t.Fatalf("parseText: %v", err)
	}

	if len(result.Transactions) != 4 {
		t.Fatalf("Transactions=%d want 4", len(result.Transactions))
	}

	tx0 := result.Transactions[0]
	if tx0.Debit != 500.00 {
		t.Errorf("tx0.Debit=%.2f want 500.00", tx0.Debit)
	}
	if tx0.Credit != 0 {
		t.Errorf("tx0.Credit=%.2f want 0.00", tx0.Credit)
	}
	wantDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if !tx0.Date.Equal(wantDate) {
		t.Errorf("tx0.Date=%v want %v", tx0.Date, wantDate)
	}

	tx1 := result.Transactions[1]
	if tx1.Credit != 50000.00 {
		t.Errorf("tx1.Credit=%.2f want 50000.00", tx1.Credit)
	}

	// Closing balance = last row's balance
	if result.ClosingBalance != 9_73_684.70 {
		t.Errorf("ClosingBalance=%.2f want 973684.70", result.ClosingBalance)
	}

	if result.Month != time.May {
		t.Errorf("Month=%v want May", result.Month)
	}
	if result.Year != 2026 {
		t.Errorf("Year=%d want 2026", result.Year)
	}
}

func TestAxisParser_Detect(t *testing.T) {
	p := &AxisParser{}
	cases := []struct {
		subject string
		want    bool
	}{
		{"Account Statement for XXXXXXXXXX6386", true},
		{"Axis Bank statement 6386", true},
		{"HDFC statement", false},
		{"", false},
	}
	for _, c := range cases {
		if got := p.Detect(c.subject); got != c.want {
			t.Errorf("Detect(%q)=%v want %v", c.subject, got, c.want)
		}
	}
}

// Ensure AxisParser implements Parser.
var _ Parser = &AxisParser{}

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }

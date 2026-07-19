package statement

import (
	"testing"
	"time"
)

const iciciCCText = `ICICI Bank Credit Card Statement
Card Number: 4XXX XXXX XXXX 6009
Statement Period: 11/06/2026 to 10/07/2026
Payment Due Date: 30/07/2026
Total Amount Due: 23,450.50
Minimum Amount Due: 1,180.00
Date Transaction Details Amount
12/06/2026 AMAZON RETAIL IN 2,999.00 DR
15/06/2026 SWIGGY BANGALORE 450.50 DR
25/06/2026 PAYMENT RECEIVED, THANK YOU 15,000.00 CR
05/07/2026 INTEREST CHARGES 740.00 DR
05/07/2026 LATE PAYMENT FEE 500.00 DR
`

func d(s string) time.Time {
	t, _ := time.Parse("02/01/2006", s)
	return t
}

func TestICICICCParseText(t *testing.T) {
	p := &ICICICCParser{}
	res, err := p.parseText(iciciCCText)
	if err != nil {
		t.Fatal(err)
	}
	if res.Last4 != "6009" {
		t.Errorf("last4: %q", res.Last4)
	}
	if !res.PeriodStart.Equal(d("11/06/2026")) || !res.PeriodEnd.Equal(d("10/07/2026")) {
		t.Errorf("period: %+v", res)
	}
	if !res.DueDate.Equal(d("30/07/2026")) || res.TotalDue != 23450.50 || res.MinDue != 1180.00 {
		t.Errorf("facts: %+v", res)
	}
	if len(res.Transactions) != 5 {
		t.Fatalf("txns: %d", len(res.Transactions))
	}
	// debit sign convention: Debit field positive
	if res.Transactions[0].Debit != 2999.00 || res.Transactions[0].Credit != 0 {
		t.Errorf("txn0: %+v", res.Transactions[0])
	}
	if res.Transactions[2].Credit != 15000.00 {
		t.Errorf("payment row: %+v", res.Transactions[2])
	}
	if !res.Transactions[3].IsInterestOrFee || !res.Transactions[4].IsInterestOrFee {
		t.Error("interest/fee rows not flagged")
	}
	if res.Transactions[0].IsInterestOrFee {
		t.Error("normal spend wrongly flagged")
	}
}

func TestICICICCParseMissingDueDateErrors(t *testing.T) {
	p := &ICICICCParser{}
	_, err := p.parseText("ICICI Bank Credit Card Statement\nCard Number: 4XXX XXXX XXXX 6009\n")
	if err == nil {
		t.Fatal("want error on missing fields")
	}
}

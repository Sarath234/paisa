package statement

import "testing"

const hdfcCCText = `HDFC Bank Credit Card Statement
Card Number 4XXX XXXX XXXX 2527
Statement Date: 14/07/2026
Payment Due Date: 05/08/2026
Total Dues 8,940.25
Minimum Amount Due 450.00
Opening Period 15/06/2026 To 14/07/2026
Date Description Amount
18/06/2026 UBER INDIA 240.00
28/06/2026 NEFT PAYMENT RECEIVED 5,000.00 Cr
10/07/2026 FINANCE CHARGES 180.25
`

func TestHDFCCCParseText(t *testing.T) {
	p := &HDFCCCParser{}
	res, err := p.parseText(hdfcCCText)
	if err != nil {
		t.Fatal(err)
	}
	if res.Last4 != "2527" {
		t.Errorf("last4: %q", res.Last4)
	}
	if !res.PeriodStart.Equal(d("15/06/2026")) || !res.PeriodEnd.Equal(d("14/07/2026")) {
		t.Errorf("period: %+v", res)
	}
	if !res.DueDate.Equal(d("05/08/2026")) || res.TotalDue != 8940.25 || res.MinDue != 450.00 {
		t.Errorf("facts: %+v", res)
	}
	if len(res.Transactions) != 3 {
		t.Fatalf("txns: %d", len(res.Transactions))
	}
	// debit sign convention: Debit field positive
	if res.Transactions[0].Debit != 240.00 || res.Transactions[0].Credit != 0 {
		t.Errorf("txn0: %+v", res.Transactions[0])
	}
	if res.Transactions[1].Credit != 5000.00 || res.Transactions[1].IsInterestOrFee {
		t.Errorf("payment row: %+v", res.Transactions[1])
	}
	if !res.Transactions[2].IsInterestOrFee {
		t.Error("FINANCE CHARGES not flagged as interest/fee")
	}
	if res.Transactions[0].IsInterestOrFee {
		t.Error("normal spend wrongly flagged")
	}
}

func TestHDFCCCParseMissingCardNumberErrors(t *testing.T) {
	p := &HDFCCCParser{}
	_, err := p.parseText("HDFC Bank Credit Card Statement\nStatement Date: 14/07/2026\n")
	if err == nil {
		t.Fatal("want error on missing card number")
	}
	if err.Error() != "hdfc_cc: card number not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHDFCCCParseMissingPeriodErrors(t *testing.T) {
	p := &HDFCCCParser{}
	_, err := p.parseText("HDFC Bank Credit Card Statement\nCard Number 4XXX XXXX XXXX 2527\n")
	if err == nil {
		t.Fatal("want error on missing period")
	}
	if err.Error() != "hdfc_cc: statement period not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHDFCCCParseMissingDueDateErrors(t *testing.T) {
	p := &HDFCCCParser{}
	_, err := p.parseText("HDFC Bank Credit Card Statement\nCard Number 4XXX XXXX XXXX 2527\nOpening Period 15/06/2026 To 14/07/2026\n")
	if err == nil {
		t.Fatal("want error on missing due date")
	}
	if err.Error() != "hdfc_cc: due date not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHDFCCCParseMissingTotalErrors(t *testing.T) {
	p := &HDFCCCParser{}
	_, err := p.parseText("HDFC Bank Credit Card Statement\nCard Number 4XXX XXXX XXXX 2527\nOpening Period 15/06/2026 To 14/07/2026\nPayment Due Date: 05/08/2026\n")
	if err == nil {
		t.Fatal("want error on missing total")
	}
	if err.Error() != "hdfc_cc: total due not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHDFCCCParseMissingMinErrors(t *testing.T) {
	p := &HDFCCCParser{}
	_, err := p.parseText("HDFC Bank Credit Card Statement\nCard Number 4XXX XXXX XXXX 2527\nOpening Period 15/06/2026 To 14/07/2026\nPayment Due Date: 05/08/2026\nTotal Dues 8,940.25\n")
	if err == nil {
		t.Fatal("want error on missing min")
	}
	if err.Error() != "hdfc_cc: minimum due not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

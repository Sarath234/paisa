package statement

import "testing"

const axisCCText = `AXIS BANK CREDIT CARD STATEMENT
Card No: XXXX XXXX XXXX 6792
Statement Date: 19/07/2026
Statement Period 20/06/2026 - 19/07/2026
Payment Due Date 08/08/2026
Total Payment Due 15,200.00
Minimum Payment Due 760.00
DATE TRANSACTION DETAILS DEBIT/CREDIT AMOUNT (Rs.)
22/06/2026 FLIPKART PAYMENTS Debit 3,499.00
01/07/2026 PAYMENT - NETBANKING Credit 20,000.00
15/07/2026 INTEREST CHARGED Debit 320.00
`

func TestAxisCCParseText(t *testing.T) {
	p := &AxisCCParser{}
	res, err := p.parseText(axisCCText)
	if err != nil {
		t.Fatal(err)
	}
	if res.Last4 != "6792" || res.TotalDue != 15200.00 || res.MinDue != 760.00 {
		t.Fatalf("%+v", res)
	}
	if !res.PeriodEnd.Equal(d("19/07/2026")) || !res.DueDate.Equal(d("08/08/2026")) {
		t.Fatalf("dates: %+v", res)
	}
	if len(res.Transactions) != 3 {
		t.Fatalf("txns: %d", len(res.Transactions))
	}
	if res.Transactions[1].Credit != 20000.00 {
		t.Errorf("credit row: %+v", res.Transactions[1])
	}
	if !res.Transactions[2].IsInterestOrFee {
		t.Error("interest row not flagged")
	}
	if res.Transactions[1].IsInterestOrFee {
		t.Error("payment row wrongly flagged as interest/fee")
	}
}

func TestAxisCCParseMissingCardNumberErrors(t *testing.T) {
	p := &AxisCCParser{}
	_, err := p.parseText("AXIS BANK CREDIT CARD STATEMENT\nStatement Date: 19/07/2026\n")
	if err == nil {
		t.Fatal("want error on missing card number")
	}
	if err.Error() != "axis_cc: card number not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAxisCCParseMissingPeriodErrors(t *testing.T) {
	p := &AxisCCParser{}
	_, err := p.parseText("AXIS BANK CREDIT CARD STATEMENT\nCard No: XXXX XXXX XXXX 6792\n")
	if err == nil {
		t.Fatal("want error on missing statement period")
	}
	if err.Error() != "axis_cc: statement period not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

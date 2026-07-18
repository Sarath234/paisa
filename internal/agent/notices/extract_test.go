package notices

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestExtractStatementICICI(t *testing.T) {
	sms := "Your ICICI Bank Credit Card XX6009 statement dated 10-Jul-26 has been sent. Total Amount Due Rs 23,450.50. Minimum Amount Due Rs 1,180.00. Due Date 30-Jul-26."
	n, err := ExtractStatement(sms)
	if err != nil {
		t.Fatal(err)
	}
	if n == nil {
		t.Fatal("not recognized")
	}
	if n.Last4 != "6009" || n.TotalDue != 23450.50 || n.MinDue != 1180.00 {
		t.Fatalf("%+v", n)
	}
	if !n.StatementDate.Equal(day("2026-07-10")) || !n.DueDate.Equal(day("2026-07-30")) {
		t.Fatalf("dates: %+v", n)
	}
}

func TestExtractStatementAxis(t *testing.T) {
	sms := "Statement for your Axis Bank Credit Card no. XX6792 dated 19-07-2026 is generated. Total amt due INR 15,200.00, Min amt due INR 760.00. Pay by 08-08-2026."
	n, err := ExtractStatement(sms)
	if err != nil {
		t.Fatal(err)
	}
	if n == nil || n.Last4 != "6792" || n.TotalDue != 15200.00 {
		t.Fatalf("%+v", n)
	}
	if !n.DueDate.Equal(day("2026-08-08")) {
		t.Fatalf("due: %+v", n)
	}
}

func TestExtractStatementHDFC(t *testing.T) {
	sms := "HDFC Bank Credit Card statement for card ending 2527 dated 14/07/2026: Total due Rs.8,940.25 Min due Rs.450.00 Due date 05/08/2026."
	n, err := ExtractStatement(sms)
	if err != nil {
		t.Fatal(err)
	}
	if n == nil || n.Last4 != "2527" || n.MinDue != 450.00 {
		t.Fatalf("%+v", n)
	}
}

func TestExtractStatementNotANotice(t *testing.T) {
	n, err := ExtractStatement("INR 500.00 spent on ICICI Bank Card XX6009 at AMAZON on 12-Jul-26")
	if n != nil || err != nil {
		t.Fatalf("transaction SMS must not be a statement notice: %+v %v", n, err)
	}
}

func TestExtractStatementPartialFailureNamesField(t *testing.T) {
	// looks like a statement notice, but due date is mangled
	sms := "Your ICICI Bank Credit Card XX6009 statement dated 10-Jul-26 has been sent. Total Amount Due Rs 23,450.50. Minimum Amount Due Rs 1,180.00. Due Date soon."
	n, err := ExtractStatement(sms)
	if n != nil || err == nil {
		t.Fatalf("want field error, got %+v %v", n, err)
	}
}

func TestExtractPaymentICICI(t *testing.T) {
	sms := "Payment of Rs 23,450.50 received on your ICICI Bank Credit Card XX6009 on 25-Jul-26. Thank you."
	n, err := ExtractPayment(sms)
	if err != nil {
		t.Fatal(err)
	}
	if n == nil || n.Last4 != "6009" || n.Amount != 23450.50 || !n.Date.Equal(day("2026-07-25")) {
		t.Fatalf("%+v", n)
	}
}

func TestExtractPaymentAxis(t *testing.T) {
	sms := "We have received payment of INR 15,200.00 towards your Axis Bank Credit Card XX6792 on 05-08-2026."
	n, err := ExtractPayment(sms)
	if err != nil {
		t.Fatal(err)
	}
	if n == nil || n.Last4 != "6792" || n.Amount != 15200.00 {
		t.Fatalf("%+v", n)
	}
}

func TestExtractPaymentNotANotice(t *testing.T) {
	n, err := ExtractPayment("INR 500.00 spent on ICICI Bank Card XX6009 at AMAZON on 12-Jul-26")
	if n != nil || err != nil {
		t.Fatalf("spend SMS must not be a payment notice: %+v %v", n, err)
	}
}

func TestStatementNoticeIsNotPayment(t *testing.T) {
	sms := "Your ICICI Bank Credit Card XX6009 statement dated 10-Jul-26 has been sent. Total Amount Due Rs 23,450.50. Minimum Amount Due Rs 1,180.00. Due Date 30-Jul-26."
	n, _ := ExtractPayment(sms)
	if n != nil {
		t.Fatalf("statement notice matched payment extractor: %+v", n)
	}
}

// TestSpendAlertIsNotPayment guards against a spend-alert SMS being
// misread as a payment: "Payment of Rs.X made using your Credit Card" is
// a SPEND notification (the card was used to pay a merchant/biller), not
// money received against the card balance. Treating it as a payment would
// fake PaidDate (killing cc_due reminders for a bill that isn't actually
// paid) and would eat the spend (never surfaced as a transaction).
func TestSpendAlertIsNotPayment(t *testing.T) {
	sms := "Payment of Rs.1200 made using your Credit Card XX1234 at MERCHANT on 12-Jul-26."
	if LooksLikePayment(sms) {
		t.Fatalf("spend-alert phrasing must not look like a payment: %q", sms)
	}
	n, err := ExtractPayment(sms)
	if n != nil || err != nil {
		t.Fatalf("spend-alert must yield (nil, nil), got %+v, %v", n, err)
	}
}

// TestSpendAlertUsingYourCardIsNotPayment covers the "using your Credit
// Card" phrasing variant directly (without "made"), since the exclusion
// must catch it even when it still has a "payment of Rs" preamble.
func TestSpendAlertUsingYourCardIsNotPayment(t *testing.T) {
	sms := "Payment of Rs.1200 using your Credit Card XX1234 at MERCHANT on 12-Jul-26."
	if LooksLikePayment(sms) {
		t.Fatalf("spend phrasing must not look like a payment: %q", sms)
	}
}

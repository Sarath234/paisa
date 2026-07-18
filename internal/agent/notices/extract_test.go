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

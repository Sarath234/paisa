package main

import (
	"testing"
	"time"
)

func TestParseCCDueCallbackPaid(t *testing.T) {
	action, account, dueDate, err := parseCCDueCallback("ccdue_paid:Liabilities:CreditCard:ICIC6009:2026-07-30")
	if err != nil {
		t.Fatal(err)
	}
	if action != "paid" {
		t.Errorf("action = %q, want paid", action)
	}
	if account != "Liabilities:CreditCard:ICIC6009" {
		t.Errorf("account = %q", account)
	}
	want, _ := time.ParseInLocation("2006-01-02", "2026-07-30", time.Local)
	if !dueDate.Equal(want) {
		t.Errorf("dueDate = %v, want %v", dueDate, want)
	}
}

func TestParseCCDueCallbackRemind(t *testing.T) {
	action, account, _, err := parseCCDueCallback("ccdue_remind:Liabilities:CreditCard:MyZone1610:2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	if action != "remind" {
		t.Errorf("action = %q, want remind", action)
	}
	if account != "Liabilities:CreditCard:MyZone1610" {
		t.Errorf("account = %q", account)
	}
}

func TestParseCCDueCallbackRejectsUnknownPrefix(t *testing.T) {
	if _, _, _, err := parseCCDueCallback("approve"); err == nil {
		t.Fatal("want error for non-ccdue callback data")
	}
}

func TestParseCCDueCallbackRejectsMalformedDate(t *testing.T) {
	if _, _, _, err := parseCCDueCallback("ccdue_paid:Liabilities:CreditCard:ICIC6009:not-a-date"); err == nil {
		t.Fatal("want error for malformed due date")
	}
}

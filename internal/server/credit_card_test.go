package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestLoadBillTruthMissingFileReturnsNil(t *testing.T) {
	bills := loadBillTruth(t.TempDir(), "Liabilities:CreditCard:ICIC6009")
	if bills != nil {
		t.Fatalf("want nil for missing file, got %+v", bills)
	}
}

func TestLoadBillTruthCorruptFileReturnsNil(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bill-truth.json"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	bills := loadBillTruth(dir, "Liabilities:CreditCard:ICIC6009")
	if bills != nil {
		t.Fatalf("want nil for corrupt file, got %+v", bills)
	}
}

func TestLoadBillTruthReturnsSortedNewestFirst(t *testing.T) {
	dir := t.TempDir()
	data := `{
	  "Liabilities:CreditCard:ICIC6009": [
	    {"periodEnd": "2026-06-10T00:00:00Z", "dueDate": "2026-06-30T00:00:00Z", "totalDue": 1000, "sources": {"due_date": 1}},
	    {"periodEnd": "2026-07-10T00:00:00Z", "dueDate": "2026-07-30T00:00:00Z", "totalDue": 2000, "sources": {"due_date": 1}}
	  ]
	}`
	if err := os.WriteFile(filepath.Join(dir, "bill-truth.json"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	bills := loadBillTruth(dir, "Liabilities:CreditCard:ICIC6009")
	if len(bills) != 2 {
		t.Fatalf("want 2 bills, got %d", len(bills))
	}
	if bills[0].TotalDue != 2000 {
		t.Fatalf("want newest (2000) first, got %+v", bills[0])
	}
}

func TestLoadBillTruthUnknownAccountReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	data := `{"Liabilities:CreditCard:OTHER": [{"periodEnd": "2026-06-10T00:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(dir, "bill-truth.json"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	bills := loadBillTruth(dir, "Liabilities:CreditCard:ICIC6009")
	if len(bills) != 0 {
		t.Fatalf("want empty for unknown account, got %+v", bills)
	}
}

func TestMatchTruthBillWithinWindow(t *testing.T) {
	end := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	bills := []truthBill{{PeriodEnd: end.AddDate(0, 0, 3)}}
	got := matchTruthBill(bills, end)
	if got == nil {
		t.Fatal("want match within 7-day window")
	}
}

func TestMatchTruthBillOutsideWindowReturnsNil(t *testing.T) {
	end := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	bills := []truthBill{{PeriodEnd: end.AddDate(0, 0, 10)}}
	got := matchTruthBill(bills, end)
	if got != nil {
		t.Fatalf("want no match outside window, got %+v", got)
	}
}

func TestMatchTruthBillPicksFirstOfMultipleCandidates(t *testing.T) {
	end := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	newest := truthBill{PeriodEnd: end.AddDate(0, 0, 2), TotalDue: 999}
	older := truthBill{PeriodEnd: end.AddDate(0, 0, -2), TotalDue: 111}
	got := matchTruthBill([]truthBill{newest, older}, end)
	if got == nil || got.TotalDue != 999 {
		t.Fatalf("want first-in-list match (999), got %+v", got)
	}
}

func TestFieldStatusComputedWhenAuthorityBelowSMS(t *testing.T) {
	got := fieldStatus(0, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if got != "computed" {
		t.Fatalf("want computed, got %s", got)
	}
}

func TestFieldStatusConfirmedWhenSameCalendarDay(t *testing.T) {
	d := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	got := fieldStatus(authoritySMS, d, d)
	if got != "confirmed" {
		t.Fatalf("want confirmed, got %s", got)
	}
}

func TestFieldStatusCorrectedWhenDifferentCalendarDay(t *testing.T) {
	computed := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	truthDate := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	got := fieldStatus(authoritySMS, computed, truthDate)
	if got != "corrected" {
		t.Fatalf("want corrected, got %s", got)
	}
}

func TestAmountFieldStatusToleratesExactlyOneRupee(t *testing.T) {
	got := amountFieldStatus(authoritySMS, decimal.NewFromFloat(1000), 999)
	if got != "confirmed" {
		t.Fatalf("want confirmed at exactly ₹1 diff, got %s", got)
	}
}

func TestAmountFieldStatusCorrectedJustOverOneRupee(t *testing.T) {
	got := amountFieldStatus(authoritySMS, decimal.NewFromFloat(1000), 998.99)
	if got != "corrected" {
		t.Fatalf("want corrected just over ₹1 diff, got %s", got)
	}
}

func TestAmountFieldStatusComputedWhenAuthorityIsAPI(t *testing.T) {
	got := amountFieldStatus(0, decimal.NewFromFloat(1000), 500) // huge diff, but authority too low
	if got != "computed" {
		t.Fatalf("want computed regardless of diff when authority < SMS, got %s", got)
	}
}

func TestPaidDateStatusBothNilIsConfirmed(t *testing.T) {
	got := paidDateStatus(authoritySMS, nil, nil, nil)
	if got != "confirmed" {
		t.Fatalf("want confirmed, got %s", got)
	}
}

func TestPaidDateStatusNilVsSetIsCorrected(t *testing.T) {
	d := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	got := paidDateStatus(authoritySMS, nil, &d, nil)
	if got != "corrected" {
		t.Fatalf("want corrected, got %s", got)
	}
}

func TestPaidDateStatusSameDayIsConfirmed(t *testing.T) {
	a := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	b := time.Date(2026, 7, 25, 23, 0, 0, 0, time.UTC)
	got := paidDateStatus(authoritySMS, &a, &b, nil)
	if got != "confirmed" {
		t.Fatalf("want confirmed for same calendar day, got %s", got)
	}
}

func TestPaidDateStatusSelfReportedWhenOnlyUserPaidSet(t *testing.T) {
	u := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	got := paidDateStatus(authoritySMS, nil, nil, &u)
	if got != "self_reported" {
		t.Fatalf("want self_reported, got %s", got)
	}
}

func TestPaidDateStatusSelfReportedIgnoresAuthorityGate(t *testing.T) {
	// authority 0 (below SMS) would normally force "computed" — but a
	// self-reported mark has no bank authority at all, so it must still win.
	u := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	got := paidDateStatus(0, nil, nil, &u)
	if got != "self_reported" {
		t.Fatalf("want self_reported even at authority 0, got %s", got)
	}
}

func TestPaidDateStatusBankConfirmationOverridesSelfReported(t *testing.T) {
	u := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	truth := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	got := paidDateStatus(authoritySMS, nil, &truth, &u)
	if got != "corrected" {
		t.Fatalf("bank confirmation must override self-reported, got %s", got)
	}
}

func TestApplyTruthSelfReportedSetsStatusFromUserPaidDate(t *testing.T) {
	u := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	bill := &CreditCardBill{DueDate: time.Now(), ClosingBalance: decimal.NewFromInt(100)}
	truth := &truthBill{DueDate: bill.DueDate, TotalDue: 100, UserPaidDate: &u}

	applyTruth(bill, truth)

	if bill.PaidDateStatus != "self_reported" {
		t.Fatalf("want self_reported, got %s", bill.PaidDateStatus)
	}
}

func TestChannelLabelPdfWhenAuthorityAtLeastPdf(t *testing.T) {
	if got := channelLabel(authorityPDF); got != "pdf" {
		t.Fatalf("want pdf, got %s", got)
	}
}

func TestChannelLabelSmsWhenAuthorityBelowPdf(t *testing.T) {
	if got := channelLabel(authoritySMS); got != "sms" {
		t.Fatalf("want sms, got %s", got)
	}
}

func TestApplyTruthNilLeavesAllFieldsComputed(t *testing.T) {
	bill := &CreditCardBill{DueDate: time.Now(), ClosingBalance: decimal.NewFromInt(100)}
	applyTruth(bill, nil)
	if bill.DueDateStatus != "computed" || bill.ClosingBalanceStatus != "computed" || bill.PaidDateStatus != "computed" {
		t.Fatalf("want all computed, got %+v", bill)
	}
}

func TestApplyTruthCorrectedDueDateSetsComputedAndTruthPairAndChannel(t *testing.T) {
	computed := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	truthDate := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	bill := &CreditCardBill{DueDate: computed, ClosingBalance: decimal.NewFromInt(100)}
	truth := &truthBill{DueDate: truthDate, TotalDue: 100, Sources: map[string]int{"due_date": authoritySMS}}

	applyTruth(bill, truth)

	if bill.DueDateStatus != "corrected" {
		t.Fatalf("want corrected, got %s", bill.DueDateStatus)
	}
	if bill.ComputedDueDate == nil || !bill.ComputedDueDate.Equal(computed) {
		t.Fatalf("want ComputedDueDate == original computed value, got %+v", bill.ComputedDueDate)
	}
	if bill.TruthDueDate == nil || !bill.TruthDueDate.Equal(truthDate) {
		t.Fatalf("want TruthDueDate == truth value, got %+v", bill.TruthDueDate)
	}
	if !bill.DueDate.Equal(truthDate) {
		t.Fatalf("want DueDate overwritten with truth value, got %+v", bill.DueDate)
	}
	if bill.DueDateChannel == nil || *bill.DueDateChannel != "sms" {
		t.Fatalf("want channel sms, got %+v", bill.DueDateChannel)
	}
}

func TestApplyTruthConfirmedLeavesComputedTruthPairNil(t *testing.T) {
	d := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	bill := &CreditCardBill{DueDate: d, ClosingBalance: decimal.NewFromInt(100)}
	truth := &truthBill{DueDate: d, TotalDue: 100, Sources: map[string]int{"due_date": authoritySMS}}

	applyTruth(bill, truth)

	if bill.DueDateStatus != "confirmed" {
		t.Fatalf("want confirmed, got %s", bill.DueDateStatus)
	}
	if bill.ComputedDueDate != nil || bill.TruthDueDate != nil {
		t.Fatalf("want no pair on confirmed, got computed=%+v truth=%+v", bill.ComputedDueDate, bill.TruthDueDate)
	}
}

func TestApplyTruthCorrectedClosingBalanceSetsPair(t *testing.T) {
	bill := &CreditCardBill{DueDate: time.Now(), ClosingBalance: decimal.NewFromFloat(1000)}
	truth := &truthBill{DueDate: bill.DueDate, TotalDue: 1500, Sources: map[string]int{"total_due": authorityPDF}}

	applyTruth(bill, truth)

	if bill.ClosingBalanceStatus != "corrected" {
		t.Fatalf("want corrected, got %s", bill.ClosingBalanceStatus)
	}
	if bill.ClosingBalanceChannel == nil || *bill.ClosingBalanceChannel != "pdf" {
		t.Fatalf("want channel pdf, got %+v", bill.ClosingBalanceChannel)
	}
	if bill.ComputedClosingBalance == nil || !bill.ComputedClosingBalance.Equal(decimal.NewFromFloat(1000)) {
		t.Fatalf("want ComputedClosingBalance == 1000, got %+v", bill.ComputedClosingBalance)
	}
	if bill.TruthClosingBalance == nil || !bill.TruthClosingBalance.Equal(decimal.NewFromFloat(1500)) {
		t.Fatalf("want TruthClosingBalance == 1500, got %+v", bill.TruthClosingBalance)
	}
	if !bill.ClosingBalance.Equal(decimal.NewFromFloat(1500)) {
		t.Fatalf("want ClosingBalance overwritten with truth value, got %+v", bill.ClosingBalance)
	}
}

func TestApplyTruthCorrectedPaidDateComputedNilIsPreserved(t *testing.T) {
	paidDate := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	bill := &CreditCardBill{DueDate: time.Now(), ClosingBalance: decimal.NewFromInt(100), PaidDate: nil}
	truth := &truthBill{DueDate: bill.DueDate, TotalDue: 100, PaidDate: &paidDate, Sources: map[string]int{"paid_date": authoritySMS}}

	applyTruth(bill, truth)

	if bill.PaidDateStatus != "corrected" {
		t.Fatalf("want corrected, got %s", bill.PaidDateStatus)
	}
	if bill.ComputedPaidDate != nil {
		t.Fatalf("want ComputedPaidDate nil (that IS the mismatch), got %+v", bill.ComputedPaidDate)
	}
	if bill.TruthPaidDate == nil || !bill.TruthPaidDate.Equal(paidDate) {
		t.Fatalf("want TruthPaidDate == truth value, got %+v", bill.TruthPaidDate)
	}
	if bill.PaidDate == nil || !bill.PaidDate.Equal(paidDate) {
		t.Fatalf("want PaidDate overwritten with truth value, got %+v", bill.PaidDate)
	}
}

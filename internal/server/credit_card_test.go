package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

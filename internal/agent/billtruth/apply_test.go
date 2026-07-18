package billtruth

import (
	"testing"
	"time"
)

func ptrT(t time.Time) *time.Time { return &t }
func ptrF(f float64) *float64     { return &f }

const acct = "Liabilities:CreditCard:ICIC6009"

func smsFacts() Facts {
	return Facts{
		Account:   acct,
		PeriodEnd: ptrT(day("2026-07-10")),
		DueDate:   ptrT(day("2026-07-30")),
		TotalDue:  ptrF(23450.50),
		MinDue:    ptrF(1180.00),
		Source:    AuthoritySMS,
	}
}

func TestApplyCreatesBillFromSMS(t *testing.T) {
	s, _ := Open(t.TempDir())
	changed, err := s.Apply(smsFacts())
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 4 {
		t.Fatalf("changed: %v", changed)
	}
	bills := s.BillsFor(acct)
	if len(bills) != 1 || bills[0].TotalDue != 23450.50 || !bills[0].DueDate.Equal(day("2026-07-30")) {
		t.Fatalf("bills: %+v", bills)
	}
}

func TestPDFConvergesOntoSMSBill(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.Apply(smsFacts())
	// PDF arrives later: period end differs by 1 day, total differs
	changed, err := s.Apply(Facts{
		Account:     acct,
		PeriodStart: ptrT(day("2026-06-11")),
		PeriodEnd:   ptrT(day("2026-07-11")),
		DueDate:     ptrT(day("2026-07-31")),
		TotalDue:    ptrF(24100.00),
		MinDue:      ptrF(1205.00),
		Source:      AuthorityPDF,
	})
	if err != nil {
		t.Fatal(err)
	}
	bills := s.BillsFor(acct)
	if len(bills) != 1 {
		t.Fatalf("SMS+PDF must converge onto ONE bill, got %d", len(bills))
	}
	if bills[0].TotalDue != 24100.00 || !bills[0].DueDate.Equal(day("2026-07-31")) {
		t.Fatalf("pdf must overwrite sms: %+v", bills[0])
	}
	if len(changed) == 0 {
		t.Error("changed fields not reported")
	}
}

func TestAPINeverOverwritesSMS(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.Apply(smsFacts())
	changed, _ := s.Apply(Facts{
		Account:   acct,
		PeriodEnd: ptrT(day("2026-07-10")),
		DueDate:   ptrT(day("2026-07-28")), // computed guess, wrong
		TotalDue:  ptrF(20000.00),
		Source:    AuthorityAPI,
	})
	if len(changed) != 0 {
		t.Fatalf("api must not overwrite sms fields: changed=%v", changed)
	}
	if got := s.BillsFor(acct)[0].DueDate; !got.Equal(day("2026-07-30")) {
		t.Fatalf("due date clobbered: %v", got)
	}
}

func TestAPIFillsHoles(t *testing.T) {
	s, _ := Open(t.TempDir())
	// api sees a cycle nothing else reported
	changed, _ := s.Apply(Facts{
		Account:   acct,
		PeriodEnd: ptrT(day("2026-06-10")),
		DueDate:   ptrT(day("2026-06-30")),
		TotalDue:  ptrF(9999.00),
		Source:    AuthorityAPI,
	})
	if len(changed) != 3 {
		t.Fatalf("api should fill empty bill: %v", changed)
	}
}

func TestPaymentAttachesToNewestUnpaidBill(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.Apply(smsFacts())
	changed, err := s.Apply(Facts{
		Account:    acct,
		PaidDate:   ptrT(day("2026-07-25")),
		PaidAmount: ptrF(23450.50),
		Source:     AuthoritySMS,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 {
		t.Fatalf("changed: %v", changed)
	}
	b := s.BillsFor(acct)[0]
	if b.PaidDate == nil || !b.PaidDate.Equal(day("2026-07-25")) {
		t.Fatalf("paid not set: %+v", b)
	}
}

func TestAuthorityUpgradePersistsWhenValuesUnchanged(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	if _, err := s.Apply(smsFacts()); err != nil {
		t.Fatal(err)
	}
	// PDF confirms the SMS values exactly: no field changes value, but the
	// authority upgrade to pdf must still reach disk.
	pdf := smsFacts()
	pdf.Source = AuthorityPDF
	changed, err := s.Apply(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("identical values must report no changes: %v", changed)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	bills := s2.BillsFor(acct)
	if len(bills) != 1 {
		t.Fatalf("bills after reload: %+v", bills)
	}
	if got := bills[0].Sources["total_due"]; got != AuthorityPDF {
		t.Fatalf("authority upgrade lost on disk: total_due source = %d, want pdf", got)
	}
}

func TestApplyInterestTotal(t *testing.T) {
	s, _ := Open(t.TempDir())
	f := smsFacts()
	f.Source = AuthorityPDF
	f.InterestTotal = ptrF(1240.00)
	s.Apply(f)
	b := s.BillsFor(acct)[0]
	if b.InterestTotal != 1240.00 || b.Sources["interest_total"] != AuthorityPDF {
		t.Fatalf("%+v", b)
	}
}

func TestPaymentWithNoUnpaidBillIsIgnored(t *testing.T) {
	s, _ := Open(t.TempDir())
	changed, err := s.Apply(Facts{
		Account:  acct,
		PaidDate: ptrT(day("2026-07-25")),
		Source:   AuthoritySMS,
	})
	if err != nil || len(changed) != 0 {
		t.Fatalf("want silent ignore, got changed=%v err=%v", changed, err)
	}
}

// TestApplyNormalizesUTCDatesToLocalMidnight guards the timezone fix: SMS
// and PDF dates parse as UTC midnight (time.Parse with no zone), while
// monitors compute "today" as DateOnly(time.Now()) — a LOCAL midnight.
// Comparing a UTC midnight against a local midnight can be off by a whole
// day depending on the host's UTC offset, lagging overdue/announcement
// checks. Apply must normalize every incoming date to local-midnight (same
// calendar y/m/d, time.Local location) so stored bill dates and
// DateOnly(Now()) are always directly comparable.
func TestApplyNormalizesUTCDatesToLocalMidnight(t *testing.T) {
	s, _ := Open(t.TempDir())
	utcPeriodEnd := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	utcDue := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	total := 100.0
	if _, err := s.Apply(Facts{
		Account: acct, PeriodEnd: &utcPeriodEnd, DueDate: &utcDue, TotalDue: &total,
		Source: AuthoritySMS,
	}); err != nil {
		t.Fatal(err)
	}
	b := s.BillsFor(acct)[0]
	wantDue := time.Date(2026, 7, 30, 0, 0, 0, 0, time.Local)
	if b.DueDate.Location() != time.Local {
		t.Fatalf("due date location = %v, want time.Local", b.DueDate.Location())
	}
	if !b.DueDate.Equal(wantDue) {
		t.Fatalf("due date = %v, want local midnight %v", b.DueDate, wantDue)
	}
	wantPeriodEnd := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)
	if !b.PeriodEnd.Equal(wantPeriodEnd) {
		t.Fatalf("period end = %v, want local midnight %v", b.PeriodEnd, wantPeriodEnd)
	}
}

// TestApplyNormalizesUTCPaidDateToLocalMidnight covers the PaidDate
// special-case branch (which setT doesn't handle — see Apply) separately.
func TestApplyNormalizesUTCPaidDateToLocalMidnight(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.Apply(smsFacts())
	utcPaid := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	if _, err := s.Apply(Facts{
		Account: acct, PaidDate: &utcPaid, Source: AuthoritySMS,
	}); err != nil {
		t.Fatal(err)
	}
	b := s.BillsFor(acct)[0]
	wantPaid := time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local)
	if b.PaidDate == nil || b.PaidDate.Location() != time.Local || !b.PaidDate.Equal(wantPaid) {
		t.Fatalf("paid date = %v, want local midnight %v", b.PaidDate, wantPaid)
	}
}

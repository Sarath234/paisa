package billtruth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// day builds a local-midnight time.Time, matching what Apply normalizes
// every incoming date to (see apply.go's localMidnight) and what
// time.Now() genuinely returns in production — so date arithmetic and
// equality checks in these tests reflect real behavior regardless of the
// host's timezone.
func day(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func TestOpenMissingFileReturnsEmptyStore(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := s.BillsFor("Liabilities:CreditCard:ICIC6009"); len(got) != 0 {
		t.Fatalf("want empty, got %+v", got)
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	s.putForTest(Bill{
		Account:   "Liabilities:CreditCard:ICIC6009",
		PeriodEnd: day("2026-07-10"),
		DueDate:   day("2026-07-30"),
		TotalDue:  23450.50,
		Sources:   map[string]Authority{"total_due": AuthoritySMS},
	})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	bills := s2.BillsFor("Liabilities:CreditCard:ICIC6009")
	if len(bills) != 1 || bills[0].TotalDue != 23450.50 {
		t.Fatalf("reload: %+v", bills)
	}
	if bills[0].Sources["total_due"] != AuthoritySMS {
		t.Errorf("source lost on reload: %+v", bills[0].Sources)
	}
}

func TestCorruptFileRecovers(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bill-truth.json"), []byte("{nope"), 0644)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.BillsFor("x")) != 0 {
		t.Fatal("want fresh store")
	}
	if _, err := os.Stat(filepath.Join(dir, "bill-truth.json.bak")); err != nil {
		t.Error("corrupt file not renamed aside")
	}
}

func TestAuthorityValuesArePinned(t *testing.T) {
	if AuthorityAPI != 0 || AuthoritySMS != 1 || AuthorityPDF != 2 {
		t.Fatalf("Authority values reordered: api=%d sms=%d pdf=%d", AuthorityAPI, AuthoritySMS, AuthorityPDF)
	}
}

func TestBillsForReturnsDeepCopies(t *testing.T) {
	s, _ := Open(t.TempDir())
	paid := day("2026-07-20")
	s.putForTest(Bill{
		Account:  "Liabilities:CreditCard:ICIC6009",
		PaidDate: &paid,
		Sources:  map[string]Authority{"total_due": AuthoritySMS},
	})

	got := s.BillsFor("Liabilities:CreditCard:ICIC6009")
	got[0].Sources["total_due"] = AuthorityPDF
	got[0].Sources["injected"] = AuthorityAPI
	*got[0].PaidDate = day("1999-01-01")

	fresh := s.BillsFor("Liabilities:CreditCard:ICIC6009")
	if fresh[0].Sources["total_due"] != AuthoritySMS {
		t.Errorf("mutating returned Sources leaked into store: %+v", fresh[0].Sources)
	}
	if _, ok := fresh[0].Sources["injected"]; ok {
		t.Error("new key in returned Sources leaked into store")
	}
	if !fresh[0].PaidDate.Equal(day("2026-07-20")) {
		t.Errorf("mutating returned *PaidDate leaked into store: %v", fresh[0].PaidDate)
	}
}

func TestSavePrunesTo12CyclesPerCard(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	for i := 0; i < 15; i++ {
		s.putForTest(Bill{
			Account:   "Liabilities:CreditCard:ICIC6009",
			PeriodEnd: day("2025-01-10").AddDate(0, i, 0),
		})
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	bills := s.BillsFor("Liabilities:CreditCard:ICIC6009")
	if len(bills) != 12 {
		t.Fatalf("want 12 after prune, got %d", len(bills))
	}
	// newest kept: the 15th bill has PeriodEnd 2026-03-10
	found := false
	for _, b := range bills {
		if b.PeriodEnd.Equal(day("2026-03-10")) {
			found = true
		}
	}
	if !found {
		t.Error("prune removed the newest bill")
	}
}

func TestSetUserPaidThenFindBill(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	s.putForTest(Bill{
		Account:  "Liabilities:CreditCard:ICIC6009",
		DueDate:  day("2026-07-30"),
		TotalDue: 23450.50,
	})

	beforeSet := time.Now()
	if err := s.SetUserPaid("Liabilities:CreditCard:ICIC6009", day("2026-07-30")); err != nil {
		t.Fatal(err)
	}
	afterSet := time.Now()

	bill := s.FindBill("Liabilities:CreditCard:ICIC6009", day("2026-07-30"))
	if bill == nil {
		t.Fatal("want bill, got nil")
	}
	if bill.UserPaidDate == nil {
		t.Fatal("want UserPaidDate set")
	}
	// Verify UserPaidDate is approximately now (within the window between beforeSet and afterSet)
	if bill.UserPaidDate.Before(beforeSet) || bill.UserPaidDate.After(afterSet) {
		t.Errorf("UserPaidDate = %v, want within [%v, %v]", bill.UserPaidDate, beforeSet, afterSet)
	}
}

func TestSetUserPaidPersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	s.putForTest(Bill{Account: "Liabilities:CreditCard:ICIC6009", DueDate: day("2026-07-30")})
	if err := s.SetUserPaid("Liabilities:CreditCard:ICIC6009", day("2026-07-30")); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	bill := s2.FindBill("Liabilities:CreditCard:ICIC6009", day("2026-07-30"))
	if bill == nil || bill.UserPaidDate == nil {
		t.Fatalf("UserPaidDate lost on reload: %+v", bill)
	}
}

func TestSetUserPaidNoMatchingBillErrors(t *testing.T) {
	s, _ := Open(t.TempDir())
	err := s.SetUserPaid("Liabilities:CreditCard:ICIC6009", day("2026-07-30"))
	if err == nil {
		t.Fatal("want error for no matching bill")
	}
}

func TestFindBillNoMatchReturnsNil(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.putForTest(Bill{Account: "Liabilities:CreditCard:ICIC6009", DueDate: day("2026-07-30")})
	if got := s.FindBill("Liabilities:CreditCard:ICIC6009", day("2026-08-30")); got != nil {
		t.Fatalf("want nil for non-matching due date, got %+v", got)
	}
	if got := s.FindBill("Liabilities:CreditCard:OTHER", day("2026-07-30")); got != nil {
		t.Fatalf("want nil for non-matching account, got %+v", got)
	}
}

// TestBillsForReturnsDeepCopiesIncludingUserPaidDate extends the existing
// TestBillsForReturnsDeepCopies to cover the new pointer field — mutating a
// returned bill's UserPaidDate must never leak into the store.
func TestBillsForReturnsDeepCopiesIncludingUserPaidDate(t *testing.T) {
	s, _ := Open(t.TempDir())
	paid := day("2026-07-30")
	s.putForTest(Bill{
		Account:      "Liabilities:CreditCard:ICIC6009",
		UserPaidDate: &paid,
	})

	got := s.BillsFor("Liabilities:CreditCard:ICIC6009")
	*got[0].UserPaidDate = day("1999-01-01")

	fresh := s.BillsFor("Liabilities:CreditCard:ICIC6009")
	if !fresh[0].UserPaidDate.Equal(day("2026-07-30")) {
		t.Errorf("mutating returned *UserPaidDate leaked into store: %v", fresh[0].UserPaidDate)
	}
}

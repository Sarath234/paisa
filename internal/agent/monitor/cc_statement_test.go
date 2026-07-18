package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
)

func TestCCStatementFiresAfterPeriodCloses(t *testing.T) {
	s := truthStore(t, billtruth.Bill{
		Account:   "Liabilities:CreditCard:Axis",
		PeriodEnd: day("2026-07-15"), DueDate: day("2026-07-28"), TotalDue: 31200,
	})
	// truthStore doesn't set PeriodStart; apply it directly onto the same bill.
	start, end := day("2026-06-16"), day("2026-07-15")
	if _, err := s.Apply(billtruth.Facts{
		Account: "Liabilities:CreditCard:Axis", PeriodEnd: &end, PeriodStart: &start,
		Source: billtruth.AuthoritySMS,
	}); err != nil {
		t.Fatal(err)
	}

	m := NewCCStatement(s, 8)

	// period not yet closed
	m.Now = func() time.Time { return day("2026-07-15").Add(9 * time.Hour) }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 0 {
		t.Fatalf("period still open: %+v", insights)
	}

	// closed — fires even days late (downtime self-heal); dedupe is the notifier's job
	m.Now = func() time.Time { return day("2026-07-18").Add(9 * time.Hour) }
	insights, err = m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 {
		t.Fatalf("want 1 insight: %+v", insights)
	}
	in := insights[0]
	if in.Key != "cc-stmt/Liabilities:CreditCard:Axis/2026-07-15/31200" {
		t.Errorf("key: %q", in.Key)
	}
	if in.Urgency != Digest {
		t.Error("want Digest urgency")
	}
	for _, want := range []string{"Axis", "₹31200.00", "16 Jun", "15 Jul"} {
		if !strings.Contains(in.Title, want) {
			t.Errorf("title missing %q: %q", want, in.Title)
		}
	}
}

func TestCCStatementUsesLatestBill(t *testing.T) {
	s := truthStore(t,
		billtruth.Bill{Account: "Liabilities:CreditCard:Axis", PeriodEnd: day("2026-06-15"), DueDate: day("2026-06-28"), TotalDue: 900},
		billtruth.Bill{Account: "Liabilities:CreditCard:Axis", PeriodEnd: day("2026-07-15"), DueDate: day("2026-07-28"), TotalDue: 31200},
	)
	m := NewCCStatement(s, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 || !strings.Contains(insights[0].Key, "2026-07-15") {
		t.Fatalf("insights: %+v", insights)
	}
}

// sentPrefixOver builds a SentPrefix func backed by a plain sent-key set,
// mirroring how monitor.Store.WasSentPrefix scans the real sent-key map.
func sentPrefixOver(sent map[string]bool) func(prefix string) bool {
	return func(prefix string) bool {
		for k := range sent {
			if strings.HasPrefix(k, prefix) {
				return true
			}
		}
		return false
	}
}

// TestCCStatementCorrectionFiresOncePDFDiffers exercises the final
// correction design: the announce key embeds the rounded total
// (cc-stmt/<account>/<period>/<amount>), so a PDF that changes the total
// produces a NEW key under the same "cc-stmt/" namespace. The monitor
// recognizes the prior announcement via SentPrefix on the account/period
// prefix and, since Sources["total_due"] is PDF, formats this one as a
// correction rather than a fresh announcement.
func TestCCStatementCorrectionFiresOncePDFDiffers(t *testing.T) {
	s := truthStore(t, billtruth.Bill{
		Account:   "Liabilities:CreditCard:ICIC6009",
		PeriodEnd: day("2026-07-10"), DueDate: day("2026-07-30"), TotalDue: 23450.50,
	})
	m := NewCCStatement(s, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) }
	sent := map[string]bool{}
	m.SentPrefix = sentPrefixOver(sent)

	first, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || !strings.HasPrefix(first[0].Key, "cc-stmt/") {
		t.Fatalf("%+v", first)
	}
	sent[first[0].Key] = true

	// PDF corrects total by more than ₹1
	end, total := day("2026-07-10"), 24100.00
	if _, err := s.Apply(billtruth.Facts{
		Account: "Liabilities:CreditCard:ICIC6009", PeriodEnd: &end, TotalDue: &total,
		Source: billtruth.AuthorityPDF,
	}); err != nil {
		t.Fatal(err)
	}

	second, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || !strings.HasPrefix(second[0].Key, "cc-stmt/") {
		t.Fatalf("want correction insight: %+v", second)
	}
	if second[0].Key == first[0].Key {
		t.Fatalf("corrected amount must produce a distinct key: %q", second[0].Key)
	}
	if !strings.Contains(second[0].Title, "24100.00") {
		t.Errorf("%q", second[0].Title)
	}
	if !strings.Contains(second[0].Title, "corrected") {
		t.Errorf("want correction wording: %q", second[0].Title)
	}

	// Once delivered, a further Check with nothing new must stay quiet.
	sent[second[0].Key] = true
	third, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(third) != 0 {
		t.Fatalf("settled bill must be quiet: %+v", third)
	}
}

// TestCCStatementNoCorrectionWithoutPDFSource guards against api-noise:
// a total_due change whose source isn't PDF (e.g. an API refresh reusing an
// older, lower-authority guess) must never be rendered as a "corrected"
// insight — it's emitted (if at all) as a plain announcement under its own
// new key.
func TestCCStatementNoCorrectionWithoutPDFSource(t *testing.T) {
	s := truthStore(t, billtruth.Bill{
		Account:   "Liabilities:CreditCard:ICIC6009",
		PeriodEnd: day("2026-07-10"), DueDate: day("2026-07-30"), TotalDue: 23450.50,
	})
	m := NewCCStatement(s, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) }
	sent := map[string]bool{}
	m.SentPrefix = sentPrefixOver(sent)

	first, _ := m.Check(context.Background())
	sent[first[0].Key] = true

	// SMS (same authority, so it CAN change the value) nudges the total.
	end, total := day("2026-07-10"), 23460.00
	if _, err := s.Apply(billtruth.Facts{
		Account: "Liabilities:CreditCard:ICIC6009", PeriodEnd: &end, TotalDue: &total,
		Source: billtruth.AuthoritySMS,
	}); err != nil {
		t.Fatal(err)
	}

	second, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 {
		t.Fatalf("%+v", second)
	}
	if strings.Contains(second[0].Title, "corrected") {
		t.Errorf("non-PDF source must not be announced as a correction: %q", second[0].Title)
	}
}

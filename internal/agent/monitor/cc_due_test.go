package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

type fakeFetcher struct {
	cards []paisaclient.CreditCardSummary
	err   error
}

func (f *fakeFetcher) CreditCards() ([]paisaclient.CreditCardSummary, error) {
	return f.cards, f.err
}

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func cardWithBill(bill paisaclient.CreditCardBill) paisaclient.CreditCardSummary {
	return paisaclient.CreditCardSummary{
		Account:     "Liabilities:CreditCard:Axis",
		Balance:     23450.5,
		CreditLimit: 100000,
		Bills:       []paisaclient.CreditCardBill{bill},
	}
}

func TestCCDueEmitsSmallestApplicableOffset(t *testing.T) {
	bill := paisaclient.CreditCardBill{
		StatementEndDate: day("2026-07-15"),
		DueDate:          day("2026-07-28"),
		ClosingBalance:   23450.5,
	}
	m := NewCCDue(&fakeFetcher{cards: []paisaclient.CreditCardSummary{cardWithBill(bill)}}, []int{7, 3, 1, 0}, 8)

	cases := []struct {
		today   string
		wantKey string
		wantIn  string
	}{
		{"2026-07-21", "cc-due/Liabilities:CreditCard:Axis/2026-07-28/d-7", "in 7 days"},
		{"2026-07-26", "cc-due/Liabilities:CreditCard:Axis/2026-07-28/d-3", "in 2 days"}, // missed d-3 window self-heals
		{"2026-07-28", "cc-due/Liabilities:CreditCard:Axis/2026-07-28/d-0", "today"},
		{"2026-07-30", "cc-due/Liabilities:CreditCard:Axis/2026-07-28/overdue", "overdue"},
	}
	for _, c := range cases {
		m.Now = func() time.Time { return day(c.today).Add(8 * time.Hour) }
		insights, err := m.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(insights) != 1 {
			t.Fatalf("%s: got %d insights", c.today, len(insights))
		}
		in := insights[0]
		if in.Key != c.wantKey {
			t.Errorf("%s: key %q, want %q", c.today, in.Key, c.wantKey)
		}
		if !strings.Contains(in.Title, c.wantIn) || !strings.Contains(in.Title, "₹23450.50") || !strings.Contains(in.Title, "Axis") {
			t.Errorf("%s: title %q", c.today, in.Title)
		}
		if in.Urgency != Immediate {
			t.Errorf("%s: want Immediate", c.today)
		}
	}
}

func TestCCDueQuietWhenFarOrPaid(t *testing.T) {
	paid := day("2026-07-20")
	cases := []struct {
		name string
		bill paisaclient.CreditCardBill
	}{
		{"far away", paisaclient.CreditCardBill{DueDate: day("2026-08-20"), ClosingBalance: 100}},
		{"already paid", paisaclient.CreditCardBill{DueDate: day("2026-07-28"), PaidDate: &paid, ClosingBalance: 100}},
	}
	for _, c := range cases {
		m := NewCCDue(&fakeFetcher{cards: []paisaclient.CreditCardSummary{cardWithBill(c.bill)}}, []int{3, 1, 0}, 8)
		m.Now = func() time.Time { return day("2026-07-26") }
		insights, err := m.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(insights) != 0 {
			t.Errorf("%s: want quiet, got %+v", c.name, insights)
		}
	}
}

func TestCCDueUsesLatestUnpaidBill(t *testing.T) {
	paid := day("2026-06-25")
	card := paisaclient.CreditCardSummary{
		Account: "Liabilities:CreditCard:Axis",
		Bills: []paisaclient.CreditCardBill{
			{DueDate: day("2026-06-28"), PaidDate: &paid, ClosingBalance: 900},
			{DueDate: day("2026-07-28"), ClosingBalance: 23450.5},
		},
	}
	m := NewCCDue(&fakeFetcher{cards: []paisaclient.CreditCardSummary{card}}, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-07-27") }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 || !strings.Contains(insights[0].Key, "2026-07-28/d-1") {
		t.Fatalf("insights: %+v", insights)
	}
	if !strings.Contains(insights[0].Title, "tomorrow") {
		t.Errorf("d-1 title should say tomorrow: %q", insights[0].Title)
	}
}

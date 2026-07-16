package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

func TestCCStatementFiresAfterPeriodCloses(t *testing.T) {
	bill := paisaclient.CreditCardBill{
		StatementStartDate: day("2026-06-16"),
		StatementEndDate:   day("2026-07-15"),
		DueDate:            day("2026-07-28"),
		ClosingBalance:     31200,
	}
	m := NewCCStatement(&fakeFetcher{cards: []paisaclient.CreditCardSummary{cardWithBill(bill)}}, 8)

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
	if in.Key != "cc-stmt/Liabilities:CreditCard:Axis/2026-07-15" {
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
	card := paisaclient.CreditCardSummary{
		Account: "Liabilities:CreditCard:Axis",
		Bills: []paisaclient.CreditCardBill{
			{StatementStartDate: day("2026-05-16"), StatementEndDate: day("2026-06-15"), ClosingBalance: 900},
			{StatementStartDate: day("2026-06-16"), StatementEndDate: day("2026-07-15"), ClosingBalance: 31200},
		},
	}
	m := NewCCStatement(&fakeFetcher{cards: []paisaclient.CreditCardSummary{card}}, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 || !strings.Contains(insights[0].Key, "2026-07-15") {
		t.Fatalf("insights: %+v", insights)
	}
}

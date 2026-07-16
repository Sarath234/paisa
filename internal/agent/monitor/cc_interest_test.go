package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

func TestCCInterestSumsMatchingPostings(t *testing.T) {
	bill := paisaclient.CreditCardBill{
		StatementStartDate: day("2026-06-16"),
		StatementEndDate:   day("2026-07-15"),
		Postings: []paisaclient.Posting{
			{Payee: "Interest Charge June", Amount: -1240},
			{Payee: "AMAZON PAY", Amount: -2999},
			{Payee: "LATE FEE REVERSAL... late fee", Amount: -500},
		},
	}
	m := NewCCInterest(&fakeFetcher{cards: []paisaclient.CreditCardSummary{cardWithBill(bill)}}, []string{"INTEREST", "LATE FEE"}, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) }

	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 {
		t.Fatalf("insights: %+v", insights)
	}
	in := insights[0]
	if in.Key != "cc-interest/Liabilities:CreditCard:Axis/2026-07-15" {
		t.Errorf("key: %q", in.Key)
	}
	if in.Urgency != Immediate {
		t.Error("want Immediate")
	}
	if !strings.Contains(in.Title, "₹1740.00") || !strings.Contains(in.Title, "Axis") {
		t.Errorf("title: %q", in.Title)
	}
}

func TestCCInterestQuietCases(t *testing.T) {
	cleanBill := paisaclient.CreditCardBill{
		StatementEndDate: day("2026-07-15"),
		Postings:         []paisaclient.Posting{{Payee: "AMAZON PAY", Amount: -2999}},
	}
	openBill := paisaclient.CreditCardBill{
		StatementEndDate: day("2026-07-15"),
		Postings:         []paisaclient.Posting{{Payee: "INTEREST", Amount: -100}},
	}
	cases := []struct {
		name  string
		bill  paisaclient.CreditCardBill
		today string
	}{
		{"no matching postings", cleanBill, "2026-07-16"},
		{"period still open", openBill, "2026-07-15"},
	}
	for _, c := range cases {
		m := NewCCInterest(&fakeFetcher{cards: []paisaclient.CreditCardSummary{cardWithBill(c.bill)}}, []string{"INTEREST", "LATE FEE"}, 8)
		m.Now = func() time.Time { return day(c.today).Add(8 * time.Hour) }
		insights, err := m.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(insights) != 0 {
			t.Errorf("%s: want quiet, got %+v", c.name, insights)
		}
	}
}

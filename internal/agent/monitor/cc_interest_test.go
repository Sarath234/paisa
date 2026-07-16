package monitor

import (
	"context"
	"errors"
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
	m := NewCCInterest(fetcherWithDetail(cardWithBill(bill)), []string{"INTEREST", "LATE FEE"}, 8)
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
		m := NewCCInterest(fetcherWithDetail(cardWithBill(c.bill)), []string{"INTEREST", "LATE FEE"}, 8)
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

// TestCCInterestCountsPayeeMatchingMultiplePatternsOnce covers a posting
// payee that matches more than one interest/fee pattern (e.g. a combined
// "INTEREST ON LATE FEE" line item): it must be counted once, not once per
// matching pattern (the inner loop `break`s on first match).
func TestCCInterestCountsPayeeMatchingMultiplePatternsOnce(t *testing.T) {
	bill := paisaclient.CreditCardBill{
		StatementEndDate: day("2026-07-15"),
		Postings: []paisaclient.Posting{
			{Payee: "INTEREST ON LATE FEE", Amount: -300}, // matches both INTEREST and LATE FEE
		},
	}
	m := NewCCInterest(fetcherWithDetail(cardWithBill(bill)), []string{"INTEREST", "LATE FEE"}, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) }

	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 {
		t.Fatalf("insights: %+v", insights)
	}
	if !strings.Contains(insights[0].Title, "₹300.00") {
		t.Errorf("title: %q, want ₹300.00 (counted once, not ₹600.00)", insights[0].Title)
	}
}

// TestCCInterestSumsClosedBillWhileOpenCycleExists reproduces the production
// shape: an actively-used card carries an open current cycle (future
// StatementEndDate, PaidDate nil) alongside the closed statement. cc_interest
// must sum interest from the closed bill on the DETAIL response, ignoring the
// open cycle.
func TestCCInterestSumsClosedBillWhileOpenCycleExists(t *testing.T) {
	card := paisaclient.CreditCardSummary{
		Account: "Liabilities:CreditCard:Axis",
		Bills: []paisaclient.CreditCardBill{
			{ // closed
				StatementEndDate: day("2026-06-15"),
				Postings:         []paisaclient.Posting{{Payee: "INTEREST CHARGE", Amount: -750}},
			},
			{ // open current cycle
				StatementEndDate: day("2026-07-15"),
				Postings:         []paisaclient.Posting{{Payee: "INTEREST CHARGE", Amount: -50}},
			},
		},
	}
	m := NewCCInterest(fetcherWithDetail(card), []string{"INTEREST"}, 8)
	m.Now = func() time.Time { return day("2026-06-20").Add(8 * time.Hour) }

	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 {
		t.Fatalf("insights: %+v", insights)
	}
	if !strings.Contains(insights[0].Title, "₹750.00") {
		t.Errorf("title: %q, want closed bill's ₹750.00, not the open cycle's ₹50.00", insights[0].Title)
	}
	if !strings.Contains(insights[0].Key, "2026-06-15") {
		t.Errorf("key: %q, want the closed bill's statement end date", insights[0].Key)
	}
}

func TestCCInterestListFetchErrorPropagates(t *testing.T) {
	m := NewCCInterest(&fakeFetcher{err: errors.New("paisa unreachable")}, []string{"INTEREST"}, 8)
	m.Now = func() time.Time { return day("2026-07-16") }
	_, err := m.Check(context.Background())
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestCCInterestDetailFetchErrorPropagates(t *testing.T) {
	f := &fakeFetcher{
		cards:     []paisaclient.CreditCardSummary{{Account: "Liabilities:CreditCard:Axis"}},
		detailErr: errors.New("paisa unreachable"),
	}
	m := NewCCInterest(f, []string{"INTEREST"}, 8)
	m.Now = func() time.Time { return day("2026-07-16") }
	_, err := m.Check(context.Background())
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestCCInterestSkipsCardNotFoundInDetail(t *testing.T) {
	f := &fakeFetcher{
		cards:  []paisaclient.CreditCardSummary{{Account: "Liabilities:CreditCard:Axis"}},
		detail: map[string]*paisaclient.CreditCardSummary{}, // not found
	}
	m := NewCCInterest(f, []string{"INTEREST"}, 8)
	m.Now = func() time.Time { return day("2026-07-16") }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 0 {
		t.Fatalf("want quiet for not-found card, got %+v", insights)
	}
}

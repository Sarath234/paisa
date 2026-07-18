package monitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

// axisTruth builds a billtruth.Store seeded with a single closed bill for
// Liabilities:CreditCard:Axis, no interest_total set, so the account always
// takes the API-fallback path in these tests.
func axisTruth(t *testing.T, periodEnd time.Time) *billtruth.Store {
	t.Helper()
	return truthStore(t, billtruth.Bill{
		Account:   "Liabilities:CreditCard:Axis",
		PeriodEnd: periodEnd, DueDate: periodEnd.AddDate(0, 0, 20), TotalDue: 100,
	})
}

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
	s := axisTruth(t, day("2026-07-15"))
	m := NewCCInterest(s, fetcherWithDetail(cardWithBill(bill)), []string{"INTEREST", "LATE FEE"}, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) }

	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 {
		t.Fatalf("insights: %+v", insights)
	}
	in := insights[0]
	if in.Key != "cc-interest/Liabilities:CreditCard:Axis/2026-07" {
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
		s := axisTruth(t, day("2026-07-15"))
		m := NewCCInterest(s, fetcherWithDetail(cardWithBill(c.bill)), []string{"INTEREST", "LATE FEE"}, 8)
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
	s := axisTruth(t, day("2026-07-15"))
	m := NewCCInterest(s, fetcherWithDetail(cardWithBill(bill)), []string{"INTEREST", "LATE FEE"}, 8)
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
// StatementEndDate, PaidDate nil) alongside the closed statement in the
// API's list/detail response (billtruth itself never holds the open cycle;
// see apisync's fill-holes-only sync). cc_interest must sum interest from
// the closed bill on the DETAIL response, ignoring the open cycle.
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
	s := axisTruth(t, day("2026-06-15")) // truth only ever holds the closed cycle
	m := NewCCInterest(s, fetcherWithDetail(card), []string{"INTEREST"}, 8)
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
	if !strings.Contains(insights[0].Key, "2026-06") {
		t.Errorf("key: %q, want the closed bill's statement end date", insights[0].Key)
	}
}

func TestCCInterestListFetchErrorPropagates(t *testing.T) {
	s := axisTruth(t, day("2026-07-15"))
	m := NewCCInterest(s, &fakeFetcher{err: errors.New("paisa unreachable")}, []string{"INTEREST"}, 8)
	m.Now = func() time.Time { return day("2026-07-16") }
	_, err := m.Check(context.Background())
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

// TestCCInterestContinuesPastFailingCard: one card's detail fetch failing
// must not abort the sweep — remaining cards are still checked and their
// insights returned, with no error (the failed card's statement is monthly;
// it is retried on the next daily run via the unsent dedupe key).
func TestCCInterestContinuesPastFailingCard(t *testing.T) {
	billA := paisaclient.CreditCardBill{StatementEndDate: day("2026-07-15")}
	billB := paisaclient.CreditCardBill{
		StatementEndDate: day("2026-07-15"),
		Postings:         []paisaclient.Posting{{Payee: "INTEREST CHARGE", Amount: -900}},
	}
	cardB := paisaclient.CreditCardSummary{Account: "Liabilities:CreditCard:HDFC", Bills: []paisaclient.CreditCardBill{billB}}
	f := &fakeFetcher{
		cards: []paisaclient.CreditCardSummary{
			{Account: "Liabilities:CreditCard:Axis", Bills: []paisaclient.CreditCardBill{billA}},
			cardB,
		},
		detail:     map[string]*paisaclient.CreditCardSummary{cardB.Account: &cardB},
		detailErrs: map[string]error{"Liabilities:CreditCard:Axis": errors.New("paisa unreachable")},
	}
	s := truthStore(t,
		billtruth.Bill{Account: "Liabilities:CreditCard:Axis", PeriodEnd: day("2026-07-15"), DueDate: day("2026-08-05"), TotalDue: 100},
		billtruth.Bill{Account: "Liabilities:CreditCard:HDFC", PeriodEnd: day("2026-07-15"), DueDate: day("2026-08-05"), TotalDue: 100},
	)
	m := NewCCInterest(s, f, []string{"INTEREST"}, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) }

	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatalf("per-card failure must not abort the sweep: %v", err)
	}
	if len(insights) != 1 || !strings.Contains(insights[0].Key, "HDFC") {
		t.Fatalf("want the healthy card's insight, got %+v", insights)
	}
}

// TestCCInterestSkipsDetailFetchWhenAlreadySent: the per-account detail call
// is the expensive part (N+1 on the paisa server). Once a statement's
// insight key has been sent, the monitor must not re-fetch that card's
// detail on every subsequent run.
func TestCCInterestSkipsDetailFetchWhenAlreadySent(t *testing.T) {
	bill := paisaclient.CreditCardBill{
		StatementEndDate: day("2026-07-15"),
		Postings:         []paisaclient.Posting{{Payee: "INTEREST CHARGE", Amount: -100}},
	}
	f := fetcherWithDetail(cardWithBill(bill))
	s := axisTruth(t, day("2026-07-15"))
	m := NewCCInterest(s, f, []string{"INTEREST"}, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) }
	m.Sent = func(key string) bool { return key == "cc-interest/Liabilities:CreditCard:Axis/2026-07" }

	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 0 {
		t.Fatalf("want quiet for already-sent statement, got %+v", insights)
	}
	if f.detailCalls != 0 {
		t.Fatalf("detail fetched %d times for an already-sent statement, want 0", f.detailCalls)
	}
}

// TestCCInterestSkipsDetailFetchWithoutClosedBill: a card whose only cycle is
// still open (e.g. brand new card) never gets a billtruth entry at all
// (apisync's SyncFromAPI skips open cycles), so it never even reaches the
// account loop, let alone the API detail call.
func TestCCInterestSkipsDetailFetchWithoutClosedBill(t *testing.T) {
	openBill := paisaclient.CreditCardBill{StatementEndDate: day("2026-08-15")}
	f := fetcherWithDetail(cardWithBill(openBill))
	s := truthStore(t) // no truth-store entry: no closed bill exists yet
	m := NewCCInterest(s, f, []string{"INTEREST"}, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) }

	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 0 {
		t.Fatalf("want quiet, got %+v", insights)
	}
	if f.detailCalls != 0 {
		t.Fatalf("detail fetched %d times for a card with no closed bill, want 0", f.detailCalls)
	}
}

func TestCCInterestSkipsCardNotFoundInDetail(t *testing.T) {
	closedBill := paisaclient.CreditCardBill{StatementEndDate: day("2026-06-15")}
	f := &fakeFetcher{
		cards:  []paisaclient.CreditCardSummary{{Account: "Liabilities:CreditCard:Axis", Bills: []paisaclient.CreditCardBill{closedBill}}},
		detail: map[string]*paisaclient.CreditCardSummary{}, // not found
	}
	s := axisTruth(t, day("2026-06-15"))
	m := NewCCInterest(s, f, []string{"INTEREST"}, 8)
	m.Now = func() time.Time { return day("2026-07-16") }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 0 {
		t.Fatalf("want quiet for not-found card, got %+v", insights)
	}
}

// TestCCInterestPrefersPDFTotal: when the parsed statement PDF recorded an
// interest_total, cc_interest must use it directly and never touch the API
// detail endpoint at all — not even to enumerate the card list.
func TestCCInterestPrefersPDFTotal(t *testing.T) {
	s := truthStore(t, billtruth.Bill{
		Account:   "Liabilities:CreditCard:ICIC6009",
		PeriodEnd: day("2026-07-10"), DueDate: day("2026-07-30"), TotalDue: 24100.00,
	})
	end, it := day("2026-07-10"), 1240.00
	if _, err := s.Apply(billtruth.Facts{
		Account: "Liabilities:CreditCard:ICIC6009", PeriodEnd: &end, InterestTotal: &it,
		Source: billtruth.AuthorityPDF,
	}); err != nil {
		t.Fatal(err)
	}

	f := &fakeFetcher{} // must NOT be called
	m := NewCCInterest(s, f, []string{"INTEREST"}, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 || !strings.Contains(insights[0].Title, "₹1240.00") {
		t.Fatalf("%+v", insights)
	}
	if f.detailCalls != 0 {
		t.Fatalf("PDF truth present — API detail must not be fetched (%d calls)", f.detailCalls)
	}
}

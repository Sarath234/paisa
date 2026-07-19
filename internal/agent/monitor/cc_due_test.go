package monitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

// fakeFetcher implements CreditCardDetailFetcher. detail keyed by account
// backs CreditCard(account); a missing key yields the not-found (nil, nil)
// result, mirroring the real server's {"found": false} response.
type fakeFetcher struct {
	cards       []paisaclient.CreditCardSummary
	err         error
	detail      map[string]*paisaclient.CreditCardSummary
	detailErr   error
	detailErrs  map[string]error // per-account detail errors
	detailCalls int
}

func (f *fakeFetcher) CreditCards() ([]paisaclient.CreditCardSummary, error) {
	return f.cards, f.err
}

func (f *fakeFetcher) CreditCard(account string) (*paisaclient.CreditCardSummary, error) {
	f.detailCalls++
	if f.detailErr != nil {
		return nil, f.detailErr
	}
	if err := f.detailErrs[account]; err != nil {
		return nil, err
	}
	card, ok := f.detail[account]
	if !ok {
		return nil, nil
	}
	return card, nil
}

// fetcherWithDetail builds a fakeFetcher that mirrors production shape: the
// list (CreditCards) response carries the given bills with postings stripped
// (the real list endpoint never includes postings), while the detail
// (CreditCard) response carries the bills as given, postings included.
func fetcherWithDetail(card paisaclient.CreditCardSummary) *fakeFetcher {
	listCard := card
	listCard.Bills = make([]paisaclient.CreditCardBill, len(card.Bills))
	for i, b := range card.Bills {
		b.Postings = nil
		listCard.Bills[i] = b
	}
	return &fakeFetcher{
		cards:  []paisaclient.CreditCardSummary{listCard},
		detail: map[string]*paisaclient.CreditCardSummary{card.Account: &card},
	}
}

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// cardWithBill's Balance (99999.99) deliberately differs from any test bill's
// ClosingBalance: monitors must quote the bill's ClosingBalance, never the
// card's overall Balance (which reflects the current open cycle, not what's
// actually due on this bill).
func cardWithBill(bill paisaclient.CreditCardBill) paisaclient.CreditCardSummary {
	return paisaclient.CreditCardSummary{
		Account:     "Liabilities:CreditCard:Axis",
		Balance:     99999.99,
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
		if strings.Contains(in.Title, "99999") {
			t.Errorf("%s: title leaked card.Balance instead of bill.ClosingBalance: %q", c.today, in.Title)
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

// TestCCDueRemindsClosedBillWhileOpenCycleExists reproduces the production
// bug: an actively-used card always carries an open current cycle (future
// StatementEndDate/DueDate, PaidDate nil) alongside the closed statement
// that's actually due. cc_due must key off the closed bill, not whichever
// bill happens to have the latest due date (the open cycle always does).
func TestCCDueRemindsClosedBillWhileOpenCycleExists(t *testing.T) {
	card := paisaclient.CreditCardSummary{
		Account: "Liabilities:CreditCard:Axis",
		Bills: []paisaclient.CreditCardBill{
			{ // closed, unpaid, actually due
				StatementEndDate: day("2026-06-15"),
				DueDate:          day("2026-06-28"),
				ClosingBalance:   5000,
			},
			{ // open current cycle — must not be mistaken for the due bill
				StatementEndDate: day("2026-07-15"),
				DueDate:          day("2026-07-28"),
				ClosingBalance:   0,
			},
		},
	}
	m := NewCCDue(&fakeFetcher{cards: []paisaclient.CreditCardSummary{card}}, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-06-26").Add(8 * time.Hour) } // 2 days before the closed bill's due date

	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 {
		t.Fatalf("insights: %+v", insights)
	}
	if !strings.Contains(insights[0].Key, "2026-06-28/d-3") {
		t.Errorf("key: %q, want the closed bill's due date", insights[0].Key)
	}
	if !strings.Contains(insights[0].Title, "₹5000.00") {
		t.Errorf("title: %q, want closed bill's ClosingBalance", insights[0].Title)
	}
}

// TestCCDueEmitsForMultipleUnpaidClosedBills covers a missed payment (an
// older closed cycle still unpaid) plus the current closed statement: both
// are due money and must each get their own reminder.
func TestCCDueEmitsForMultipleUnpaidClosedBills(t *testing.T) {
	card := paisaclient.CreditCardSummary{
		Account: "Liabilities:CreditCard:Axis",
		Bills: []paisaclient.CreditCardBill{
			{ // missed payment from a prior cycle
				StatementEndDate: day("2026-05-15"),
				DueDate:          day("2026-05-28"),
				ClosingBalance:   900,
			},
			{ // current closed statement
				StatementEndDate: day("2026-06-15"),
				DueDate:          day("2026-06-28"),
				ClosingBalance:   23450.5,
			},
		},
	}
	m := NewCCDue(&fakeFetcher{cards: []paisaclient.CreditCardSummary{card}}, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-06-26").Add(8 * time.Hour) }

	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 2 {
		t.Fatalf("insights: %+v", insights)
	}
	var gotOverdue, gotD3 bool
	for _, in := range insights {
		if strings.Contains(in.Key, "2026-05-28/overdue") {
			gotOverdue = true
		}
		if strings.Contains(in.Key, "2026-06-28/d-3") {
			gotD3 = true
		}
	}
	if !gotOverdue || !gotD3 {
		t.Fatalf("expected both overdue and d-3 insights: %+v", insights)
	}
}

func TestCCDueFetchErrorPropagates(t *testing.T) {
	m := NewCCDue(&fakeFetcher{err: errors.New("paisa unreachable")}, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-07-26") }
	_, err := m.Check(context.Background())
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
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

// day builds a local-midnight time.Time. billtruth.Store.Apply normalizes
// every incoming bill date to local-midnight (see billtruth/apply.go's
// localMidnight), and production Now() (time.Now()) is genuinely local —
// so tests build "today" overrides and bill dates the same way to stay
// self-consistent regardless of the host's timezone.
func day(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
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

// truthStore builds an in-memory billtruth.Store (backed by a temp dir)
// seeded with the given bills at AuthoritySMS, mirroring what a real
// statement/payment SMS would produce. cc_due tests read through BillSource,
// not the fetcher, so bill facts replace API fixtures.
func truthStore(t *testing.T, bills ...billtruth.Bill) *billtruth.Store {
	t.Helper()
	s, err := billtruth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bills {
		end := b.PeriodEnd
		due := b.DueDate
		total := b.TotalDue
		f := billtruth.Facts{Account: b.Account, PeriodEnd: &end, DueDate: &due, TotalDue: &total, Source: billtruth.AuthoritySMS}
		if b.PaidDate != nil {
			f.PaidDate = b.PaidDate
		}
		if _, err := s.Apply(f); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestCCDueFromTruthStore(t *testing.T) {
	s := truthStore(t, billtruth.Bill{
		Account:   "Liabilities:CreditCard:ICIC6009",
		PeriodEnd: day("2026-07-10"),
		DueDate:   day("2026-07-30"),
		TotalDue:  23450.50,
	})
	m := NewCCDue(s, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-07-27").Add(8 * time.Hour) }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 || insights[0].Key != "cc-due/Liabilities:CreditCard:ICIC6009/2026-07-30/d-3" {
		t.Fatalf("%+v", insights)
	}
	if !strings.Contains(insights[0].Title, "₹23450.50") {
		t.Errorf("amount from TotalDue: %q", insights[0].Title)
	}
}

// TestCCDueBucketsCorrectlyWithUTCParsedDueDate guards the timezone
// normalization fix in billtruth.Apply: SMS/PDF dates parse as UTC
// midnight (e.g. notices.parseNoticeDate), regardless of host timezone.
// Without normalizing them to local-midnight on the way into the store,
// comparing against DateOnly(time.Now()) (a local midnight) can be off by
// a day, shifting which reminder offset fires. Apply a due date built the
// same way a real UTC-midnight parse would, and confirm the day-math
// still buckets to the right offset.
func TestCCDueBucketsCorrectlyWithUTCParsedDueDate(t *testing.T) {
	s, err := billtruth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	periodEnd := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	total := 23450.50
	if _, err := s.Apply(billtruth.Facts{
		Account: "Liabilities:CreditCard:ICIC6009", PeriodEnd: &periodEnd, DueDate: &due, TotalDue: &total,
		Source: billtruth.AuthoritySMS,
	}); err != nil {
		t.Fatal(err)
	}
	m := NewCCDue(s, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-07-27").Add(8 * time.Hour) }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 || insights[0].Key != "cc-due/Liabilities:CreditCard:ICIC6009/2026-07-30/d-3" {
		t.Fatalf("%+v", insights)
	}
}

func TestCCDuePaidBillSilent(t *testing.T) {
	paid := day("2026-07-25")
	s := truthStore(t, billtruth.Bill{
		Account: "Liabilities:CreditCard:ICIC6009", PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 23450.50, PaidDate: &paid,
	})
	m := NewCCDue(s, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-07-28").Add(8 * time.Hour) }
	insights, _ := m.Check(context.Background())
	if len(insights) != 0 {
		t.Fatalf("paid bill must be silent: %+v", insights)
	}
}

func TestCCDueCorrectedDueDateRefires(t *testing.T) {
	// key embeds due date: correction produces a new key
	s := truthStore(t, billtruth.Bill{
		Account: "Liabilities:CreditCard:ICIC6009", PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 23450.50,
	})
	m := NewCCDue(s, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-07-29").Add(8 * time.Hour) }
	first, _ := m.Check(context.Background())
	// PDF corrects due date to 31st
	end, due2 := day("2026-07-10"), day("2026-07-31")
	s.Apply(billtruth.Facts{Account: "Liabilities:CreditCard:ICIC6009", PeriodEnd: &end, DueDate: &due2, Source: billtruth.AuthorityPDF})
	second, _ := m.Check(context.Background())
	if len(first) != 1 || len(second) != 1 || first[0].Key == second[0].Key {
		t.Fatalf("corrected due date must change key: %v vs %v", first, second)
	}
}

// TestCCDueEmitsSmallestApplicableOffset ports the original fetcher-backed
// scenario: offset selection, self-healing across a missed run (the d-3
// window is skipped and the monitor still fires with the accurate day
// count), and escalation to overdue once the due date has passed.
func TestCCDueEmitsSmallestApplicableOffset(t *testing.T) {
	s := truthStore(t, billtruth.Bill{
		Account:   "Liabilities:CreditCard:Axis",
		PeriodEnd: day("2026-07-15"),
		DueDate:   day("2026-07-28"),
		TotalDue:  23450.5,
	})
	m := NewCCDue(s, []int{7, 3, 1, 0}, 8)

	cases := []struct {
		today   string
		wantKey string
		wantIn  string
	}{
		{"2026-07-21", "cc-due/Liabilities:CreditCard:Axis/2026-07-28/d-7", "in 7 days"},
		{"2026-07-26", "cc-due/Liabilities:CreditCard:Axis/2026-07-28/d-3", "in 2 days"}, // missed d-3 window self-heals
		{"2026-07-28", "cc-due/Liabilities:CreditCard:Axis/2026-07-28/d-0", "today"},
		{"2026-07-30", "cc-due/Liabilities:CreditCard:Axis/2026-07-28/overdue/2026-07-30", "overdue"},
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

// TestCCDueQuietWhenDueFarAway ports the "far away" half of the original
// fakeFetcher TestCCDueQuietWhenFarOrPaid; the "already paid" half is now
// TestCCDuePaidBillSilent.
func TestCCDueQuietWhenDueFarAway(t *testing.T) {
	s := truthStore(t, billtruth.Bill{
		Account: "Liabilities:CreditCard:Axis", PeriodEnd: day("2026-07-15"),
		DueDate: day("2026-08-20"), TotalDue: 100,
	})
	m := NewCCDue(s, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-07-26") }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 0 {
		t.Errorf("due far in the future: want quiet, got %+v", insights)
	}
}

// TestCCDueEmitsForMultipleUnpaidBills ports the original
// TestCCDueEmitsForMultipleUnpaidClosedBills: a missed payment from a prior
// cycle plus the current closed statement are both due money and must each
// get their own reminder, keyed by their own due date.
func TestCCDueEmitsForMultipleUnpaidBills(t *testing.T) {
	s := truthStore(t,
		billtruth.Bill{ // missed payment from a prior cycle
			Account: "Liabilities:CreditCard:Axis", PeriodEnd: day("2026-05-15"),
			DueDate: day("2026-05-28"), TotalDue: 900,
		},
		billtruth.Bill{ // current closed statement
			Account: "Liabilities:CreditCard:Axis", PeriodEnd: day("2026-06-15"),
			DueDate: day("2026-06-28"), TotalDue: 23450.5,
		},
	)
	m := NewCCDue(s, []int{3, 1, 0}, 8)
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

func TestCCDueSkipsUserPaidBill(t *testing.T) {
	s := truthStore(t, billtruth.Bill{
		Account: "Liabilities:CreditCard:ICIC6009", PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 23450.50,
	})
	if err := s.SetUserPaid("Liabilities:CreditCard:ICIC6009", day("2026-07-30")); err != nil {
		t.Fatal(err)
	}
	m := NewCCDue(s, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-07-28").Add(8 * time.Hour) }
	insights, _ := m.Check(context.Background())
	if len(insights) != 0 {
		t.Fatalf("self-reported-paid bill must be silent: %+v", insights)
	}
}

func TestCCDueOverdueRecursDaily(t *testing.T) {
	s := truthStore(t, billtruth.Bill{
		Account: "Liabilities:CreditCard:Axis", PeriodEnd: day("2026-07-15"),
		DueDate: day("2026-07-28"), TotalDue: 900,
	})
	m := NewCCDue(s, []int{3, 1, 0}, 8)

	m.Now = func() time.Time { return day("2026-07-30").Add(8 * time.Hour) }
	day1, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m.Now = func() time.Time { return day("2026-07-31").Add(8 * time.Hour) }
	day2, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(day1) != 1 || len(day2) != 1 {
		t.Fatalf("want 1 overdue insight per day, got %d and %d", len(day1), len(day2))
	}
	if day1[0].Key == day2[0].Key {
		t.Fatalf("overdue key must change day-to-day so it recurs: %q", day1[0].Key)
	}
	if !strings.Contains(day1[0].Key, "overdue/2026-07-30") || !strings.Contains(day2[0].Key, "overdue/2026-07-31") {
		t.Fatalf("keys: %q, %q", day1[0].Key, day2[0].Key)
	}
}

// TestCCDueDegradesToPlainTextWhenCallbackDataTooLong guards against Telegram
// silently rejecting the entire sendMessage call when callback_data exceeds
// its 64-byte hard limit: since Notifier.Deliver's error path never calls
// MarkSent, an oversized callback_data would otherwise make that bill's
// reminder retry and fail identically forever, with no reminder ever
// reaching the user. ccDueButtons must degrade to nil buttons (plain text)
// rather than emit an oversized callback_data.
func TestCCDueDegradesToPlainTextWhenCallbackDataTooLong(t *testing.T) {
	account := "Liabilities:CreditCard:AVeryLongCardNicknameThatExceedsTheLimit1234"
	s := truthStore(t, billtruth.Bill{
		Account: account, PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 23450.50,
	})
	m := NewCCDue(s, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-07-27").Add(8 * time.Hour) }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 || insights[0].Key != "cc-due/"+account+"/2026-07-30/d-3" {
		t.Fatalf("want the insight still emitted with the expected key, got %+v", insights)
	}
	if !strings.Contains(insights[0].Title, "₹23450.50") {
		t.Errorf("insight must still carry the normal title: %q", insights[0].Title)
	}
	if insights[0].Buttons != nil {
		t.Fatalf("want nil buttons (plain text) when callback_data would exceed Telegram's 64-byte limit, got %+v", insights[0].Buttons)
	}
}

func TestCCDueInsightsHaveButtons(t *testing.T) {
	s := truthStore(t, billtruth.Bill{
		Account: "Liabilities:CreditCard:ICIC6009", PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 23450.50,
	})
	m := NewCCDue(s, []int{3, 1, 0}, 8)
	m.Now = func() time.Time { return day("2026-07-27").Add(8 * time.Hour) }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 {
		t.Fatalf("%+v", insights)
	}
	buttons := insights[0].Buttons
	if len(buttons) != 1 || len(buttons[0]) != 2 {
		t.Fatalf("want 1 row of 2 buttons, got %+v", buttons)
	}
	wantPaid := "ccdue_paid:Liabilities:CreditCard:ICIC6009:2026-07-30"
	wantRemind := "ccdue_remind:Liabilities:CreditCard:ICIC6009:2026-07-30"
	if buttons[0][0].CallbackData != wantPaid {
		t.Errorf("paid button: got %q, want %q", buttons[0][0].CallbackData, wantPaid)
	}
	if buttons[0][1].CallbackData != wantRemind {
		t.Errorf("remind button: got %q, want %q", buttons[0][1].CallbackData, wantRemind)
	}
}

package billtruth

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

type fakeLister struct{ cards []paisaclient.CreditCardSummary }

func (f *fakeLister) CreditCards() ([]paisaclient.CreditCardSummary, error) { return f.cards, nil }

func TestSyncFromAPIFillsHolesOnly(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.Now = func() time.Time { return day("2026-07-16") }
	// SMS truth already present for the July cycle
	s.Apply(Facts{
		Account:   acct,
		PeriodEnd: ptrT(day("2026-07-10")),
		DueDate:   ptrT(day("2026-07-30")),
		TotalDue:  ptrF(23450.50),
		Source:    AuthoritySMS,
	})
	paid := day("2026-06-28")
	err := SyncFromAPI(s, &fakeLister{cards: []paisaclient.CreditCardSummary{{
		Account: acct,
		Bills: []paisaclient.CreditCardBill{
			{ // June cycle: nothing else knows it → filled at api authority
				StatementStartDate: day("2026-05-11"),
				StatementEndDate:   day("2026-06-10"),
				DueDate:            day("2026-06-30"),
				ClosingBalance:     9000.00,
				PaidDate:           &paid,
			},
			{ // July cycle: computed guess must NOT overwrite SMS truth
				StatementStartDate: day("2026-06-11"),
				StatementEndDate:   day("2026-07-10"),
				DueDate:            day("2026-07-28"),
				ClosingBalance:     20000.00,
			},
			{ // open cycle: skipped
				StatementStartDate: day("2026-07-11"),
				StatementEndDate:   day("2026-08-10"),
				DueDate:            day("2026-08-30"),
				ClosingBalance:     500.00,
			},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	bills := s.BillsFor(acct)
	if len(bills) != 2 {
		t.Fatalf("want June+July only (open cycle skipped): %+v", bills)
	}
	// newest first: July keeps SMS truth
	if bills[0].TotalDue != 23450.50 || !bills[0].DueDate.Equal(day("2026-07-30")) {
		t.Fatalf("api overwrote sms: %+v", bills[0])
	}
	// June filled from api, including PaidDate
	if bills[1].TotalDue != 9000.00 || bills[1].PaidDate == nil {
		t.Fatalf("june not filled: %+v", bills[1])
	}
}

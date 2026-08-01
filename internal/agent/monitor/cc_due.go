package monitor

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
	"github.com/ananthakumaran/paisa/internal/agent/telegram"
)

// CreditCardFetcher is the slice of paisaclient the credit-card monitors need.
type CreditCardFetcher interface {
	CreditCards() ([]paisaclient.CreditCardSummary, error)
}

// CCDueMonitor reminds about unpaid credit card bills at configured
// days-before-due offsets, self-healing across missed runs: it emits the
// smallest configured offset ≥ actual days-until-due, with the real days in
// the message; the notifier's key dedupe drops already-sent buckets.
type CCDueMonitor struct {
	Now          func() time.Time
	bills        BillSource
	reminderDays []int // sorted ascending
	due          func(now, lastRun time.Time) bool
}

func NewCCDue(bills BillSource, reminderDays []int, digestHour int) *CCDueMonitor {
	days := append([]int{}, reminderDays...)
	sort.Ints(days)
	return &CCDueMonitor{
		Now:          time.Now,
		bills:        bills,
		reminderDays: days,
		due:          DailyAt(digestHour),
	}
}

func (m *CCDueMonitor) Name() string { return "cc_due" }

func (m *CCDueMonitor) Due(now, lastRun time.Time) bool { return m.due(now, lastRun) }

func (m *CCDueMonitor) Check(ctx context.Context) ([]Insight, error) {
	today := DateOnly(m.Now())
	var insights []Insight
	for _, account := range m.bills.Accounts() {
		for _, bill := range m.bills.BillsFor(account) {
			if bill.PaidDate != nil || bill.UserPaidDate != nil || bill.DueDate.IsZero() || bill.TotalDue <= 0 {
				continue
			}
			if !today.After(DateOnly(bill.PeriodEnd)) {
				continue // period not closed yet (defensive; store holds closed bills)
			}
			// Both operands are local midnights: billtruth.Store.Apply
			// normalizes every incoming SMS/PDF date to local-midnight
			// (see billtruth/apply.go's localMidnight) specifically so
			// bill.DueDate always shares time.Local with today
			// (DateOnly of the real, local time.Now()) — without that
			// normalization a UTC-parsed date could sit a day off from
			// "today" depending on the host's UTC offset. Given that
			// invariant, in any fixed-offset timezone (IST included) the
			// difference below is an exact multiple of 24h. A DST
			// transition inside the window would skew it by an hour and
			// truncate the count by a day — acceptable for reminder
			// bucketing.
			days := int(DateOnly(bill.DueDate).Sub(today) / (24 * time.Hour))
			dueDate := bill.DueDate.Format("2006-01-02")
			if days < 0 {
				insights = append(insights, Insight{
					Key:     fmt.Sprintf("cc-due/%s/%s/overdue/%s", account, dueDate, today.Format("2006-01-02")),
					Urgency: Immediate,
					Title: fmt.Sprintf("🚨 %s on %s is overdue (was due %s)",
						INR(bill.TotalDue), Short(account), bill.DueDate.Format("02 Jan")),
					Buttons: ccDueButtons(account, dueDate),
				})
				continue
			}
			offset := -1
			for _, o := range m.reminderDays {
				if o >= days {
					offset = o
					break
				}
			}
			if offset < 0 {
				continue
			}
			insights = append(insights, Insight{
				Key:     fmt.Sprintf("cc-due/%s/%s/d-%d", account, dueDate, offset),
				Urgency: Immediate,
				Title: fmt.Sprintf("💳 %s due on %s %s (%s)",
					INR(bill.TotalDue), Short(account), inDays(days), bill.DueDate.Format("02 Jan")),
				Buttons: ccDueButtons(account, dueDate),
			})
		}
	}
	return insights, nil
}

func ccDueButtons(account, dueDate string) [][]telegram.Button {
	return [][]telegram.Button{{
		{Text: "✅ Paid", CallbackData: fmt.Sprintf("ccdue_paid:%s:%s", account, dueDate)},
		{Text: "⏰ Remind later", CallbackData: fmt.Sprintf("ccdue_remind:%s:%s", account, dueDate)},
	}}
}

func inDays(days int) string {
	switch days {
	case 0:
		return "today"
	case 1:
		return "tomorrow"
	default:
		return fmt.Sprintf("in %d days", days)
	}
}

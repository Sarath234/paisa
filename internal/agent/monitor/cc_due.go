package monitor

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
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
	client       CreditCardFetcher
	reminderDays []int // sorted ascending
	due          func(now, lastRun time.Time) bool
}

func NewCCDue(client CreditCardFetcher, reminderDays []int, digestHour int) *CCDueMonitor {
	days := append([]int{}, reminderDays...)
	sort.Ints(days)
	return &CCDueMonitor{
		Now:          time.Now,
		client:       client,
		reminderDays: days,
		due:          DailyAt(digestHour),
	}
}

func (m *CCDueMonitor) Name() string { return "cc_due" }

func (m *CCDueMonitor) Due(now, lastRun time.Time) bool { return m.due(now, lastRun) }

func (m *CCDueMonitor) Check(ctx context.Context) ([]Insight, error) {
	cards, err := m.client.CreditCards()
	if err != nil {
		return nil, err
	}
	today := DateOnly(m.Now())
	var insights []Insight
	for _, card := range cards {
		for _, bill := range closedUnpaidBills(card, today) {
			days := int(DateOnly(bill.DueDate).Sub(today) / (24 * time.Hour))
			dueDate := bill.DueDate.Format("2006-01-02")
			if days < 0 {
				insights = append(insights, Insight{
					Key:     fmt.Sprintf("cc-due/%s/%s/overdue", card.Account, dueDate),
					Urgency: Immediate,
					Title: fmt.Sprintf("🚨 %s on %s is overdue (was due %s)",
						INR(bill.ClosingBalance), Short(card.Account), bill.DueDate.Format("02 Jan")),
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
				Key:     fmt.Sprintf("cc-due/%s/%s/d-%d", card.Account, dueDate, offset),
				Urgency: Immediate,
				Title: fmt.Sprintf("💳 %s due on %s %s (%s)",
					INR(bill.ClosingBalance), Short(card.Account), inDays(days), bill.DueDate.Format("02 Jan")),
			})
		}
	}
	return insights, nil
}

// closedUnpaidBills returns every bill whose statement period has closed
// (StatementEndDate strictly before today, date-truncated) and is still
// unpaid (PaidDate nil). Cards can carry more than one such bill at once —
// e.g. a missed payment from a prior cycle plus the current statement — and
// each must be reminded about independently; the open current cycle
// (future StatementEndDate) is excluded since it isn't due yet.
func closedUnpaidBills(card paisaclient.CreditCardSummary, today time.Time) []paisaclient.CreditCardBill {
	var bills []paisaclient.CreditCardBill
	for _, b := range card.Bills {
		if b.PaidDate != nil {
			continue
		}
		if !today.After(DateOnly(b.StatementEndDate)) {
			continue
		}
		bills = append(bills, b)
	}
	return bills
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

package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

// CCStatementMonitor announces a card's statement once its period closes.
// It always emits for the latest closed bill; the notifier's key dedupe
// ensures each statement is announced once (late after downtime, never lost).
type CCStatementMonitor struct {
	Now    func() time.Time
	client CreditCardFetcher
	due    func(now, lastRun time.Time) bool
}

func NewCCStatement(client CreditCardFetcher, digestHour int) *CCStatementMonitor {
	return &CCStatementMonitor{Now: time.Now, client: client, due: DailyAt(digestHour)}
}

func (m *CCStatementMonitor) Name() string { return "cc_statement" }

func (m *CCStatementMonitor) Due(now, lastRun time.Time) bool { return m.due(now, lastRun) }

func (m *CCStatementMonitor) Check(ctx context.Context) ([]Insight, error) {
	cards, err := m.client.CreditCards()
	if err != nil {
		return nil, err
	}
	today := DateOnly(m.Now())
	var insights []Insight
	for _, card := range cards {
		bill := latestClosedBill(card, today)
		if bill == nil {
			continue
		}
		insights = append(insights, Insight{
			Key:     fmt.Sprintf("cc-stmt/%s/%s", card.Account, bill.StatementEndDate.Format("2006-01-02")),
			Urgency: Digest,
			Title: fmt.Sprintf("📄 Statement generated for %s: %s (%s – %s)",
				Short(card.Account), INR(bill.ClosingBalance),
				bill.StatementStartDate.Format("02 Jan"), bill.StatementEndDate.Format("02 Jan")),
		})
	}
	return insights, nil
}

// latestClosedBill returns the bill with the max StatementEndDate strictly
// before today (date-truncated), or nil. Cards in active use always carry a
// current, still-open cycle (future StatementEndDate, PaidDate nil) alongside
// closed ones — that open cycle must never be mistaken for the latest closed
// statement, so bills are filtered to closed ones before taking the max.
func latestClosedBill(card paisaclient.CreditCardSummary, today time.Time) *paisaclient.CreditCardBill {
	var latest *paisaclient.CreditCardBill
	for i := range card.Bills {
		b := &card.Bills[i]
		if !today.After(DateOnly(b.StatementEndDate)) {
			continue
		}
		if latest == nil || b.StatementEndDate.After(latest.StatementEndDate) {
			latest = b
		}
	}
	return latest
}

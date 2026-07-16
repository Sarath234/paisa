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
		bill := latestBill(card)
		if bill == nil || !today.After(DateOnly(bill.StatementEndDate)) {
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

// latestBill returns the bill with the latest statement end date, or nil.
func latestBill(card paisaclient.CreditCardSummary) *paisaclient.CreditCardBill {
	var latest *paisaclient.CreditCardBill
	for i := range card.Bills {
		b := &card.Bills[i]
		if latest == nil || b.StatementEndDate.After(latest.StatementEndDate) {
			latest = b
		}
	}
	return latest
}

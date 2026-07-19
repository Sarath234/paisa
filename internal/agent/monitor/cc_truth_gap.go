package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
)

// CCTruthGapMonitor nudges when a computed statement day passed gapDays ago
// but no statement SMS/PDF has arrived for that cycle — the signal that the
// human-in-the-loop pipeline is degrading to computed guesses.
type CCTruthGapMonitor struct {
	Now     func() time.Time
	bills   BillSource
	client  CreditCardFetcher
	gapDays int
	due     func(now, lastRun time.Time) bool
}

func NewCCTruthGap(bills BillSource, client CreditCardFetcher, gapDays, digestHour int) *CCTruthGapMonitor {
	return &CCTruthGapMonitor{Now: time.Now, bills: bills, client: client, gapDays: gapDays, due: DailyAt(digestHour)}
}

func (m *CCTruthGapMonitor) Name() string { return "cc_truth_gap" }

func (m *CCTruthGapMonitor) Due(now, lastRun time.Time) bool { return m.due(now, lastRun) }

func (m *CCTruthGapMonitor) Check(ctx context.Context) ([]Insight, error) {
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
		gap := int(today.Sub(DateOnly(bill.StatementEndDate)) / (24 * time.Hour))
		if gap <= m.gapDays {
			continue
		}
		if m.hasTruth(card.Account, bill.StatementEndDate) {
			continue
		}
		insights = append(insights, Insight{
			Key:     fmt.Sprintf("cc-truth-gap/%s/%s", card.Account, bill.StatementEndDate.Format("2006-01-02")),
			Urgency: Digest,
			Title: fmt.Sprintf("📮 No statement seen for %s (cycle ~%s) — forward the statement SMS or drop the PDF",
				Short(card.Account), bill.StatementEndDate.Format("02 Jan")),
		})
	}
	return insights, nil
}

func (m *CCTruthGapMonitor) hasTruth(account string, periodEnd time.Time) bool {
	for _, b := range m.bills.BillsFor(account) {
		diff := b.PeriodEnd.Sub(periodEnd)
		if diff < 0 {
			diff = -diff
		}
		if diff <= 7*24*time.Hour && b.Sources["total_due"] >= billtruth.AuthoritySMS {
			return true
		}
	}
	return false
}

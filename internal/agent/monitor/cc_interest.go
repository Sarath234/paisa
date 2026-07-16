package monitor

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// CCInterestMonitor detects interest/fee charges in the most recent closed
// statement by case-insensitive substring match on posting payees. Charges on
// a liability account are negative; amounts are summed as absolute values.
type CCInterestMonitor struct {
	Now      func() time.Time
	client   CreditCardFetcher
	patterns []string // uppercased at construction
	due      func(now, lastRun time.Time) bool
}

func NewCCInterest(client CreditCardFetcher, patterns []string, digestHour int) *CCInterestMonitor {
	upper := make([]string, len(patterns))
	for i, p := range patterns {
		upper[i] = strings.ToUpper(p)
	}
	return &CCInterestMonitor{Now: time.Now, client: client, patterns: upper, due: DailyAt(digestHour)}
}

func (m *CCInterestMonitor) Name() string { return "cc_interest" }

func (m *CCInterestMonitor) Due(now, lastRun time.Time) bool { return m.due(now, lastRun) }

func (m *CCInterestMonitor) Check(ctx context.Context) ([]Insight, error) {
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
		var total float64
		for _, p := range bill.Postings {
			payee := strings.ToUpper(p.Payee)
			for _, pat := range m.patterns {
				if strings.Contains(payee, pat) {
					total += math.Abs(p.Amount)
					break
				}
			}
		}
		if total == 0 {
			continue
		}
		insights = append(insights, Insight{
			Key:     fmt.Sprintf("cc-interest/%s/%s", card.Account, bill.StatementEndDate.Format("2006-01-02")),
			Urgency: Immediate,
			Title: fmt.Sprintf("⚠️ You paid %s in interest/fees on %s last cycle — consider paying the full balance",
				INR(total), Short(card.Account)),
		})
	}
	return insights, nil
}

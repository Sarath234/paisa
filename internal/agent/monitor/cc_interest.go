package monitor

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

// CreditCardDetailFetcher extends CreditCardFetcher with the per-account
// detail call that includes bill postings. The list endpoint
// (CreditCardFetcher.CreditCards) always returns bills with empty Postings.
type CreditCardDetailFetcher interface {
	CreditCardFetcher
	CreditCard(account string) (*paisaclient.CreditCardSummary, error)
}

// CCInterestMonitor detects interest/fee charges in the most recent closed
// statement by case-insensitive substring match on posting payees. Charges on
// a liability account are negative; amounts are summed as absolute values.
type CCInterestMonitor struct {
	Now      func() time.Time
	client   CreditCardDetailFetcher
	patterns []string // uppercased at construction
	due      func(now, lastRun time.Time) bool
}

func NewCCInterest(client CreditCardDetailFetcher, patterns []string, digestHour int) *CCInterestMonitor {
	upper := make([]string, len(patterns))
	for i, p := range patterns {
		upper[i] = strings.ToUpper(p)
	}
	return &CCInterestMonitor{Now: time.Now, client: client, patterns: upper, due: DailyAt(digestHour)}
}

func (m *CCInterestMonitor) Name() string { return "cc_interest" }

func (m *CCInterestMonitor) Due(now, lastRun time.Time) bool { return m.due(now, lastRun) }

func (m *CCInterestMonitor) Check(ctx context.Context) ([]Insight, error) {
	// The list endpoint never includes postings, so it's only used to
	// enumerate accounts; postings come from the per-account detail call.
	cards, err := m.client.CreditCards()
	if err != nil {
		return nil, err
	}
	today := DateOnly(m.Now())
	var insights []Insight
	for _, card := range cards {
		detail, err := m.client.CreditCard(card.Account)
		if err != nil {
			return nil, err
		}
		if detail == nil {
			continue // not found (e.g. removed from config since the list call)
		}
		bill := latestClosedBill(*detail, today)
		if bill == nil {
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

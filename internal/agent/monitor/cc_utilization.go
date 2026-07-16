package monitor

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// CCUtilizationMonitor warns when a card's balance crosses configured
// utilization bands. The key includes band and month: crossing a higher band
// re-alerts (escalation) and a sustained balance re-alerts monthly.
type CCUtilizationMonitor struct {
	Now    func() time.Time
	client CreditCardFetcher
	bands  []int // sorted ascending
	due    func(now, lastRun time.Time) bool
}

func NewCCUtilization(client CreditCardFetcher, bands []int, digestHour int) *CCUtilizationMonitor {
	sorted := append([]int{}, bands...)
	sort.Ints(sorted)
	return &CCUtilizationMonitor{Now: time.Now, client: client, bands: sorted, due: DailyAt(digestHour)}
}

func (m *CCUtilizationMonitor) Name() string { return "cc_utilization" }

func (m *CCUtilizationMonitor) Due(now, lastRun time.Time) bool { return m.due(now, lastRun) }

func (m *CCUtilizationMonitor) Check(ctx context.Context) ([]Insight, error) {
	cards, err := m.client.CreditCards()
	if err != nil {
		return nil, err
	}
	month := m.Now().Format("2006-01")
	var insights []Insight
	for _, card := range cards {
		if card.CreditLimit <= 0 {
			continue
		}
		pct := card.Balance / card.CreditLimit * 100
		band := -1
		for _, b := range m.bands {
			if pct >= float64(b) {
				band = b
			}
		}
		if band < 0 {
			continue
		}
		insights = append(insights, Insight{
			Key:     fmt.Sprintf("cc-util/%s/%d/%s", card.Account, band, month),
			Urgency: Digest,
			Title: fmt.Sprintf("⚠️ %s card at %.0f%% of credit limit (%s / %s)",
				Short(card.Account), pct, INR(card.Balance), INR(card.CreditLimit)),
		})
	}
	return insights, nil
}

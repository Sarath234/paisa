package monitor

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
	log "github.com/sirupsen/logrus"
)

// CreditCardDetailFetcher extends CreditCardFetcher with the per-account
// detail call that includes bill postings. The list endpoint
// (CreditCardFetcher.CreditCards) always returns bills with empty Postings.
type CreditCardDetailFetcher interface {
	CreditCardFetcher
	CreditCard(account string) (*paisaclient.CreditCardSummary, error)
}

// CCInterestMonitor detects interest/fee charges in the most recent closed
// statement, preferring a PDF-sourced total from billtruth when the parsed
// statement recorded one; otherwise it falls back to case-insensitive
// substring match on posting payees fetched from the API. Charges on a
// liability account are negative; amounts are summed as absolute values.
type CCInterestMonitor struct {
	Now   func() time.Time
	bills BillSource
	// Sent reports whether an insight key was already delivered (wire to
	// Store.WasSent). When set, cards whose latest closed statement was
	// already announced skip the per-account detail fetch entirely —
	// without it every run costs one detail call per card. Nil means
	// never sent. Only consulted on the API-fallback path; the PDF-total
	// path never calls the API at all.
	Sent     func(key string) bool
	client   CreditCardDetailFetcher
	patterns []string // uppercased at construction
	due      func(now, lastRun time.Time) bool
}

func NewCCInterest(bills BillSource, client CreditCardDetailFetcher, patterns []string, digestHour int) *CCInterestMonitor {
	upper := make([]string, len(patterns))
	for i, p := range patterns {
		upper[i] = strings.ToUpper(p)
	}
	return &CCInterestMonitor{Now: time.Now, bills: bills, client: client, patterns: upper, due: DailyAt(digestHour)}
}

func (m *CCInterestMonitor) Name() string { return "cc_interest" }

func (m *CCInterestMonitor) Due(now, lastRun time.Time) bool { return m.due(now, lastRun) }

func (m *CCInterestMonitor) Check(ctx context.Context) ([]Insight, error) {
	today := DateOnly(m.Now())
	var insights []Insight

	// First pass: decide per account, from billtruth alone, whether a
	// PDF-sourced interest total is already on hand. Those accounts are
	// resolved here and must never reach the API detail call below.
	fallback := map[string]bool{}
	for _, account := range m.bills.Accounts() {
		bills := m.bills.BillsFor(account)
		if len(bills) == 0 {
			continue
		}
		b := bills[0] // newest closed truth bill
		if !today.After(DateOnly(b.PeriodEnd)) {
			continue
		}
		if b.Sources["interest_total"] == billtruth.AuthorityPDF {
			if b.InterestTotal > 0 {
				insights = append(insights, Insight{
					// Keyed by statement MONTH, not exact PeriodEnd date —
					// see interestKey below for why (same premise as
					// cc_statement's month-keying: a PeriodEnd shift
					// between the API fallback path and this PDF path
					// must resolve to the same identity, not double-fire).
					Key:     fmt.Sprintf("cc-interest/%s/%s", account, b.PeriodEnd.Format("2006-01")),
					Urgency: Immediate,
					Title: fmt.Sprintf("⚠️ You paid %s in interest/fees on %s last cycle — consider paying the full balance",
						INR(b.InterestTotal), Short(account)),
				})
			}
			continue // PDF truth present — API detail must never be fetched
		}
		fallback[account] = true
	}
	if len(fallback) == 0 {
		return insights, nil
	}

	// Existing list→Sent-skip→detail flow, filtered to accounts with no
	// PDF-sourced interest total. The list endpoint never includes
	// postings, so it's only used to enumerate accounts and closed-bill
	// dates; postings come from the per-account detail call.
	cards, err := m.client.CreditCards()
	if err != nil {
		return nil, err
	}
	for _, card := range cards {
		if !fallback[card.Account] {
			continue
		}
		// The list response carries bill dates (only postings are
		// stripped), so both the no-closed-bill and already-sent cases
		// are decided here without the per-account detail call.
		listBill := latestClosedBill(card, today)
		if listBill == nil {
			continue
		}
		if m.Sent != nil && m.Sent(interestKey(card.Account, *listBill)) {
			continue
		}
		detail, err := m.client.CreditCard(card.Account)
		if err != nil {
			// One card's failure must not silence the rest; this
			// statement stays unsent, so it is retried tomorrow.
			log.Warnf("cc_interest: %s: %v", card.Account, err)
			continue
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
			Key:     interestKey(card.Account, *bill),
			Urgency: Immediate,
			Title: fmt.Sprintf("⚠️ You paid %s in interest/fees on %s last cycle — consider paying the full balance",
				INR(total), Short(card.Account)),
		})
	}
	return insights, nil
}

// interestKey is keyed by statement MONTH (not the exact StatementEndDate)
// so that a PeriodEnd shift between this API-fallback path and the
// PDF-truth path in Check above (billtruth's own ±7d bill-identity
// imprecision — see billtruth/apply.go's findBillLocked) resolves to the
// same key instead of double-firing the same cycle's interest insight
// under two dates. Trade-off: a correction that crosses a month boundary
// (e.g. Jul-31 -> Aug-2) re-announces once — accepted, matches cc_statement.
func interestKey(account string, bill paisaclient.CreditCardBill) string {
	return fmt.Sprintf("cc-interest/%s/%s", account, bill.StatementEndDate.Format("2006-01"))
}

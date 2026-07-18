package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

// CCStatementMonitor announces a card's statement once its period closes,
// reading truth-merged bills (billtruth). The announce key embeds the total
// rounded to the whole rupee: cc-stmt/<account>/<period>/<amount>. When a
// PDF later corrects the total by enough to change that rounded amount, the
// new total produces a distinct key — the notifier's key dedupe delivers it
// as a fresh message rather than dropping it, and the monitor formats it as
// a correction instead of a fresh announcement.
type CCStatementMonitor struct {
	Now func() time.Time
	// SentPrefix reports whether any previously-sent key starts with the
	// given prefix (wired to Store.WasSentPrefix). It serves two roles
	// here: checking whether this bill was announced under some earlier
	// amount (pass the account/period prefix), and — since no insight key
	// in this scheme is a strict extension of another — checking whether
	// this exact announce key was already sent (pass the full key). Nil
	// means never sent.
	SentPrefix func(prefix string) bool
	bills      BillSource
	due        func(now, lastRun time.Time) bool
}

func NewCCStatement(bills BillSource, digestHour int) *CCStatementMonitor {
	return &CCStatementMonitor{Now: time.Now, bills: bills, due: DailyAt(digestHour)}
}

func (m *CCStatementMonitor) Name() string { return "cc_statement" }

func (m *CCStatementMonitor) Due(now, lastRun time.Time) bool { return m.due(now, lastRun) }

func (m *CCStatementMonitor) Check(ctx context.Context) ([]Insight, error) {
	today := DateOnly(m.Now())
	var insights []Insight
	for _, account := range m.bills.Accounts() {
		bills := m.bills.BillsFor(account)
		if len(bills) == 0 {
			continue
		}
		b := bills[0] // newest (billtruth never holds open-cycle bills; see SyncFromAPI)
		if !today.After(DateOnly(b.PeriodEnd)) {
			continue
		}
		period := b.PeriodEnd.Format("2006-01-02")
		prefix := fmt.Sprintf("cc-stmt/%s/%s/", account, period)
		announceKey := fmt.Sprintf("%s%.0f", prefix, b.TotalDue)

		if m.SentPrefix != nil && m.SentPrefix(announceKey) {
			continue // this exact amount was already delivered
		}
		if m.SentPrefix != nil && m.SentPrefix(prefix) && b.Sources["total_due"] == billtruth.AuthorityPDF {
			// Announced before under a different amount, and the PDF is
			// the authority for this new total: a genuine correction.
			insights = append(insights, Insight{
				Key:     announceKey,
				Urgency: Digest,
				Title: fmt.Sprintf("✏️ %s statement total corrected to %s (per PDF)",
					Short(account), INR(b.TotalDue)),
			})
			continue
		}
		insights = append(insights, Insight{
			Key:     announceKey,
			Urgency: Digest,
			Title: fmt.Sprintf("📄 Statement generated for %s: %s (%s – %s)",
				Short(account), INR(b.TotalDue),
				b.PeriodStart.Format("02 Jan"), b.PeriodEnd.Format("02 Jan")),
		})
	}
	return insights, nil
}

// latestClosedBill returns the bill with the max StatementEndDate strictly
// before today (date-truncated), or nil. Cards in active use always carry a
// current, still-open cycle (future StatementEndDate, PaidDate nil) alongside
// closed ones — that open cycle must never be mistaken for the latest closed
// statement, so bills are filtered to closed ones before taking the max.
// Used by cc_interest's API-detail fallback path.
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

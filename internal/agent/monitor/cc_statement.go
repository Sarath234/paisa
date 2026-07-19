package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

// CCStatementMonitor announces a card's statement once its period closes,
// reading truth-merged bills (billtruth). The announce key embeds the
// statement MONTH (not the exact PeriodEnd date) and the total rounded to
// the whole rupee: cc-stmt/<account>/<YYYY-MM>/<amount>. Keying by month
// rather than exact date keeps the key stable across a PeriodEnd shift
// (e.g. an API-guessed date corrected a couple of days by SMS/PDF) — a
// full-date key would miss the prior SentPrefix and re-announce the whole
// bill. When a PDF later corrects the total by enough to change the
// rounded amount, the new total produces a distinct key — the notifier's
// key dedupe delivers it as a fresh message rather than dropping it, and
// the monitor formats it as a correction instead of a fresh announcement.
//
// Trade-off: a genuine cycle whose PeriodEnd is corrected across a month
// boundary (e.g. Jul-31 -> Aug-2) lands under a new month key and
// re-announces once. Accepted — it matches billtruth's own ±7d bill-identity
// imprecision (see billtruth/apply.go's findBillLocked) and is rare.
type CCStatementMonitor struct {
	Now func() time.Time
	// SentPrefix reports whether any previously-sent key starts with the
	// given prefix (wired to Store.WasSentPrefix) — used ONLY to detect
	// "this bill was announced before under some earlier amount" (pass the
	// account/month prefix), to decide whether a changed total is a
	// correction. Nil means never sent.
	SentPrefix func(prefix string) bool
	// Sent reports whether this exact announce key was already delivered
	// (wired to Store.WasSent). Using an exact check here — rather than
	// SentPrefix on the full key — matters because SentPrefix is a raw
	// string-prefix scan: "cc-stmt/acct/2026-07/100" is itself a string
	// prefix of an already-sent "cc-stmt/acct/2026-07/1004" key, so using
	// SentPrefix for this check could wrongly treat an unrelated ₹100 bill
	// as already delivered. Nil means never sent.
	Sent  func(key string) bool
	bills BillSource
	due   func(now, lastRun time.Time) bool
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
		month := b.PeriodEnd.Format("2006-01")
		prefix := fmt.Sprintf("cc-stmt/%s/%s/", account, month)
		announceKey := fmt.Sprintf("%s%.0f", prefix, b.TotalDue)

		if m.Sent != nil && m.Sent(announceKey) {
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

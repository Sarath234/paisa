// Package ccrecon orchestrates credit-card statement intake: decrypt, parse,
// merge bill facts into billtruth at PDF authority, reconcile against the
// ledger, and surface missing transactions as Telegram approval cards using
// the same approve/edit/reject flow as sms-spend entries.
package ccrecon

import (
	"fmt"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/approval"
	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/ledger"
	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/ananthakumaran/paisa/internal/agent/reconcile"
	"github.com/ananthakumaran/paisa/internal/agent/statement"

	log "github.com/sirupsen/logrus"
)

// UnclassifiedAccount is the expense account used for a missing CC spend
// whose merchant doesn't match any configured rule. The user edits the
// account on the approval card before posting — no LLM fallback here.
const UnclassifiedAccount = "Expenses:Unclassified"

// defaultMaxCards is used when Deps.MaxCards is left at zero.
const defaultMaxCards = 10

// ccDateWindow mirrors reconcile.CompareCC's own matching window: postings
// are fetched from a slightly wider range so near-boundary transactions
// aren't excluded before CompareCC gets to look at them.
const ccDateWindow = 3 * 24 * time.Hour

// Bot is the subset of telegram.Bot ccrecon needs.
type Bot interface {
	SendText(text string) error
	SendDraft(text string) (int, error)
}

// PaisaClient is the subset of paisaclient.Client ccrecon needs. SyncJournal
// isn't called by HandleCCStatement yet — it's here for the next task
// (post-approval journal sync) so Deps.Client doesn't need to change shape.
type PaisaClient interface {
	Postings() ([]paisaclient.Posting, error)
	SyncJournal() error
}

// Deps holds everything HandleCCStatement needs, built once in main.go.
type Deps struct {
	Store      *billtruth.Store
	Parsers    map[string]statement.CCParser // key: bank id from parser Name()
	Client     PaisaClient
	Approvals  *approval.Store
	Bot        Bot
	ChatID     int64
	Merchants  []config.MerchantRule
	JournalDir string
	MaxCards   int // action cards per run, default 10
}

// HandleCCStatement decrypts (if needed), parses the statement PDF for
// ledgerAccount, merges its facts into billtruth at PDF authority,
// reconciles against ledger postings for the statement period, writes the
// reconciliation record, sends a summary, and raises approval cards (capped
// at MaxCards) for transactions missing from the ledger.
func (d *Deps) HandleCCStatement(filename string, pdfBytes []byte, ledgerAccount, password string) error {
	maxCards := d.MaxCards
	if maxCards == 0 {
		maxCards = defaultMaxCards
	}

	pdf := pdfBytes
	if password != "" {
		decrypted, err := statement.Decrypt(pdfBytes, password)
		if err != nil {
			d.reportErr(fmt.Sprintf("Failed to decrypt %s: %v", filename, err))
			return fmt.Errorf("ccrecon: decrypt %s: %w", filename, err)
		}
		pdf = decrypted
	}

	p, err := parserFor(ledgerAccount, d.Parsers)
	if err != nil {
		d.reportErr(fmt.Sprintf("Failed to route statement %s: %v", filename, err))
		return err
	}

	res, err := p.Parse(pdf)
	if err != nil {
		d.reportErr(fmt.Sprintf("Failed to parse statement %s (%s): %v", filename, p.Name(), err))
		return fmt.Errorf("ccrecon: parse %s: %w", p.Name(), err)
	}

	var interestTotal float64
	for _, tx := range res.Transactions {
		if tx.IsInterestOrFee {
			interestTotal += tx.Debit
		}
	}

	periodStart, periodEnd, dueDate := res.PeriodStart, res.PeriodEnd, res.DueDate
	totalDue, minDue := res.TotalDue, res.MinDue
	facts := billtruth.Facts{
		Account:     ledgerAccount,
		PeriodStart: &periodStart,
		PeriodEnd:   &periodEnd,
		DueDate:     &dueDate,
		TotalDue:    &totalDue,
		MinDue:      &minDue,
		Source:      billtruth.AuthorityPDF,
	}
	if interestTotal > 0 {
		facts.InterestTotal = &interestTotal
	}
	if _, err := d.Store.Apply(facts); err != nil {
		log.Errorf("ccrecon: apply bill facts %s: %v", ledgerAccount, err)
	}

	postings, err := d.Client.Postings()
	if err != nil {
		d.reportErr(fmt.Sprintf("Failed to fetch ledger for %s: %v", ledgerAccount, err))
		return fmt.Errorf("ccrecon: fetch postings: %w", err)
	}

	windowStart := periodStart.Add(-ccDateWindow)
	windowEnd := periodEnd.Add(ccDateWindow)
	var entries []reconcile.LedgerEntry
	for _, posting := range postings {
		if posting.Account != ledgerAccount {
			continue
		}
		if posting.Date.Before(windowStart) || posting.Date.After(windowEnd) {
			continue
		}
		entries = append(entries, reconcile.LedgerEntry{
			Date:        posting.Date,
			Description: posting.Payee,
			Amount:      posting.Amount,
		})
	}

	diff := reconcile.CompareCC(res, entries)
	diff.Account = ledgerAccount

	rec := reconcile.Record{
		Period:      periodEnd.Format("2006-01"),
		GeneratedAt: time.Now(),
		Diff:        diff,
	}
	if err := reconcile.Write(d.JournalDir, rec); err != nil {
		log.Errorf("ccrecon: write reconcile record %s: %v", ledgerAccount, err)
	}

	matched := len(res.Transactions) - len(diff.Missing)
	summary := fmt.Sprintf("%s %s-cycle: %d matched, %d missing, %d extra",
		shortAccount(ledgerAccount), periodEnd.Format("Jan"), matched, len(diff.Missing), len(diff.Extra))

	shown := diff.Missing
	if len(shown) > maxCards {
		summary += fmt.Sprintf(" (%d more)", len(shown)-maxCards)
		shown = shown[:maxCards]
	}
	if err := d.Bot.SendText(summary); err != nil {
		log.Errorf("ccrecon: send summary: %v", err)
	}

	for _, tx := range shown {
		desc := tx.Description
		expenseAccount := UnclassifiedAccount
		if account, matchedDesc := parser.RouteMerchant(tx.Description, d.Merchants); account != "" {
			expenseAccount = account
			desc = matchedDesc
		}
		entry := ledger.Entry{
			Date:   tx.Date.Format("2006/01/02"),
			Desc:   desc,
			Src:    ledgerAccount,
			Amt:    fmt.Sprintf("%.2f INR", -(tx.Debit - tx.Credit)),
			Dest:   expenseAccount,
			Source: "reconcile",
		}
		msgID, err := d.Bot.SendDraft(entry.Format())
		if err != nil {
			log.Errorf("ccrecon: send draft: %v", err)
			continue
		}
		d.Approvals.Set(&approval.Pending{
			Entry:     entry,
			Original:  entry,
			ChatID:    d.ChatID,
			MessageID: msgID,
			Status:    approval.StatusPending,
		})
	}

	return nil
}

func (d *Deps) reportErr(msg string) {
	if err := d.Bot.SendText("❌ " + msg); err != nil {
		log.Errorf("ccrecon: send error notice: %v", err)
	}
}

// shortAccount returns the last ":"-separated segment of a ledger account,
// e.g. "Liabilities:CreditCard:ICIC6009" -> "ICIC6009".
func shortAccount(account string) string {
	if idx := strings.LastIndex(account, ":"); idx >= 0 {
		return account[idx+1:]
	}
	return account
}

// parserFor picks the CCParser registered for account's bank, matching by
// infix: :ICIC -> icici_cc; :SELECT/:FK/:MyZone -> axis_cc; :HDFC -> hdfc_cc.
func parserFor(account string, parsers map[string]statement.CCParser) (statement.CCParser, error) {
	var name string
	switch {
	case strings.Contains(account, ":ICIC"):
		name = "icici_cc"
	case strings.Contains(account, ":SELECT"), strings.Contains(account, ":FK"), strings.Contains(account, ":MyZone"):
		name = "axis_cc"
	case strings.Contains(account, ":HDFC"):
		name = "hdfc_cc"
	default:
		return nil, fmt.Errorf("ccrecon: no parser mapping for account %q", account)
	}
	p, ok := parsers[name]
	if !ok {
		return nil, fmt.Errorf("ccrecon: parser %q not configured for account %q", name, account)
	}
	return p, nil
}

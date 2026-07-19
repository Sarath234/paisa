// internal/agent/notices/capability.go
package notices

import (
	"fmt"

	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
	log "github.com/sirupsen/logrus"
)

type TextSender interface {
	SendText(text string) error
}

// applier is the tiny slice of *billtruth.Store this capability needs —
// narrow enough that tests can inject a failure without a real store.
type applier interface {
	Apply(f billtruth.Facts) ([]string, error)
}

// Capability routes statement/payment notice SMSes into the billtruth
// store. Registered BEFORE the sms (transaction) capability so a notice is
// never mis-parsed as a spend.
type Capability struct {
	store        applier
	bot          TextSender
	cardsByLast4 map[string]string // "6009" → ledger account
}

func NewCapability(store *billtruth.Store, bot TextSender, cardsByLast4 map[string]string) *Capability {
	return NewCapabilityWithApplier(store, bot, cardsByLast4)
}

// NewCapabilityWithApplier is NewCapability generalized over the applier
// seam, for tests that need to inject a persistence failure.
func NewCapabilityWithApplier(store applier, bot TextSender, cardsByLast4 map[string]string) *Capability {
	return &Capability{store: store, bot: bot, cardsByLast4: cardsByLast4}
}

func (c *Capability) Name() string { return "notices" }

func (c *Capability) Match(text string) bool {
	return LooksLikeStatement(text) || LooksLikePayment(text)
}

func (c *Capability) HasPending(chatID int64) bool             { return false }
func (c *Capability) HandleReply(chatID int64, t string) error { return nil }

func (c *Capability) Handle(text string) error {
	if n, err := ExtractStatement(text); n != nil || err != nil {
		return c.handleStatement(n, err)
	}
	if n, err := ExtractPayment(text); n != nil || err != nil {
		return c.handlePayment(n, err)
	}
	// Match() said yes but neither extractor claimed it — treat as failure.
	return c.bot.SendText("⚠️ That looks like a card notice, but I couldn't parse it. Pattern may need updating.")
}

func (c *Capability) handleStatement(n *StatementNotice, exErr error) error {
	if exErr != nil {
		return c.bot.SendText(fmt.Sprintf("⚠️ Statement notice recognized but extraction failed: %v", exErr))
	}
	account, ok := c.cardsByLast4[n.Last4]
	if !ok {
		return c.bot.SendText(fmt.Sprintf("⚠️ Statement notice for unknown card ••%s — add it to parser_rules/credit_cards.", n.Last4))
	}
	start := n.StatementDate.AddDate(0, -1, 1)
	changed, err := c.store.Apply(billtruth.Facts{
		Account:     account,
		PeriodStart: &start,
		PeriodEnd:   &n.StatementDate,
		DueDate:     &n.DueDate,
		TotalDue:    &n.TotalDue,
		MinDue:      &n.MinDue,
		Source:      billtruth.AuthoritySMS,
	})
	if err != nil {
		log.Warnf("notices: apply statement: %v", err)
		return c.bot.SendText(fmt.Sprintf("⚠️ Noted but NOT saved to disk — will be lost on restart: %v", err))
	}
	_ = changed // reply is the same whether fields changed or repeated
	return c.bot.SendText(fmt.Sprintf("📄 Noted: %s statement %s — total due ₹%.2f, due %s",
		shortAccount(account), n.StatementDate.Format("02 Jan"), n.TotalDue, n.DueDate.Format("02 Jan")))
}

func (c *Capability) handlePayment(n *PaymentNotice, exErr error) error {
	if exErr != nil {
		return c.bot.SendText(fmt.Sprintf("⚠️ Payment notice recognized but extraction failed: %v", exErr))
	}
	account, ok := c.cardsByLast4[n.Last4]
	if !ok {
		return c.bot.SendText(fmt.Sprintf("⚠️ Payment notice for unknown card ••%s.", n.Last4))
	}
	changed, err := c.store.Apply(billtruth.Facts{
		Account:    account,
		PaidDate:   &n.Date,
		PaidAmount: &n.Amount,
		Source:     billtruth.AuthoritySMS,
	})
	if err != nil {
		log.Warnf("notices: apply payment: %v", err)
		return c.bot.SendText(fmt.Sprintf("⚠️ Noted but NOT saved to disk — will be lost on restart: %v", err))
	}
	if len(changed) == 0 {
		return c.bot.SendText(fmt.Sprintf("✅ Payment of ₹%.2f noted for %s (no open bill to attach — recorded nothing new)", n.Amount, shortAccount(account)))
	}
	return c.bot.SendText(fmt.Sprintf("✅ Payment of ₹%.2f recorded for %s — reminders stop.", n.Amount, shortAccount(account)))
}

func shortAccount(account string) string {
	for i := len(account) - 1; i >= 0; i-- {
		if account[i] == ':' {
			return account[i+1:]
		}
	}
	return account
}

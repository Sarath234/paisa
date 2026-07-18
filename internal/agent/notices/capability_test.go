package notices

import (
	"strings"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
)

type fakeSender struct{ sent []string }

func (f *fakeSender) SendText(s string) error { f.sent = append(f.sent, s); return nil }

func newCap(t *testing.T) (*Capability, *billtruth.Store, *fakeSender) {
	t.Helper()
	store, err := billtruth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bot := &fakeSender{}
	c := NewCapability(store, bot, map[string]string{
		"6009": "Liabilities:CreditCard:ICIC6009",
	})
	return c, store, bot
}

const stmtSMS = "Your ICICI Bank Credit Card XX6009 statement dated 10-Jul-26 has been sent. Total Amount Due Rs 23,450.50. Minimum Amount Due Rs 1,180.00. Due Date 30-Jul-26."

func TestCapabilityMatchesNotices(t *testing.T) {
	c, _, _ := newCap(t)
	if !c.Match(stmtSMS) {
		t.Error("statement notice must match")
	}
	if c.Match("INR 500.00 spent on ICICI Bank Card XX6009 at AMAZON on 12-Jul-26") {
		t.Error("transaction SMS must NOT match — it belongs to the sms capability")
	}
}

func TestHandleStatementAppliesFactsAndConfirms(t *testing.T) {
	c, store, bot := newCap(t)
	if err := c.Handle(stmtSMS); err != nil {
		t.Fatal(err)
	}
	bills := store.BillsFor("Liabilities:CreditCard:ICIC6009")
	if len(bills) != 1 || bills[0].TotalDue != 23450.50 {
		t.Fatalf("facts not applied: %+v", bills)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0], "ICIC6009") {
		t.Fatalf("confirmation reply: %q", bot.sent)
	}
}

func TestHandleUnknownCardReplies(t *testing.T) {
	c, store, bot := newCap(t)
	sms := strings.ReplaceAll(stmtSMS, "6009", "9999")
	if err := c.Handle(sms); err != nil {
		t.Fatal(err)
	}
	if len(store.Accounts()) != 0 {
		t.Fatal("unknown card must not create bills")
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0], "9999") {
		t.Fatalf("want unknown-card reply naming last4: %q", bot.sent)
	}
}

func TestHandleExtractionFailureReplies(t *testing.T) {
	c, _, bot := newCap(t)
	bad := strings.ReplaceAll(stmtSMS, "Due Date 30-Jul-26", "Due Date soon")
	if err := c.Handle(bad); err != nil {
		t.Fatal(err)
	}
	if len(bot.sent) != 1 || !strings.Contains(strings.ToLower(bot.sent[0]), "due date") {
		t.Fatalf("failure reply must name the field: %q", bot.sent)
	}
}

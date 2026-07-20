package notices

import (
	"errors"
	"strings"
	"testing"
	"time"

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

// TestCapabilityDoesNotMatchSpendAlertPhrasedAsPayment: a spend alert
// worded as "Payment of Rs.X made using your Credit Card" must fall through
// to the sms (transaction) capability, not be claimed here as a payment
// notice — claiming it would fake PaidDate and eat the spend.
func TestCapabilityDoesNotMatchSpendAlertPhrasedAsPayment(t *testing.T) {
	c, _, _ := newCap(t)
	sms := "Payment of Rs.1200 made using your Credit Card XX1234 at MERCHANT on 12-Jul-26."
	if c.Match(sms) {
		t.Errorf("spend-alert phrased as payment must NOT match notices — it belongs to sms: %q", sms)
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

// TestHandleStatementInfersDateWhenMissing covers a notice format with no
// explicit statement date (e.g. real Axis "is generated" notices): the
// capability must substitute Now() as PeriodEnd, apply the bill facts
// under that date, and say so in the reply rather than silently guessing.
func TestHandleStatementInfersDateWhenMissing(t *testing.T) {
	store, err := billtruth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bot := &fakeSender{}
	c := NewCapability(store, bot, map[string]string{"1610": "Liabilities:CreditCard:MyZone1610"})
	// billtruth.Apply normalizes incoming dates to LOCAL midnight (see
	// billtruth/apply.go), so fixedNow must already be local to compare
	// equal after the round-trip through the store — unlike day() (UTC),
	// used elsewhere in this package for pure-extractor date comparisons.
	fixedNow := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
	c.Now = func() time.Time { return fixedNow }

	sms := "Your statement for Axis Bank Credit Card no. XX1610 is generated.\n" +
		"Due on: 07-08-26\n" +
		"Total amt: INR  Dr. 24,567.89\n" +
		"Min amt due: INR  Dr. 1,230.00"
	if err := c.Handle(sms); err != nil {
		t.Fatal(err)
	}
	bills := store.BillsFor("Liabilities:CreditCard:MyZone1610")
	if len(bills) != 1 {
		t.Fatalf("bills: %+v", bills)
	}
	if !bills[0].PeriodEnd.Equal(fixedNow) {
		t.Errorf("PeriodEnd should default to Now(): got %v want %v", bills[0].PeriodEnd, fixedNow)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0], "inferred") {
		t.Errorf("reply should mention the date was inferred: %q", bot.sent)
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

// failingApplier always fails Apply, simulating a persistence failure (e.g.
// disk full, read-only filesystem) without needing a real broken store.
type failingApplier struct{ err error }

func (f *failingApplier) Apply(billtruth.Facts) ([]string, error) { return nil, f.err }

// TestHandleStatementReplyWarnsOnPersistenceFailure: when store.Apply fails,
// the reply must warn that the fact was NOT saved — replying with the
// normal "📄 Noted" success text would silently lose the statement facts on
// restart, since the caller has no other signal the write failed.
func TestHandleStatementReplyWarnsOnPersistenceFailure(t *testing.T) {
	bot := &fakeSender{}
	applier := &failingApplier{err: errors.New("disk full")}
	c := NewCapabilityWithApplier(applier, bot, map[string]string{
		"6009": "Liabilities:CreditCard:ICIC6009",
	})
	if err := c.Handle(stmtSMS); err != nil {
		t.Fatal(err)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0], "NOT saved") {
		t.Fatalf("want a NOT-saved warning reply, got %q", bot.sent)
	}
	if strings.Contains(bot.sent[0], "📄 Noted") {
		t.Fatalf("must not send the success text on persistence failure: %q", bot.sent[0])
	}
}

// TestHandlePaymentReplyWarnsOnPersistenceFailure mirrors the statement
// case for handlePayment.
func TestHandlePaymentReplyWarnsOnPersistenceFailure(t *testing.T) {
	bot := &fakeSender{}
	applier := &failingApplier{err: errors.New("disk full")}
	c := NewCapabilityWithApplier(applier, bot, map[string]string{
		"6009": "Liabilities:CreditCard:ICIC6009",
	})
	sms := "Payment of Rs 23,450.50 received on your ICICI Bank Credit Card XX6009 on 25-Jul-26. Thank you."
	if err := c.Handle(sms); err != nil {
		t.Fatal(err)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0], "NOT saved") {
		t.Fatalf("want a NOT-saved warning reply, got %q", bot.sent)
	}
	if strings.Contains(bot.sent[0], "✅") {
		t.Fatalf("must not send the success text on persistence failure: %q", bot.sent[0])
	}
}

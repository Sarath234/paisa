package monitor

import (
	"errors"
	"strings"
	"testing"
)

type fakeSender struct {
	sent []string
	err  error
}

func (f *fakeSender) SendText(text string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, text)
	return nil
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDeliverImmediate(t *testing.T) {
	bot := &fakeSender{}
	store := newTestStore(t)
	n := NewNotifier(bot, store)

	in := Insight{Key: "k1", Urgency: Immediate, Title: "💳 due", Body: "pay up"}
	n.Deliver("cc_due", []Insight{in})
	if len(bot.sent) != 1 || bot.sent[0] != "💳 due\npay up" {
		t.Fatalf("sent: %q", bot.sent)
	}
	// second delivery of same key is a no-op
	n.Deliver("cc_due", []Insight{in})
	if len(bot.sent) != 1 {
		t.Fatalf("dedupe failed: %q", bot.sent)
	}
}

func TestDeliverImmediateSendFailureRetries(t *testing.T) {
	bot := &fakeSender{err: errors.New("telegram down")}
	store := newTestStore(t)
	n := NewNotifier(bot, store)

	in := Insight{Key: "k1", Urgency: Immediate, Title: "T"}
	n.Deliver("cc_due", []Insight{in})
	if store.WasSent("k1") {
		t.Fatal("failed send must not mark key sent")
	}
	bot.err = nil
	n.Deliver("cc_due", []Insight{in})
	if len(bot.sent) != 1 || !store.WasSent("k1") {
		t.Fatalf("retry failed: sent=%q", bot.sent)
	}
}

func TestDeliverDigestQueuesAndFlushes(t *testing.T) {
	bot := &fakeSender{}
	store := newTestStore(t)
	n := NewNotifier(bot, store)

	n.Deliver("cc_utilization", []Insight{{Key: "u1", Urgency: Digest, Title: "⚠️ Axis at 78%"}})
	n.Deliver("cc_statement", []Insight{{Key: "s1", Urgency: Digest, Title: "📄 Statement", Body: "₹31200.00"}})
	if len(bot.sent) != 0 {
		t.Fatalf("digest must not send immediately: %q", bot.sent)
	}
	if !store.WasSent("u1") {
		t.Fatal("digest insight should be marked sent at enqueue")
	}

	if err := n.FlushDigest(); err != nil {
		t.Fatal(err)
	}
	if len(bot.sent) != 1 {
		t.Fatalf("flush: %q", bot.sent)
	}
	msg := bot.sent[0]
	for _, want := range []string{"Daily digest", "2 insights", "cc_utilization", "⚠️ Axis at 78%", "📄 Statement"} {
		if !strings.Contains(msg, want) {
			t.Errorf("digest missing %q:\n%s", want, msg)
		}
	}
	if len(store.DigestQueue()) != 0 {
		t.Error("queue should drain after flush")
	}
	// empty flush: no message
	if err := n.FlushDigest(); err != nil {
		t.Fatal(err)
	}
	if len(bot.sent) != 1 {
		t.Error("empty flush must not send")
	}
}

func TestFlushDigestFailureKeepsQueue(t *testing.T) {
	bot := &fakeSender{}
	store := newTestStore(t)
	n := NewNotifier(bot, store)
	n.Deliver("cc_statement", []Insight{{Key: "s1", Urgency: Digest, Title: "T"}})
	bot.err = errors.New("telegram down")
	if err := n.FlushDigest(); err == nil {
		t.Fatal("want error")
	}
	if len(store.DigestQueue()) != 1 {
		t.Fatal("queue must be retained on failure")
	}
}

func TestFlushDigestGroupsNonContiguousMonitors(t *testing.T) {
	bot := &fakeSender{}
	store := newTestStore(t)
	n := NewNotifier(bot, store)

	// Deliver in order: monitor "a" (key a1), monitor "b" (key b1), monitor "a" again (key a2)
	n.Deliver("a", []Insight{{Key: "a1", Urgency: Digest, Title: "Alert A1"}})
	n.Deliver("b", []Insight{{Key: "b1", Urgency: Digest, Title: "Alert B1"}})
	n.Deliver("a", []Insight{{Key: "a2", Urgency: Digest, Title: "Alert A2"}})

	if err := n.FlushDigest(); err != nil {
		t.Fatal(err)
	}

	if len(bot.sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(bot.sent))
	}

	msg := bot.sent[0]

	// Assert exactly one "a:" section header
	aCount := strings.Count(msg, "\na:\n")
	if aCount != 1 {
		t.Errorf("expected 1 'a:' section, got %d:\n%s", aCount, msg)
	}

	// Assert total says "3 insights"
	if !strings.Contains(msg, "3 insights") {
		t.Errorf("expected '3 insights' in message:\n%s", msg)
	}

	// Assert both a-titles appear
	if !strings.Contains(msg, "Alert A1") || !strings.Contains(msg, "Alert A2") {
		t.Errorf("both a-titles should appear:\n%s", msg)
	}

	// Assert b-title appears
	if !strings.Contains(msg, "Alert B1") {
		t.Errorf("b-title should appear:\n%s", msg)
	}

	// Assert queue is drained
	if len(store.DigestQueue()) != 0 {
		t.Error("queue should drain after flush")
	}
}

func TestFlushDigestSingularHeader(t *testing.T) {
	bot := &fakeSender{}
	store := newTestStore(t)
	n := NewNotifier(bot, store)

	n.Deliver("cc_statement", []Insight{{Key: "k1", Urgency: Digest, Title: "📄 Statement"}})
	if err := n.FlushDigest(); err != nil {
		t.Fatal(err)
	}
	if len(bot.sent) != 1 {
		t.Fatalf("sent: %q", bot.sent)
	}
	if !strings.Contains(bot.sent[0], "1 insight\n") || strings.Contains(bot.sent[0], "1 insights") {
		t.Errorf("digest header must pluralize correctly, got:\n%s", bot.sent[0])
	}
}

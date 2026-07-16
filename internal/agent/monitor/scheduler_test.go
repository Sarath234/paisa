package monitor

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeMonitor struct {
	name     string
	insights []Insight
	err      error
	panics   bool
	calls    int
}

func (m *fakeMonitor) Name() string { return m.name }
func (m *fakeMonitor) Due(now, lastRun time.Time) bool {
	return DailyAt(8)(now, lastRun)
}
func (m *fakeMonitor) Check(ctx context.Context) ([]Insight, error) {
	m.calls++
	if m.panics {
		panic("boom")
	}
	return m.insights, m.err
}

func newSched(t *testing.T, mons ...Monitor) (*Scheduler, *fakeSender, *Store) {
	t.Helper()
	store := newTestStore(t)
	bot := &fakeSender{}
	s := NewScheduler(mons, NewNotifier(bot, store), store, 8)
	return s, bot, store
}

func TestRunOnceRunsDueMonitorsAndAdvancesLastRun(t *testing.T) {
	m := &fakeMonitor{name: "m1", insights: []Insight{{Key: "k", Urgency: Immediate, Title: "T"}}}
	s, bot, store := newSched(t, m)
	s.Now = func() time.Time { return at("08:15") }

	s.RunOnce()
	if m.calls != 1 || len(bot.sent) != 1 {
		t.Fatalf("calls=%d sent=%q", m.calls, bot.sent)
	}
	if !store.LastRun("m1").Equal(at("08:15")) {
		t.Fatalf("lastRun: %v", store.LastRun("m1"))
	}
	// same tick again: not due
	s.RunOnce()
	if m.calls != 1 {
		t.Fatal("monitor ran twice in one day")
	}
}

func TestRunOnceErrorDoesNotAdvanceLastRun(t *testing.T) {
	m := &fakeMonitor{name: "m1", err: errors.New("api down")}
	s, _, store := newSched(t, m)
	s.Now = func() time.Time { return at("08:15") }
	s.RunOnce()
	if !store.LastRun("m1").IsZero() {
		t.Fatal("failed run must not advance lastRun")
	}
	s.RunOnce() // retried
	if m.calls != 2 {
		t.Fatalf("calls=%d, want retry", m.calls)
	}
}

func TestRunOncePanicIsIsolated(t *testing.T) {
	bad := &fakeMonitor{name: "bad", panics: true}
	good := &fakeMonitor{name: "good", insights: []Insight{{Key: "g", Urgency: Immediate, Title: "ok"}}}
	s, bot, _ := newSched(t, bad, good)
	s.Now = func() time.Time { return at("08:15") }
	s.RunOnce() // must not panic
	if len(bot.sent) != 1 {
		t.Fatalf("good monitor blocked by panicking sibling: %q", bot.sent)
	}
}

func TestDigestFlushesOncePerDay(t *testing.T) {
	m := &fakeMonitor{name: "m1", insights: []Insight{{Key: "d1", Urgency: Digest, Title: "D"}}}
	s, bot, _ := newSched(t, m)

	s.Now = func() time.Time { return at("07:00") }
	s.RunOnce() // before digest hour: monitor not due (DailyAt 8), no flush
	if len(bot.sent) != 0 {
		t.Fatalf("nothing should send at 07:00: %q", bot.sent)
	}

	s.Now = func() time.Time { return at("08:15") }
	s.RunOnce() // monitor runs, queues digest; flush happens same pass
	if len(bot.sent) != 1 {
		t.Fatalf("digest should flush at 08:15: %q", bot.sent)
	}

	s.Now = func() time.Time { return at("08:30") }
	s.RunOnce() // no double flush
	if len(bot.sent) != 1 {
		t.Fatalf("digest flushed twice: %q", bot.sent)
	}
}

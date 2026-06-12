// internal/agent/router/router_test.go
package router

import (
	"errors"
	"testing"
)

// fakeCap records which method handled the message.
type fakeCap struct {
	name    string
	match   bool
	pending bool
	handled []string
	replies []string
}

func (f *fakeCap) Name() string                  { return f.name }
func (f *fakeCap) Match(text string) bool        { return f.match }
func (f *fakeCap) HasPending(chatID int64) bool  { return f.pending }
func (f *fakeCap) Handle(text string) error      { f.handled = append(f.handled, text); return nil }
func (f *fakeCap) HandleReply(text string) error { f.replies = append(f.replies, text); return nil }

func TestRoutePendingWinsOverMatch(t *testing.T) {
	sms := &fakeCap{name: "sms_ingest", match: true, pending: true}
	r := New([]Capability{sms}, nil, func(string) {})
	r.Route(1, "edited entry")
	if len(sms.replies) != 1 || len(sms.handled) != 0 {
		t.Errorf("pending conversation must route to HandleReply: %+v", sms)
	}
}

func TestRouteDeterministicMatch(t *testing.T) {
	sms := &fakeCap{name: "sms_ingest", match: true}
	qa := &fakeCap{name: "finance_qa"}
	classifierCalled := false
	classify := func(string) (string, error) { classifierCalled = true; return "finance_qa", nil }
	r := New([]Capability{sms, qa}, classify, func(string) {})
	r.Route(1, "HDFC debit SMS")
	if len(sms.handled) != 1 {
		t.Error("matching capability must handle")
	}
	if classifierCalled {
		t.Error("fast path must not invoke the LLM classifier")
	}
}

func TestRouteLLMFallback(t *testing.T) {
	sms := &fakeCap{name: "sms_ingest"}
	qa := &fakeCap{name: "finance_qa"}
	classify := func(string) (string, error) { return "finance_qa", nil }
	r := New([]Capability{sms, qa}, classify, func(string) {})
	r.Route(1, "how much did I spend?")
	if len(qa.handled) != 1 || len(sms.handled) != 0 {
		t.Errorf("LLM intent must route to finance_qa: qa=%+v sms=%+v", qa, sms)
	}
}

func TestRouteUnknownIntentFallback(t *testing.T) {
	qa := &fakeCap{name: "finance_qa"}
	var fellBack string
	r := New([]Capability{qa}, func(string) (string, error) { return "unknown", nil },
		func(text string) { fellBack = text })
	r.Route(1, "hello there")
	if fellBack != "hello there" || len(qa.handled) != 0 {
		t.Errorf("unknown intent must hit fallback: fellBack=%q", fellBack)
	}
}

func TestRouteClassifierErrorFallback(t *testing.T) {
	qa := &fakeCap{name: "finance_qa"}
	var fellBack string
	r := New([]Capability{qa}, func(string) (string, error) { return "", errors.New("ollama down") },
		func(text string) { fellBack = text })
	r.Route(1, "anything")
	if fellBack != "anything" {
		t.Error("classifier error must hit fallback")
	}
}

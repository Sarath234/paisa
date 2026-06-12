// internal/agent/qa/capability_test.go
package qa

import (
	"strings"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

type fakeSender struct {
	sent []string
}

func (f *fakeSender) SendText(text string) error {
	f.sent = append(f.sent, text)
	return nil
}

func TestCapabilityMatch(t *testing.T) {
	c := &Capability{}
	if !c.Match("/q food spend") {
		t.Error("/q prefix must match deterministically")
	}
	if c.Match("how much did I spend?") {
		t.Error("natural language must NOT match deterministically (LLM stage decides)")
	}
}

func TestCapabilityName(t *testing.T) {
	if (&Capability{}).Name() != "finance_qa" {
		t.Error("Name must be finance_qa (router intent label)")
	}
}

func TestCapabilityNoPending(t *testing.T) {
	c := &Capability{}
	if c.HasPending(42) {
		t.Error("QA has no multi-turn state in v1")
	}
	if err := c.HandleReply(42, "x"); err != nil {
		t.Errorf("HandleReply should be a no-op: %v", err)
	}
}

func TestCapabilityHandleSuccess(t *testing.T) {
	paisa := paisaStub(t)
	defer paisa.Close()
	ollama := ollamaStub(t, `{\"intent\": \"networth\"}`)
	defer ollama.Close()

	bot := &fakeSender{}
	c := &Capability{
		Bot:    bot,
		Ollama: config.OllamaConfig{URL: ollama.URL},
		Answerer: &Answerer{
			Client: paisaclient.New(paisa.URL),
			Now:    func() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) },
		},
	}
	if err := c.Handle("what is my net worth?"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0], "Net worth") {
		t.Errorf("sent = %v", bot.sent)
	}
}

func TestCapabilityHandleExtractFailure(t *testing.T) {
	ollama := ollamaStub(t, `no json`)
	defer ollama.Close()
	bot := &fakeSender{}
	c := &Capability{Bot: bot, Ollama: config.OllamaConfig{URL: ollama.URL}}
	if err := c.Handle("gibberish"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0], "I can answer") {
		t.Errorf("want help text, got %v", bot.sent)
	}
}

func TestCapabilityHandlePaisaDown(t *testing.T) {
	ollama := ollamaStub(t, `{\"intent\": \"networth\"}`)
	defer ollama.Close()
	bot := &fakeSender{}
	c := &Capability{
		Bot:      bot,
		Ollama:   config.OllamaConfig{URL: ollama.URL},
		Answerer: &Answerer{Client: paisaclient.New("http://127.0.0.1:1"), Now: time.Now},
	}
	if err := c.Handle("net worth?"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0], "unreachable") {
		t.Errorf("want unreachable message, got %v", bot.sent)
	}
}

func TestCapabilityHandleStripsPrefix(t *testing.T) {
	paisa := paisaStub(t)
	defer paisa.Close()
	ollama := ollamaStub(t, `{\"intent\": \"networth\"}`)
	defer ollama.Close()
	bot := &fakeSender{}
	c := &Capability{
		Bot:    bot,
		Ollama: config.OllamaConfig{URL: ollama.URL},
		Answerer: &Answerer{
			Client: paisaclient.New(paisa.URL),
			Now:    func() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) },
		},
	}
	if err := c.Handle("/q what is my net worth?"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(bot.sent) != 1 || !strings.Contains(bot.sent[0], "Net worth") {
		t.Errorf("sent = %v", bot.sent)
	}
}

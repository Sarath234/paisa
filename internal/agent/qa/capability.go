// internal/agent/qa/capability.go
package qa

import (
	"strings"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	log "github.com/sirupsen/logrus"
)

// Sender is the slice of telegram.Bot the QA capability needs.
type Sender interface {
	SendText(text string) error
}

// HelpText lists what the bot can answer; sent on extraction failure or
// unknown intent.
const HelpText = `🤔 I can answer questions like:
- "how much did I spend on food this month?"
- "what's my net worth?"
- "balance in my Axis account?"
- "am I over budget?"

Or forward a bank SMS to record a transaction.`

// Capability answers finance questions. Router plug-in.
type Capability struct {
	Bot      Sender
	Ollama   config.OllamaConfig
	Answerer *Answerer
}

func (c *Capability) Name() string { return "finance_qa" }

// Match claims only the explicit /q prefix. Natural-language questions reach
// this capability via the router's LLM stage instead.
func (c *Capability) Match(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "/q ")
}

func (c *Capability) HasPending(chatID int64) bool { return false }

func (c *Capability) HandleReply(chatID int64, text string) error { return nil }

func (c *Capability) Handle(text string) error {
	text = strings.TrimPrefix(strings.TrimSpace(text), "/q ")

	q, err := Extract(text, c.Ollama)
	if err != nil {
		log.Warnf("qa: extract failed: %v", err)
		return c.Bot.SendText(HelpText)
	}

	answer, err := c.Answerer.Answer(q)
	if err != nil {
		log.Errorf("qa: answer failed: %v", err)
		return c.Bot.SendText("⚠️ Paisa server unreachable — is paisa running?\n(" + err.Error() + ")")
	}
	return c.Bot.SendText(answer)
}

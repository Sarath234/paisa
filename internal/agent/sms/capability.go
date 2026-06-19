// internal/agent/sms/capability.go
// Package sms wraps the existing SMS→ledger ingest pipeline as a router
// capability. The parse/draft/approval behavior is unchanged from the
// pre-router flow.
package sms

import (
	"fmt"

	"github.com/ananthakumaran/paisa/internal/agent/approval"
	"github.com/ananthakumaran/paisa/internal/agent/config"
	agentledger "github.com/ananthakumaran/paisa/internal/agent/ledger"
	"github.com/ananthakumaran/paisa/internal/agent/llm"
	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/ananthakumaran/paisa/internal/agent/telegram"
	log "github.com/sirupsen/logrus"
)

// Bot is the slice of telegram.Bot the SMS capability needs.
type Bot interface {
	SendText(text string) error
	SendDraft(text string) (int, error)
	SendDraftDuplicate(text string) (int, error)
}

type Capability struct {
	Bot   Bot
	Store *approval.Store
	Cfg   *config.Config
}

func (c *Capability) Name() string { return "sms_ingest" }

// Match claims messages that match a configured account rule — the fast path.
// parser.Classify is pure string matching: microseconds, no network.
func (c *Capability) Match(text string) bool {
	_, err := parser.Classify(text, c.Cfg.ParserRules.Accounts)
	return err == nil
}

func (c *Capability) HasPending(chatID int64) bool {
	return c.Store.GetEditingByChatID(chatID) != nil
}

// HandleReply merges an edit reply into the pending entry and re-sends the draft.
// Preserves Original from the current pending across the edit round.
func (c *Capability) HandleReply(chatID int64, text string) error {
	pending := c.Store.GetEditingByChatID(chatID)
	if pending == nil {
		return nil
	}
	log.Debugf("sms: routing as edit reply for msgID=%d", pending.MessageID)
	original := pending.Original // capture before delete
	updated := telegram.ParseEditReply(text, pending.Entry)
	c.Store.Delete(pending.MessageID)
	c.sendDraft(updated, original)
	return nil
}

// Handle treats the message as a new bank SMS: parse → LLM fill → draft.
// On first parse, original == entry.
func (c *Capability) Handle(text string) error {
	preview := text
	if len(preview) > 80 {
		preview = preview[:80] + "…"
	}
	log.Infof("sms: new SMS received (len=%d): %q", len(text), preview)

	entry, err := c.parseAndFill(text)
	if err != nil {
		log.Errorf("parse: %v", err)
		return c.Bot.SendText(fmt.Sprintf("❌ Could not parse: %v", err))
	}
	log.Infof("parse: success — date=%q desc=%q amt=%q src=%q dest=%q",
		entry.Date, entry.Desc, entry.Amt, entry.Src, entry.Dest)
	c.sendDraft(*entry, *entry) // original == entry on first parse
	return nil
}

// ParseAndFill is the exported parse+LLM-fill entry point used by the HTTP server.
func (c *Capability) ParseAndFill(sms string) (*agentledger.Entry, error) {
	return c.parseAndFill(sms)
}

func (c *Capability) parseAndFill(sms string) (*agentledger.Entry, error) {
	rule, err := parser.Classify(sms, c.Cfg.ParserRules.Accounts)
	if err != nil {
		return nil, err
	}
	log.Debugf("parse: classified as bank=%q destinations=%q", rule.Bank, rule.Destinations)

	entry, err := parser.Parse(sms, rule, c.Cfg.ParserRules.Merchants)
	if err != nil {
		return nil, err
	}

	needsLLM := entry.Dest == "" || entry.Desc == ""
	if needsLLM {
		log.Infof("parse: regex incomplete — desc=%q dest=%q — invoking LLM", entry.Desc, entry.Dest)
		if llmErr := llm.FillMissing(sms, entry, c.Cfg.Ollama); llmErr != nil {
			log.Warnf("llm fallback: %v", llmErr)
		}
	}
	return entry, nil
}

// sendDraft formats and sends a draft, storing it with the original (pre-edit) entry.
func (c *Capability) sendDraft(entry agentledger.Entry, original agentledger.Entry) {
	draftText := telegram.FormatDraft(entry)

	dup, err := agentledger.IsDuplicate(c.Cfg.Paisa.JournalDir, &entry)
	if err != nil {
		log.Warnf("duplicate check: %v", err)
		// treat as non-duplicate on error — never block the ingest flow
	}

	var msgID int
	if dup {
		log.Infof("draft: sending as duplicate (date=%q amt=%q)", entry.Date, entry.Amt)
		msgID, err = c.Bot.SendDraftDuplicate(draftText)
	} else {
		msgID, err = c.Bot.SendDraft(draftText)
	}
	if err != nil {
		log.Errorf("send draft: %v", err)
		return
	}
	log.Debugf("draft: sent msgID=%d", msgID)

	c.Store.Set(&approval.Pending{
		Entry:     entry,
		Original:  original,
		ChatID:    c.Cfg.Telegram.ChatID,
		MessageID: msgID,
		Status:    approval.StatusPending,
	})
}

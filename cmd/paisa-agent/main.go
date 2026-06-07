// cmd/paisa-agent/main.go
package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/ananthakumaran/paisa/internal/agent/approval"
	"github.com/ananthakumaran/paisa/internal/agent/config"
	agentledger "github.com/ananthakumaran/paisa/internal/agent/ledger"
	"github.com/ananthakumaran/paisa/internal/agent/llm"
	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/ananthakumaran/paisa/internal/agent/telegram"
	log "github.com/sirupsen/logrus"
)

func main() {
	cfgPath := flag.String("config", "paisa-agent.yaml", "path to paisa-agent.yaml")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := agentledger.EnsureFile(cfg.Paisa.JournalDir); err != nil {
		log.Warnf("ensure auto-import.ledger: %v", err)
	}

	bot := telegram.NewBot(cfg.Telegram.BotToken, cfg.Telegram.ChatID)
	store := approval.NewStore()

	log.Infof("paisa-agent started — polling Telegram (chat_id=%d)", cfg.Telegram.ChatID)

	for {
		updates, err := bot.Poll()
		if err != nil {
			log.Errorf("poll error: %v", err)
			continue
		}
		for _, u := range updates {
			switch {
			case u.CallbackQuery != nil:
				handleCallback(u.CallbackQuery, bot, store, cfg)
			case u.Message != nil:
				handleMessage(u.Message, bot, store, cfg)
			}
		}
	}
}

func handleMessage(msg *telegram.Message, bot *telegram.Bot, store *approval.Store, cfg *config.Config) {
	// If this chat has an entry in editing state, route as an edit reply.
	if pending := store.GetEditingByChatID(msg.Chat.ID); pending != nil {
		updated := telegram.ParseEditReply(msg.Text, pending.Entry)
		store.Delete(pending.MessageID)
		sendDraft(updated, bot, store, cfg)
		return
	}

	// Otherwise treat the message text as a new bank SMS.
	entry, err := parseAndFill(msg.Text, cfg)
	if err != nil {
		log.Errorf("parse: %v", err)
		bot.SendText(fmt.Sprintf("❌ Could not parse: %v", err))
		return
	}
	sendDraft(*entry, bot, store, cfg)
}

func parseAndFill(sms string, cfg *config.Config) (*agentledger.Entry, error) {
	rule, err := parser.Classify(sms, cfg.ParserRules.Accounts)
	if err != nil {
		return nil, err
	}
	entry, err := parser.Parse(sms, rule, cfg.ParserRules.Merchants)
	if err != nil {
		return nil, err
	}
	if entry.Dest == "" || entry.Desc == "" {
		if llmErr := llm.FillMissing(sms, entry, cfg.Ollama); llmErr != nil {
			log.Warnf("llm fallback: %v", llmErr)
		}
	}
	return entry, nil
}

func sendDraft(entry agentledger.Entry, bot *telegram.Bot, store *approval.Store, cfg *config.Config) {
	draftText := telegram.FormatDraft(entry)

	dup, err := agentledger.IsDuplicate(cfg.Paisa.JournalDir, &entry)
	if err != nil {
		log.Warnf("duplicate check: %v", err)
	}

	var msgID int
	if dup {
		msgID, err = bot.SendDraftDuplicate(draftText)
	} else {
		msgID, err = bot.SendDraft(draftText)
	}
	if err != nil {
		log.Errorf("send draft: %v", err)
		return
	}

	store.Set(&approval.Pending{
		Entry:     entry,
		ChatID:    cfg.Telegram.ChatID,
		MessageID: msgID,
		Status:    approval.StatusPending,
	})
}

func handleCallback(cb *telegram.CallbackQuery, bot *telegram.Bot, store *approval.Store, cfg *config.Config) {
	bot.AnswerCallback(cb.ID)

	if cb.Message == nil {
		return
	}
	msgID := cb.Message.MessageID
	pending := store.Get(msgID)
	if pending == nil {
		log.Debugf("callback for unknown messageID %d (agent may have restarted)", msgID)
		return
	}

	switch strings.ToLower(cb.Data) {
	case "approve":
		if err := agentledger.Append(cfg.Paisa.JournalDir, &pending.Entry); err != nil {
			log.Errorf("append entry: %v", err)
			bot.EditMessage(msgID, "❌ Failed to post: "+err.Error())
			return
		}
		bot.EditMessage(msgID, "✅ Posted\n\n"+telegram.FormatDraft(pending.Entry))
		store.Delete(msgID)
		log.Infof("posted: %s %s %s", pending.Entry.Date, pending.Entry.Desc, pending.Entry.Amt)

	case "edit":
		store.SetEditing(msgID)
		bot.SendText(telegram.FormatEditTemplate(pending.Entry))

	case "skip":
		bot.EditMessage(msgID, "⏭ Skipped\n\n"+telegram.FormatDraft(pending.Entry))
		store.Delete(msgID)
	}
}

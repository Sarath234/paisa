// cmd/paisa-agent/main.go
package main

import (
	"flag"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/approval"
	"github.com/ananthakumaran/paisa/internal/agent/config"
	agentledger "github.com/ananthakumaran/paisa/internal/agent/ledger"
	"github.com/ananthakumaran/paisa/internal/agent/llm"
	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
	"github.com/ananthakumaran/paisa/internal/agent/qa"
	"github.com/ananthakumaran/paisa/internal/agent/router"
	"github.com/ananthakumaran/paisa/internal/agent/sms"
	"github.com/ananthakumaran/paisa/internal/agent/telegram"
	log "github.com/sirupsen/logrus"
)

// Intent names must match the Name() return of each Capability registered
// with the router: sms.Capability → "sms_ingest", qa.Capability → "finance_qa".
var intents = []llm.Intent{
	{Name: "sms_ingest", Description: "a bank transaction SMS or alert (debit, credit, UPI, account balance notification)"},
	{Name: "finance_qa", Description: "a question about the user's own finances (spending, net worth, balances, budget)"},
}

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

	smsCap := &sms.Capability{Bot: bot, Store: store, Cfg: cfg}
	qaCap := &qa.Capability{
		Bot:    bot,
		Ollama: cfg.Ollama,
		Answerer: &qa.Answerer{
			Client: paisaclient.New(cfg.Paisa.URL),
			Now:    time.Now,
		},
	}

	rt := router.New(
		[]router.Capability{smsCap, qaCap},
		func(text string) (string, error) {
			return llm.ClassifyIntent(text, intents, cfg.Ollama)
		},
		func(text string) {
			log.Infof("router: no capability claimed message — sending help")
			if err := bot.SendText(qa.HelpText); err != nil {
				log.Warnf("router: fallback send failed: %v", err)
			}
		},
	)

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
				if u.Message.Chat.ID != cfg.Telegram.ChatID {
					continue
				}
				rt.Route(u.Message.Chat.ID, u.Message.Text)
			}
		}
	}
}

func handleCallback(cb *telegram.CallbackQuery, bot *telegram.Bot, store *approval.Store, cfg *config.Config) {
	bot.AnswerCallback(cb.ID)

	if cb.Message == nil {
		return
	}
	if cb.Message.Chat.ID != cfg.Telegram.ChatID {
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

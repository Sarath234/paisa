// cmd/paisa-agent/main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
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

	go serveHTTP(cfg)

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
	if msg.Chat.ID != cfg.Telegram.ChatID {
		return
	}

	// If this chat has an entry in editing state, route as an edit reply.
	if pending := store.GetEditingByChatID(msg.Chat.ID); pending != nil {
		log.Debugf("message: routing as edit reply for msgID=%d", pending.MessageID)
		updated := telegram.ParseEditReply(msg.Text, pending.Entry)
		store.Delete(pending.MessageID)
		sendDraft(updated, bot, store, cfg)
		return
	}

	preview := msg.Text
	if len(preview) > 80 {
		preview = preview[:80] + "…"
	}
	log.Infof("message: new SMS received (len=%d): %q", len(msg.Text), preview)

	// Otherwise treat the message text as a new bank SMS.
	entry, err := parseAndFill(msg.Text, cfg)
	if err != nil {
		log.Errorf("parse: %v", err)
		bot.SendText(fmt.Sprintf("❌ Could not parse: %v", err))
		return
	}
	log.Infof("parse: success — date=%q desc=%q amt=%q src=%q dest=%q",
		entry.Date, entry.Desc, entry.Amt, entry.Src, entry.Dest)
	sendDraft(*entry, bot, store, cfg)
}

func parseAndFill(sms string, cfg *config.Config) (*agentledger.Entry, error) {
	rule, err := parser.Classify(sms, cfg.ParserRules.Accounts)
	if err != nil {
		return nil, err
	}
	log.Debugf("parse: classified as bank=%q destinations=%q", rule.Bank, rule.Destinations)

	entry, err := parser.Parse(sms, rule, cfg.ParserRules.Merchants)
	if err != nil {
		return nil, err
	}

	needsLLM := entry.Dest == "" || entry.Desc == ""
	if needsLLM {
		log.Infof("parse: regex incomplete — desc=%q dest=%q — invoking LLM", entry.Desc, entry.Dest)
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
		log.Infof("draft: sending as duplicate (date=%q amt=%q)", entry.Date, entry.Amt)
		msgID, err = bot.SendDraftDuplicate(draftText)
	} else {
		msgID, err = bot.SendDraft(draftText)
	}
	if err != nil {
		log.Errorf("send draft: %v", err)
		return
	}
	log.Debugf("draft: sent msgID=%d", msgID)

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

func serveHTTP(cfg *config.Config) {
	mux := http.NewServeMux()

	mux.HandleFunc("/parse", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		entry, err := parseAndFill(req.Text, cfg)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entry)
	})

	mux.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Entry agentledger.Entry `json:"entry"`
			Force bool              `json:"force"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		entry := req.Entry
		if !req.Force {
			dup, err := agentledger.IsDuplicate(cfg.Paisa.JournalDir, &entry)
			if err != nil {
				log.Warnf("http /post: duplicate check: %v", err)
			}
			if dup {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "duplicate"})
				return
			}
		}
		if err := agentledger.Append(cfg.Paisa.JournalDir, &entry); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		log.Infof("http /post: posted %s %s %s", entry.Date, entry.Desc, entry.Amt)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "posted"})
	})

	addr := "127.0.0.1:7501"
	log.Infof("paisa-agent HTTP listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("http server: %v", err)
	}
}

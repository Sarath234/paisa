// cmd/paisa-agent/main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/approval"
	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/gmail"
	agentledger "github.com/ananthakumaran/paisa/internal/agent/ledger"
	"github.com/ananthakumaran/paisa/internal/agent/llm"
	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
	"github.com/ananthakumaran/paisa/internal/agent/qa"
	"github.com/ananthakumaran/paisa/internal/agent/reconcile"
	"github.com/ananthakumaran/paisa/internal/agent/router"
	"github.com/ananthakumaran/paisa/internal/agent/rulelearning"
	"github.com/ananthakumaran/paisa/internal/agent/sms"
	"github.com/ananthakumaran/paisa/internal/agent/statement"
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

	var gmailClient *gmail.Client
	var gmailPoller *gmail.Poller

	if cfg.Gmail != nil {
		var err error
		gmailClient, err = gmail.New(cfg.Gmail.ClientID, cfg.Gmail.ClientSecret, cfg.Gmail.TokenFile)
		if err != nil {
			log.Fatalf("gmail: init: %v", err)
		}

		parsers := []statement.Parser{&statement.AxisParser{}}
		pc := paisaclient.New(cfg.Paisa.URL)
		stateDir := filepath.Dir(cfg.Gmail.TokenFile)

		var subjectMatches []gmail.SubjectMatch
		for _, sa := range cfg.Gmail.Accounts {
			subjectMatches = append(subjectMatches, gmail.SubjectMatch{
				Pattern:       sa.SubjectMatch,
				LedgerAccount: sa.LedgerAccount,
			})
		}

		gmailPoller = gmail.NewPoller(gmailClient, subjectMatches, stateDir, func(ev gmail.StatementEmail) {
			handleStatementEmail(ev, parsers, pc, cfg.Paisa.JournalDir, bot)
		})

		if !gmailClient.IsAuthorized() {
			authURL := gmailClient.AuthURL()
			msg := fmt.Sprintf("Gmail auth required. Opening browser for OAuth...\nIf browser doesn't open, visit:\n%s", authURL)
			bot.SendText(msg) //nolint:errcheck
			log.Infof("gmail: auth required — starting local callback server on :8787")
			go func() {
				code, err := gmail.OAuthCallbackServer()
				if err != nil {
					log.Errorf("gmail: oauth callback: %v", err)
					bot.SendText("❌ Gmail OAuth timed out. Restart the agent to try again.") //nolint:errcheck
					return
				}
				if err := gmailClient.ExchangeCode(code); err != nil {
					log.Errorf("gmail: exchange code: %v", err)
					bot.SendText(fmt.Sprintf("❌ Gmail auth failed: %v", err)) //nolint:errcheck
					return
				}
				bot.SendText("✅ Gmail connected — will poll for statements every 5 minutes") //nolint:errcheck
				log.Infof("gmail: authorised")
				go gmailPoller.Start()
			}()
		} else {
			go gmailPoller.Start()
		}
	}

	store := approval.NewStore()
	ruleStore := rulelearning.NewStore()

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

	go serveHTTP(cfg, smsCap)

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
				handleCallback(u.CallbackQuery, bot, store, ruleStore, cfg, *cfgPath)
			case u.Message != nil:
				if u.Message.Chat.ID != cfg.Telegram.ChatID {
					continue
				}
				if ruleStore.GetEditingByChatID(u.Message.Chat.ID) != nil {
					handleRuleEditReply(u.Message.Chat.ID, u.Message.Text, bot, ruleStore)
				} else {
					rt.Route(u.Message.Chat.ID, u.Message.Text)
				}
			}
		}
	}
}

func handleCallback(
	cb *telegram.CallbackQuery,
	bot *telegram.Bot,
	store *approval.Store,
	ruleStore *rulelearning.Store,
	cfg *config.Config,
	cfgPath string,
) {
	bot.AnswerCallback(cb.ID)

	if cb.Message == nil {
		return
	}
	if cb.Message.Chat.ID != cfg.Telegram.ChatID {
		return
	}
	msgID := cb.Message.MessageID

	switch strings.ToLower(cb.Data) {
	case "approve":
		pending := store.Get(msgID)
		if pending == nil {
			log.Debugf("callback for unknown messageID %d (agent may have restarted)", msgID)
			return
		}
		pending.Entry.Source = "telegram_approved"
		if err := agentledger.Append(cfg.Paisa.JournalDir, &pending.Entry); err != nil {
			log.Errorf("append entry: %v", err)
			bot.EditMessage(msgID, "❌ Failed to post: "+err.Error())
			return
		}
		bot.EditMessage(msgID, "✅ Posted\n\n"+telegram.FormatDraft(pending.Entry))
		log.Infof("posted: %s %s %s", pending.Entry.Date, pending.Entry.Desc, pending.Entry.Amt)

		// Rule learning: propose a merchant rule if dest was corrected
		kw, acc, desc, ok := rulelearning.Derive(pending.Original, pending.Entry)
		store.Delete(msgID)
		if ok {
			dup, err := config.IsDuplicateKeyword(cfgPath, kw)
			if err != nil {
				log.Warnf("rulelearning: duplicate check: %v — skipping rule proposal", err)
			} else if !dup {
				confirmText := fmt.Sprintf(
					"📝 New rule detected:\n  keyword: %q\n  account: %q\n  description: %q\n\nAdd to paisa-agent.yaml?",
					kw, acc, desc,
				)
				ruleMsgID, err := bot.SendRuleConfirmation(confirmText)
				if err != nil {
					log.Errorf("rulelearning: send confirmation: %v", err)
					return
				}
				ruleStore.Set(&rulelearning.PendingRule{
					MessageID:   ruleMsgID,
					ChatID:      cfg.Telegram.ChatID,
					Keyword:     kw,
					Account:     acc,
					Description: desc,
					Status:      rulelearning.RuleStatusPending,
				})
				log.Infof("rulelearning: pending confirmation msgID=%d keyword=%q", ruleMsgID, kw)
			}
		}

	case "edit":
		pending := store.Get(msgID)
		if pending == nil {
			log.Debugf("callback for unknown messageID %d (agent may have restarted)", msgID)
			return
		}
		store.SetEditing(msgID)
		bot.SendText(telegram.FormatEditTemplate(pending.Entry))

	case "skip":
		pending := store.Get(msgID)
		if pending == nil {
			log.Debugf("callback for unknown messageID %d (agent may have restarted)", msgID)
			return
		}
		bot.EditMessage(msgID, "⏭ Skipped\n\n"+telegram.FormatDraft(pending.Entry))
		store.Delete(msgID)

	case "add_rule":
		rule := ruleStore.Get(msgID)
		if rule == nil {
			log.Debugf("add_rule callback for unknown msgID=%d", msgID)
			return
		}
		if err := config.PrependMerchantRule(cfgPath, config.MerchantRule{
			Keyword:     rule.Keyword,
			Account:     rule.Account,
			Description: rule.Description,
		}); err != nil {
			log.Errorf("rulelearning: write rule: %v", err)
			bot.EditMessage(msgID, fmt.Sprintf("❌ Failed to write rule: %v", err))
			return
		}
		bot.EditMessage(msgID, fmt.Sprintf("✅ Rule added: %q → %q", rule.Keyword, rule.Account))
		ruleStore.Delete(msgID)
		log.Infof("rulelearning: rule written keyword=%q account=%q", rule.Keyword, rule.Account)

	case "skip_rule":
		rule := ruleStore.Get(msgID)
		if rule == nil {
			log.Debugf("skip_rule callback for unknown msgID=%d", msgID)
			return
		}
		bot.EditMessage(msgID, "⏭ Rule skipped")
		ruleStore.Delete(msgID)
		log.Debugf("rulelearning: rule skipped msgID=%d keyword=%q", msgID, rule.Keyword)

	case "edit_rule":
		rule := ruleStore.Get(msgID)
		if rule == nil {
			log.Debugf("edit_rule callback for unknown msgID=%d", msgID)
			return
		}
		if err := bot.SendText(rulelearning.FormatEditTemplate(*rule)); err != nil {
			log.Errorf("rulelearning: send edit template: %v", err)
			return
		}
		ruleStore.SetEditing(msgID)
		log.Debugf("rulelearning: edit template sent msgID=%d keyword=%q", msgID, rule.Keyword)
	}
}

func handleRuleEditReply(chatID int64, text string, bot *telegram.Bot, ruleStore *rulelearning.Store) {
	rule := ruleStore.GetEditingByChatID(chatID)
	if rule == nil {
		return
	}
	updated := rulelearning.ParseEditReply(text, *rule)
	confirmText := fmt.Sprintf(
		"📝 Updated rule:\n  keyword: %q\n  account: %q\n  description: %q\n\nAdd to paisa-agent.yaml?",
		updated.Keyword, updated.Account, updated.Description,
	)
	msgID, err := bot.SendRuleConfirmationFinal(confirmText)
	if err != nil {
		log.Errorf("rulelearning: send final confirmation: %v", err)
		return // rule stays in editing state; next message retries
	}
	ruleStore.Delete(rule.MessageID)
	updated.MessageID = msgID
	updated.Status = rulelearning.RuleStatusPending
	ruleStore.Set(&updated)
	log.Infof("rulelearning: rule updated and re-confirmed msgID=%d keyword=%q", msgID, updated.Keyword)
}

func handleStatementEmail(
	ev gmail.StatementEmail,
	parsers []statement.Parser,
	pc *paisaclient.Client,
	journalDir string,
	bot *telegram.Bot,
) {
	var result statement.ParseResult
	var parsed bool
	for _, p := range parsers {
		if !p.Detect(ev.Subject) {
			continue
		}
		r, err := p.Parse(ev.PDFBytes)
		if err != nil {
			log.Errorf("statement: parse %s: %v", p.Name(), err)
			bot.SendText(fmt.Sprintf("❌ Failed to parse statement (%s): %v", p.Name(), err)) //nolint:errcheck
			return
		}
		result = r
		parsed = true
		break
	}
	if !parsed {
		log.Warnf("statement: no parser matched subject=%q", ev.Subject)
		bot.SendText(fmt.Sprintf("❌ No parser matched statement email: %q", ev.Subject)) //nolint:errcheck
		return
	}

	postings, err := pc.Postings()
	if err != nil {
		log.Errorf("reconcile: fetch postings: %v", err)
		bot.SendText(fmt.Sprintf("❌ Failed to fetch ledger for %s: %v", ev.LedgerAccount, err)) //nolint:errcheck
		return
	}

	var entries []reconcile.LedgerEntry
	for _, p := range postings {
		if p.Account != ev.LedgerAccount {
			continue
		}
		if int(p.Date.Month()) != int(result.Month) || p.Date.Year() != result.Year {
			continue
		}
		entries = append(entries, reconcile.LedgerEntry{
			Date:        p.Date,
			Description: p.Payee,
			Amount:      p.Amount,
		})
	}

	diff := reconcile.Compare(result, entries)
	diff.Account = ev.LedgerAccount

	rec := reconcile.Record{
		Period:      fmt.Sprintf("%04d-%02d", result.Year, int(result.Month)),
		GeneratedAt: time.Now(),
		Diff:        diff,
	}
	if err := reconcile.Write(journalDir, rec); err != nil {
		log.Errorf("reconcile: write store: %v", err)
	}

	// Telegram report.
	var sb strings.Builder
	acctShort := ev.LedgerAccount
	if idx := strings.LastIndex(acctShort, ":"); idx >= 0 {
		acctShort = acctShort[idx+1:]
	}
	fmt.Fprintf(&sb, "📊 %s — %s %d\n\n", acctShort, result.Month, result.Year)
	fmt.Fprintf(&sb, "Closing balance: ₹%.2f\n", result.ClosingBalance)
	fmt.Fprintf(&sb, "Statement txns: %d | Matched in ledger: %d\n\n",
		len(result.Transactions), len(result.Transactions)-len(diff.Missing))

	if len(diff.Missing) == 0 && len(diff.Extra) == 0 {
		sb.WriteString("✅ All transactions matched\n")
	}
	if len(diff.Missing) > 0 {
		fmt.Fprintf(&sb, "❌ %d missing from ledger:\n", len(diff.Missing))
		for _, tx := range diff.Missing {
			if tx.Debit > 0 {
				fmt.Fprintf(&sb, "  • %s %s  -₹%.2f\n", tx.Date.Format("02-01"), tx.Description, tx.Debit)
			} else {
				fmt.Fprintf(&sb, "  • %s %s  +₹%.2f\n", tx.Date.Format("02-01"), tx.Description, tx.Credit)
			}
		}
	}
	if len(diff.Extra) > 0 {
		fmt.Fprintf(&sb, "⚠️ %d extra in ledger (not in statement):\n", len(diff.Extra))
		for _, le := range diff.Extra {
			fmt.Fprintf(&sb, "  • %s %s  %.2f\n", le.Date.Format("02-01"), le.Description, le.Amount)
		}
	}

	if err := bot.SendText(sb.String()); err != nil {
		log.Errorf("reconcile: send Telegram report: %v", err)
	}
}

func serveHTTP(cfg *config.Config, smsCap *sms.Capability) {
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
		entry, err := smsCap.ParseAndFill(req.Text)
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
		entry.Source = "ui"
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

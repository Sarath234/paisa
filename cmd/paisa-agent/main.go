// cmd/paisa-agent/main.go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/approval"
	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
	"github.com/ananthakumaran/paisa/internal/agent/ccrecon"
	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/dropfolder"
	"github.com/ananthakumaran/paisa/internal/agent/gmail"
	agentledger "github.com/ananthakumaran/paisa/internal/agent/ledger"
	"github.com/ananthakumaran/paisa/internal/agent/llm"
	"github.com/ananthakumaran/paisa/internal/agent/monitor"
	"github.com/ananthakumaran/paisa/internal/agent/notices"
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

	parsers := []statement.Parser{&statement.AxisParser{}}
	pc := paisaclient.New(cfg.Paisa.URL)

	truthStore, err := billtruth.Open(cfg.Paisa.JournalDir)
	if err != nil {
		log.Fatalf("billtruth: open store: %v", err)
	}

	store := approval.NewStore()

	ccParsers := map[string]statement.CCParser{
		"icici_cc": &statement.ICICICCParser{},
		"axis_cc":  &statement.AxisCCParser{},
		"hdfc_cc":  &statement.HDFCCCParser{},
	}
	ccreconDeps := &ccrecon.Deps{
		Store:      truthStore,
		Parsers:    ccParsers,
		Client:     pc,
		Approvals:  store,
		Bot:        bot,
		ChatID:     cfg.Telegram.ChatID,
		Merchants:  cfg.ParserRules.Merchants,
		JournalDir: cfg.Paisa.JournalDir,
		MaxCards:   10,
	}

	var gmailClient *gmail.Client
	var gmailPoller *gmail.Poller

	if cfg.Gmail != nil {
		var err error
		gmailClient, err = gmail.New(cfg.Gmail.ClientID, cfg.Gmail.ClientSecret, cfg.Gmail.TokenFile)
		if err != nil {
			log.Fatalf("gmail: init: %v", err)
		}

		stateDir := filepath.Dir(cfg.Gmail.TokenFile)

		var subjectMatches []gmail.SubjectMatch
		for _, sa := range cfg.Gmail.Accounts {
			subjectMatches = append(subjectMatches, gmail.SubjectMatch{
				Pattern:       sa.SubjectMatch,
				LedgerAccount: sa.LedgerAccount,
			})
		}

		gmailPoller = gmail.NewPoller(gmailClient, subjectMatches, stateDir, func(ev gmail.StatementEmail) {
			//nolint:errcheck // errors already reported via Telegram inside
			handleStatement(ev.Subject, ev.PDFBytes, ev.LedgerAccount, parsers, pc, cfg.Paisa.JournalDir, bot)
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

	if cfg.Statements != nil && cfg.Statements.DropDir == "" {
		log.Errorf("statements: drop_dir is empty — skipping drop-folder poller")
	} else if cfg.Statements != nil {
		var matches []dropfolder.AccountMatch
		for _, a := range cfg.Statements.Accounts {
			matches = append(matches, dropfolder.AccountMatch{
				Pattern:       a.FilenameMatch,
				LedgerAccount: a.LedgerAccount,
				Kind:          a.Kind,
				Password:      a.PDFPassword,
			})
		}
		dropPoller := dropfolder.New(cfg.Statements.DropDir, matches,
			func(s dropfolder.Statement) error {
				if s.Kind == "credit_card" {
					return ccreconDeps.HandleCCStatement(s.Filename, s.PDFBytes, s.LedgerAccount, s.Password)
				}
				return handleStatement(s.Filename, s.PDFBytes, s.LedgerAccount, parsers, pc, cfg.Paisa.JournalDir, bot)
			},
			func(msg string) {
				if err := bot.SendText(msg); err != nil {
					log.Warnf("dropfolder: notify: %v", err)
				}
			})
		go dropPoller.Start()
		log.Infof("dropfolder: watching %s", cfg.Statements.DropDir)
	}

	if cfg.Monitors != nil {
		monStore, err := monitor.OpenStore(cfg.Paisa.JournalDir)
		if err != nil {
			log.Fatalf("monitor: open store: %v", err)
		}
		cc := cfg.Monitors.CreditCards
		hour := cfg.Monitors.DigestHour
		ccInterest := monitor.NewCCInterest(truthStore, pc, cc.InterestPatterns, hour)
		ccInterest.Sent = monStore.WasSent
		ccStatement := monitor.NewCCStatement(truthStore, hour)
		ccStatement.SentPrefix = monStore.WasSentPrefix
		mons := []monitor.Monitor{
			// apiSyncMonitor must run first each pass: it fills billtruth
			// holes from paisa's computed bills before cc_due reads the
			// store, so a card with no SMS/PDF facts yet still gets
			// reminders.
			&apiSyncMonitor{store: truthStore, client: pc, due: monitor.DailyAt(hour)},
			monitor.NewCCDue(truthStore, cc.DueReminderDays, hour),
			ccStatement,
			monitor.NewCCUtilization(pc, cc.UtilizationBands, hour),
			ccInterest,
			monitor.NewCCTruthGap(truthStore, pc, cc.TruthGapDays, hour),
		}
		sched := monitor.NewScheduler(mons, monitor.NewNotifier(bot, monStore), monStore, hour)
		go sched.Start()
		log.Infof("monitor: scheduler started (%d monitors, digest at %02d:00)", len(mons), hour)
	}

	cardsByLast4 := map[string]string{}
	for _, r := range cfg.ParserRules.Accounts {
		dest := r.Destinations
		if strings.HasPrefix(dest, "Liabilities:CreditCard:") && len(dest) >= 4 {
			last4 := dest[len(dest)-4:]
			if _, err := strconv.Atoi(last4); err == nil {
				cardsByLast4[last4] = dest
			}
		}
	}

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
	noticesCap := notices.NewCapability(truthStore, bot, cardsByLast4)

	rt := router.New(
		// notices MUST precede sms: statement/payment notice SMSes must never
		// fall through to transaction parsing.
		[]router.Capability{noticesCap, smsCap, qaCap},
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

	go serveHTTP(cfg, smsCap, qaCap.Answerer)

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
				handleCallback(u.CallbackQuery, bot, store, ruleStore, ccreconDeps, cfg, *cfgPath)
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

// apiSyncMonitor fills billtruth holes from paisa's computed bills
// (fill-holes-only: never overwrites sms/pdf facts). It emits no insights of
// its own; it must run before cc_due in the monitor slice so a card with no
// SMS/PDF facts yet still has a bill for cc_due to read.
type apiSyncMonitor struct {
	store  *billtruth.Store
	client billtruth.CreditCardLister
	due    func(now, lastRun time.Time) bool
}

func (m *apiSyncMonitor) Name() string { return "billtruth_api_sync" }

func (m *apiSyncMonitor) Due(now, lastRun time.Time) bool { return m.due(now, lastRun) }

func (m *apiSyncMonitor) Check(ctx context.Context) ([]monitor.Insight, error) {
	return nil, billtruth.SyncFromAPI(m.store, m.client)
}

func handleCallback(
	cb *telegram.CallbackQuery,
	bot *telegram.Bot,
	store *approval.Store,
	ruleStore *rulelearning.Store,
	ccreconDeps *ccrecon.Deps,
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

	data := strings.ToLower(cb.Data)
	if handled, err := ccreconDeps.HandleCallback(data, msgID); handled {
		if err != nil {
			log.Errorf("ccrecon: handle callback %q: %v", data, err)
		}
		return
	}

	switch data {
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

// handleStatement parses a statement PDF, reconciles it against the ledger,
// stores the diff, and reports via Telegram. name is the email subject or
// filename (parsers Detect on it). Returns an error when parsing or the
// ledger fetch failed (callers use it for file disposition); reconciliation
// store/report problems are logged, not returned.
func handleStatement(
	name string,
	pdfBytes []byte,
	ledgerAccount string,
	parsers []statement.Parser,
	pc *paisaclient.Client,
	journalDir string,
	bot *telegram.Bot,
) error {
	var result statement.ParseResult
	var parsed bool
	for _, p := range parsers {
		if !p.Detect(name) {
			continue
		}
		r, err := p.Parse(pdfBytes)
		if err != nil {
			log.Errorf("statement: parse %s: %v", p.Name(), err)
			bot.SendText(fmt.Sprintf("❌ Failed to parse statement (%s): %v", p.Name(), err)) //nolint:errcheck
			return fmt.Errorf("parse %s: %w", p.Name(), err)
		}
		result = r
		parsed = true
		break
	}
	if !parsed {
		log.Warnf("statement: no parser matched subject=%q", name)
		bot.SendText(fmt.Sprintf("❌ No parser matched statement email: %q", name)) //nolint:errcheck
		return fmt.Errorf("no parser matched %q", name)
	}

	postings, err := pc.Postings()
	if err != nil {
		log.Errorf("reconcile: fetch postings: %v", err)
		bot.SendText(fmt.Sprintf("❌ Failed to fetch ledger for %s: %v", ledgerAccount, err)) //nolint:errcheck
		return fmt.Errorf("fetch postings: %w", err)
	}

	var entries []reconcile.LedgerEntry
	for _, p := range postings {
		if p.Account != ledgerAccount {
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
	diff.Account = ledgerAccount

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
	acctShort := ledgerAccount
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
	return nil
}

func serveHTTP(cfg *config.Config, smsCap *sms.Capability, answerer *qa.Answerer) {
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

	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		q, err := qa.Extract(req.Message, cfg.Ollama)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		answer, err := answerer.Answer(q)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"reply": answer})
	})

	addr := "127.0.0.1:7501"
	log.Infof("paisa-agent HTTP listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("http server: %v", err)
	}
}

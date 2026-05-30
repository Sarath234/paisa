package pipeline

import (
	"fmt"
	"strings"
	"sync"

	agentconfig "github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/dedup"
	"github.com/ananthakumaran/paisa/internal/agent/journal"
	"github.com/ananthakumaran/paisa/internal/agent/merchant"
	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/ananthakumaran/paisa/internal/agent/telegram"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Action int

const (
	ActionPosted          Action = iota
	ActionPendingApproval        // sent to Telegram for approval
	ActionDuplicate              // dedup matched, skipped
	ActionSkipped                // user skipped via Telegram
)

type Pipeline struct {
	db      *gorm.DB
	cfg     agentconfig.Config
	parser  *parser.Parser
	bot     *telegram.Bot
	pending sync.Map // refID → parser.ParsedTransaction
}

func New(db *gorm.DB, cfg agentconfig.Config) *Pipeline {
	return &Pipeline{
		db:     db,
		cfg:    cfg,
		parser: parser.New(cfg.Ollama.URL, cfg.Ollama.Model),
		bot:    telegram.New(cfg.Telegram.BotToken, cfg.Telegram.ChatID),
	}
}

// Bot returns the underlying Telegram bot (used by main.go goroutine).
func (p *Pipeline) Bot() *telegram.Bot {
	return p.bot
}

// Process parses rawText from source, deduplicates, gates on merchant rules,
// and either auto-posts or sends a Telegram approval card.
func (p *Pipeline) Process(rawText, source string) (Action, error) {
	accounts := make([]string, 0, len(p.cfg.Accounts))
	for _, v := range p.cfg.Accounts {
		accounts = append(accounts, v)
	}

	tx, err := p.parser.Parse(rawText, accounts)
	if err != nil {
		return ActionSkipped, fmt.Errorf("parse: %w", err)
	}

	result := dedup.Check(p.db, tx.RefID, tx.Date, tx.Bank+":"+tx.AccountLast4, tx.Amount)
	if result == dedup.Duplicate {
		log.Infof("pipeline: duplicate skipped ref=%s", tx.RefID)
		return ActionDuplicate, nil
	}
	if result == dedup.Fuzzy {
		log.Warnf("pipeline: fuzzy duplicate, sending for approval ref=%s", tx.RefID)
		return ActionPendingApproval, p.sendApproval(tx)
	}

	absAmount := tx.Amount
	if absAmount < 0 {
		absAmount = -absAmount
	}
	decision := merchant.Gate(p.db, tx.Merchant, absAmount, tx.Confidence,
		p.cfg.MerchantRules.AutoApproveThreshold,
		p.cfg.MerchantRules.PromoteAfterApprovals)

	if decision == merchant.AutoPost {
		return ActionPosted, p.post(tx, source)
	}

	return ActionPendingApproval, p.sendApproval(tx)
}

func parseCallback(data string) (action, refID string) {
	idx := strings.Index(data, ":")
	if idx < 0 {
		return "", ""
	}
	return data[:idx], data[idx+1:]
}

// HandleCallback processes a Telegram inline button callback.
// data format: "approve:<refID>" | "skip:<refID>" | "edit:<refID>"
func (p *Pipeline) HandleCallback(callbackID, data string) {
	_ = p.bot.AnswerCallback(callbackID)

	action, refID := parseCallback(data)
	if action == "" {
		log.Warnf("pipeline: malformed callback data %q", data)
		return
	}

	val, ok := p.pending.Load(refID)
	if !ok {
		log.Warnf("pipeline: callback for unknown refID %s", refID)
		return
	}
	tx := val.(parser.ParsedTransaction)

	switch action {
	case "approve":
		if err := p.post(tx, "telegram_approved"); err != nil {
			log.Errorf("pipeline: post failed: %v", err)
			_ = p.bot.SendText("❌ Failed to post transaction")
			return
		}
		_ = merchant.RecordApproval(p.db, tx.Merchant, tx.SuggestedLedgerAccount,
			p.cfg.MerchantRules.PromoteAfterApprovals)
		p.pending.Delete(refID)

	case "skip":
		_ = dedup.Record(p.db, tx.RefID, tx.Date, tx.Bank+":"+tx.AccountLast4, "skipped", tx.Amount)
		p.pending.Delete(refID)
		_ = p.bot.SendText("✗ Skipped")

	case "edit":
		_ = p.bot.SendText("Send the correct ledger account name (e.g. Expenses:Transport:Fuel):")
		p.pending.Store(refID+"_edit", tx)
	}
}

func (p *Pipeline) post(tx parser.ParsedTransaction, source string) error {
	debitAccount := p.resolveAccount(tx.Bank, tx.AccountLast4)
	entry := journal.Format(tx, source, debitAccount)
	if err := journal.Append(p.cfg.Paisa.JournalDir, entry); err != nil {
		return fmt.Errorf("append journal: %w", err)
	}
	if err := journal.TriggerSync(p.cfg.Paisa.URL, p.cfg.Paisa.APIToken); err != nil {
		log.Warnf("pipeline: sync failed (non-fatal): %v", err)
	}
	_ = dedup.Record(p.db, tx.RefID, tx.Date, tx.Bank+":"+tx.AccountLast4, source, tx.Amount)
	log.Infof("pipeline: posted %s %.2f %s", tx.Merchant, tx.Amount, tx.Currency)
	return nil
}

func (p *Pipeline) sendApproval(tx parser.ParsedTransaction) error {
	p.pending.Store(tx.RefID, tx)
	return p.bot.SendApprovalCard(tx)
}

func (p *Pipeline) resolveAccount(bank, last4 string) string {
	key := bank + ":" + last4
	if acct, ok := p.cfg.Accounts[key]; ok {
		return acct
	}
	return "Assets:" + bank + ":XX" + last4
}

// internal/agent/config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Paisa       PaisaConfig       `yaml:"paisa"`
	Ollama      OllamaConfig      `yaml:"ollama"`
	Telegram    TelegramConfig    `yaml:"telegram"`
	ParserRules ParserRules       `yaml:"parser_rules"`
	Gmail       *GmailConfig      `yaml:"gmail,omitempty"`
	Monitors    *MonitorsConfig   `yaml:"monitors,omitempty"`
	Statements  *StatementsConfig `yaml:"statements,omitempty"`
}

type PaisaConfig struct {
	URL        string `yaml:"url"`
	JournalDir string `yaml:"journal_dir"`
}

type OllamaConfig struct {
	URL   string `yaml:"url"`
	Model string `yaml:"model"`
}

type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   int64  `yaml:"chat_id"`
}

type ParserRules struct {
	Accounts  []AccountRule  `yaml:"accounts"`
	Merchants []MerchantRule `yaml:"merchants"`
}

// AccountRule matches an SMS to a bank account.
// Fixed routes (bank="fixed") have Src+Description set; format routes do not.
// Identifiers are AND-matched: ALL must appear in the SMS.
// Destinations = first ledger posting account (Entry.Src).
// Src (fixed only) = second ledger posting account (Entry.Dest), auto-balanced.
type AccountRule struct {
	Bank         string   `yaml:"bank"`
	Identifiers  []string `yaml:"identifiers"`
	Destinations string   `yaml:"destinations"`
	Src          string   `yaml:"src"`
	Description  string   `yaml:"description"`
}

type MerchantRule struct {
	Keyword     string `yaml:"keyword"`
	Account     string `yaml:"account"`
	Description string `yaml:"description"`
}

type GmailConfig struct {
	ClientID     string             `yaml:"client_id"`
	ClientSecret string             `yaml:"client_secret"`
	TokenFile    string             `yaml:"token_file"`
	Accounts     []StatementAccount `yaml:"statement_accounts"`
}

type StatementAccount struct {
	SubjectMatch  string `yaml:"subject_match"`
	LedgerAccount string `yaml:"ledger_account"`
}

type MonitorsConfig struct {
	DigestHour  int                      `yaml:"digest_hour"`
	CreditCards CreditCardsMonitorConfig `yaml:"credit_cards"`
}

type CreditCardsMonitorConfig struct {
	DueReminderDays  []int    `yaml:"due_reminder_days"`
	UtilizationBands []int    `yaml:"utilization_bands"`
	InterestPatterns []string `yaml:"interest_patterns"`
}

type StatementsConfig struct {
	DropDir  string              `yaml:"drop_dir"`
	Accounts []DropfolderAccount `yaml:"accounts"`
}

type DropfolderAccount struct {
	FilenameMatch string `yaml:"filename_match"`
	LedgerAccount string `yaml:"ledger_account"`
}

func (m *MonitorsConfig) setDefaults() {
	if m.DigestHour == 0 {
		m.DigestHour = 8
	}
	if len(m.CreditCards.DueReminderDays) == 0 {
		m.CreditCards.DueReminderDays = []int{3, 1, 0}
	}
	if len(m.CreditCards.UtilizationBands) == 0 {
		m.CreditCards.UtilizationBands = []int{50, 75, 90}
	}
	if len(m.CreditCards.InterestPatterns) == 0 {
		m.CreditCards.InterestPatterns = []string{"INTEREST", "LATE FEE"}
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Monitors != nil {
		cfg.Monitors.setDefaults()
		if h := cfg.Monitors.DigestHour; h < 0 || h > 23 {
			return nil, fmt.Errorf("monitors.digest_hour must be between 0 and 23, got %d", h)
		}
	}
	if cfg.Statements != nil && strings.HasPrefix(cfg.Statements.DropDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		cfg.Statements.DropDir = filepath.Join(home, cfg.Statements.DropDir[2:])
	}
	return &cfg, nil
}

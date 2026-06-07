// internal/agent/config/config.go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Paisa       PaisaConfig    `yaml:"paisa"`
	Ollama      OllamaConfig   `yaml:"ollama"`
	Telegram    TelegramConfig `yaml:"telegram"`
	ParserRules ParserRules    `yaml:"parser_rules"`
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

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Paisa         PaisaConfig         `yaml:"paisa"`
	Ollama        OllamaConfig        `yaml:"ollama"`
	Telegram      TelegramConfig      `yaml:"telegram"`
	Gmail         GmailConfig         `yaml:"gmail"`
	MerchantRules MerchantRulesConfig `yaml:"merchant_rules"`
	Accounts      map[string]string   `yaml:"accounts"` // "BANK:last4" → "Assets:BANK:AccountName"
}

type PaisaConfig struct {
	URL        string `yaml:"url"`
	JournalDir string `yaml:"journal_dir"`
	APIToken   string `yaml:"api_token"`
}

type OllamaConfig struct {
	URL   string `yaml:"url"`
	Model string `yaml:"model"`
}

type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   int64  `yaml:"chat_id"`
}

type GmailConfig struct {
	CredentialsFile     string   `yaml:"credentials_file"`
	PollIntervalSeconds int      `yaml:"poll_interval_seconds"`
	Labels              []string `yaml:"labels"`
}

type MerchantRulesConfig struct {
	AutoApproveThreshold  float64 `yaml:"auto_approve_threshold"`
	PromoteAfterApprovals int     `yaml:"promote_after_approvals"`
}

func DefaultConfig() Config {
	return Config{
		Paisa:  PaisaConfig{URL: "http://localhost:7500"},
		Ollama: OllamaConfig{URL: "http://localhost:11434", Model: "gemma3:12b"},
		Gmail:  GmailConfig{PollIntervalSeconds: 300},
		MerchantRules: MerchantRulesConfig{
			AutoApproveThreshold:  10000,
			PromoteAfterApprovals: 3,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	return cfg, yaml.Unmarshal(data, &cfg)
}

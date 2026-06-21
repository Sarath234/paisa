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
	ParserRules   ParserRules         `yaml:"parser_rules"`
	Listen        string              `yaml:"listen"`
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

type ParserRules struct {
	DayParts  DayPartsConfig    `yaml:"day_parts"`
	Merchants []MerchantPattern `yaml:"merchants"`
	Sources   []SourceRule      `yaml:"sources"`
}

type DayPartsConfig struct {
	BreakfastEnd int `yaml:"breakfast_end"`
	LunchEnd     int `yaml:"lunch_end"`
	DinnerEnd    int `yaml:"dinner_end"`
}

type MerchantPattern struct {
	Keyword     string `yaml:"keyword"`
	Description string `yaml:"description"`
	Account     string `yaml:"account"`
}

type SourceRule struct {
	ID          string   `yaml:"id"`
	Contains    []string `yaml:"contains"`
	Account     string   `yaml:"account"`
	DestAccount string   `yaml:"dest_account"`
	Description string   `yaml:"description"`
}

func DefaultConfig() Config {
	return Config{
		Paisa:  PaisaConfig{URL: "http://localhost:7500"},
		Ollama: OllamaConfig{URL: "http://localhost:11434", Model: "gemma3:12b"},
		Listen: "127.0.0.1:7501",
		Gmail:  GmailConfig{PollIntervalSeconds: 300},
		MerchantRules: MerchantRulesConfig{
			AutoApproveThreshold:  10000,
			PromoteAfterApprovals: 3,
		},
		ParserRules: ParserRules{
			DayParts: DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20},
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

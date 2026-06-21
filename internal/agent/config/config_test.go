package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "http://localhost:7500", cfg.Paisa.URL)
	assert.Equal(t, "http://localhost:11434", cfg.Ollama.URL)
	assert.Equal(t, "gemma3:12b", cfg.Ollama.Model)
	assert.Equal(t, 300, cfg.Gmail.PollIntervalSeconds)
	assert.Equal(t, float64(10000), cfg.MerchantRules.AutoApproveThreshold)
	assert.Equal(t, 3, cfg.MerchantRules.PromoteAfterApprovals)
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	yaml := `
paisa:
  url: http://myhost:7500
ollama:
  model: gemma3:27b
telegram:
  bot_token: "abc123"
  chat_id: 999
`
	path := filepath.Join(dir, "paisa-agent.yaml")
	_ = os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := Load(path)
	assert.NoError(t, err)
	assert.Equal(t, "http://myhost:7500", cfg.Paisa.URL)
	assert.Equal(t, "gemma3:27b", cfg.Ollama.Model)
	assert.Equal(t, "abc123", cfg.Telegram.BotToken)
	assert.Equal(t, int64(999), cfg.Telegram.ChatID)
	// defaults preserved for unset fields
	assert.Equal(t, float64(10000), cfg.MerchantRules.AutoApproveThreshold)
}

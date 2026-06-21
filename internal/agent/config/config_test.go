// internal/agent/config/config_test.go
package config_test

import (
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	cfg, err := config.Load("testdata/sample.yaml")
	assert.NoError(t, err)
	assert.Equal(t, "http://localhost:7500", cfg.Paisa.URL)
	assert.Equal(t, "/tmp/journal", cfg.Paisa.JournalDir)
	assert.Equal(t, "gemma3:4b", cfg.Ollama.Model)
	assert.Equal(t, int64(123456), cfg.Telegram.ChatID)
	assert.Len(t, cfg.ParserRules.Accounts, 2)
	assert.Equal(t, "fixed", cfg.ParserRules.Accounts[0].Bank)
	assert.Equal(t, []string{"CRD-PMNT", "2222"}, cfg.ParserRules.Accounts[0].Identifiers)
	assert.Equal(t, "Assets:Checking:AXIS1111", cfg.ParserRules.Accounts[0].Src)
	assert.Len(t, cfg.ParserRules.Merchants, 1)
	assert.Equal(t, "swiggy", cfg.ParserRules.Merchants[0].Keyword)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load("testdata/nonexistent.yaml")
	assert.Error(t, err)
}

// internal/agent/config/config_test.go
package config_test

import (
	"os"
	"path/filepath"
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

func TestLoadMonitorsAndStatements(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "paisa-agent.yaml")
	yaml := `
paisa:
  url: http://localhost:7500
  journal_dir: /tmp/j
telegram:
  bot_token: x
  chat_id: 1
monitors:
  digest_hour: 9
  credit_cards:
    due_reminder_days: [7, 3, 1, 0]
statements:
  drop_dir: /tmp/statements
  accounts:
    - filename_match: "*Axis*"
      ledger_account: Liabilities:CreditCard:Axis
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Monitors == nil || cfg.Monitors.DigestHour != 9 {
		t.Fatalf("digest_hour: got %+v", cfg.Monitors)
	}
	if got := cfg.Monitors.CreditCards.DueReminderDays; len(got) != 4 || got[0] != 7 {
		t.Fatalf("due_reminder_days: got %v", got)
	}
	// omitted keys get defaults
	if got := cfg.Monitors.CreditCards.UtilizationBands; len(got) != 3 || got[0] != 50 || got[2] != 90 {
		t.Fatalf("utilization_bands default: got %v", got)
	}
	if got := cfg.Monitors.CreditCards.InterestPatterns; len(got) != 2 || got[0] != "INTEREST" {
		t.Fatalf("interest_patterns default: got %v", got)
	}
	if cfg.Statements == nil || cfg.Statements.DropDir != "/tmp/statements" {
		t.Fatalf("statements: got %+v", cfg.Statements)
	}
	if len(cfg.Statements.Accounts) != 1 || cfg.Statements.Accounts[0].LedgerAccount != "Liabilities:CreditCard:Axis" {
		t.Fatalf("statement accounts: got %+v", cfg.Statements.Accounts)
	}
}

func TestLoadStatementsDropDirTildeExpansion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "paisa-agent.yaml")
	yaml := `
paisa:
  url: http://localhost:7500
  journal_dir: /tmp/j
telegram:
  bot_token: x
  chat_id: 1
statements:
  drop_dir: ~/Downloads/statements
  accounts:
    - filename_match: "*Axis*"
      ledger_account: Liabilities:CreditCard:Axis
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Downloads/statements")
	if cfg.Statements.DropDir != want {
		t.Fatalf("drop_dir: got %q, want %q", cfg.Statements.DropDir, want)
	}
}

func TestLoadStatementsDropDirNoTildeUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "paisa-agent.yaml")
	yaml := `
paisa:
  url: http://localhost:7500
  journal_dir: /tmp/j
statements:
  drop_dir: /absolute/path/statements
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Statements.DropDir != "/absolute/path/statements" {
		t.Fatalf("drop_dir: got %q", cfg.Statements.DropDir)
	}
}

func TestLoadMonitorsAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "paisa-agent.yaml")
	yaml := "paisa:\n  url: http://localhost:7500\n  journal_dir: /tmp/j\n"
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Monitors != nil || cfg.Statements != nil {
		t.Fatalf("expected nil Monitors/Statements, got %+v %+v", cfg.Monitors, cfg.Statements)
	}
}

func TestLoadMonitorsDefaultDigestHour(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "paisa-agent.yaml")
	yaml := "paisa:\n  url: http://x\n  journal_dir: /tmp/j\nmonitors: {}\n"
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Monitors.DigestHour != 8 {
		t.Fatalf("default digest_hour: got %d", cfg.Monitors.DigestHour)
	}
	if got := cfg.Monitors.CreditCards.DueReminderDays; len(got) != 3 || got[0] != 3 || got[2] != 0 {
		t.Fatalf("default due_reminder_days: got %v", got)
	}
}

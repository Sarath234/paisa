# Paisa Ingestion Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `paisa-agent`, a standalone Go binary that ingests bank transactions from Gmail and SMS (via Telegram), auto-posts known merchants, routes unknown merchants through Telegram approval, and appends hledger journal entries + calls Paisa's existing `/api/sync`.

**Architecture:** Standalone binary in `cmd/paisa-agent/` within the same Go module as Paisa but with no shared code. Internal packages under `internal/agent/`. Two goroutines: Gmail poller and Telegram long-poller. Both feed into a shared pipeline that deduplicates, gates on merchant rules, writes to `auto-imported.journal`, and calls `POST /api/sync`.

**Tech Stack:** Go, SQLite (gorm + modernc sqlite already in go.mod), yaml.v3 (already in go.mod), Google Gmail REST API v1 + golang.org/x/oauth2 (new), github.com/ledongthuc/pdf (new), Telegram Bot API over raw HTTP, Ollama HTTP API over raw HTTP.

---

## File Map

```
cmd/paisa-agent/
  main.go                            entry point: load config, open db, start goroutines

internal/agent/
  config/
    config.go                        Config struct, Load(), DefaultConfig()
    config_test.go

  db/
    models.go                        ImportedRef, MerchantRule GORM models
    db.go                            Open() → SQLite + AutoMigrate
    db_test.go

  parser/
    parser.go                        Parser struct, Parse(rawText) → ParsedTransaction
    parser_test.go

  dedup/
    dedup.go                         Check(), Record()
    dedup_test.go

  merchant/
    merchant.go                      Gate(), RecordApproval(), Bootstrap()
    merchant_test.go

  journal/
    journal.go                       Format(), Append()
    sync.go                          TriggerSync()
    journal_test.go

  telegram/
    telegram.go                      Bot struct, GetUpdates(), SendApprovalCard(), SendText(), AnswerCallback()
    telegram_test.go

  gmail/
    auth.go                          OAuth2 token setup, NewGmailService()
    gmail.go                         Poller struct, Poll() → []RawEmail
    classify.go                      Classify(email) → AlertEmail | StatementEmail
    pdf.go                           ExtractPDFText(attachment) → string
    gmail_test.go

  pipeline/
    pipeline.go                      Process(rawText, source, accounts), HandleCallback()
    pipeline_test.go
```

---

## Task 1: Add dependencies + binary scaffold

**Files:**
- Modify: `go.mod`
- Create: `cmd/paisa-agent/main.go`

- [ ] **Step 1: Add new Go dependencies**

```bash
cd /Users/sarath.m/workspace/work/paisa
go get google.golang.org/api/gmail/v1
go get golang.org/x/oauth2
go get golang.org/x/oauth2/google
go get github.com/ledongthuc/pdf
```

Expected: `go.mod` and `go.sum` updated, no errors.

- [ ] **Step 2: Create the binary entry point**

Create `cmd/paisa-agent/main.go`:

```go
package main

import (
	"flag"
	"os"
	"path/filepath"

	agentconfig "github.com/ananthakumaran/paisa/internal/agent/config"
	log "github.com/sirupsen/logrus"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "", "path to paisa-agent.yaml (default: ~/.config/paisa-agent/paisa-agent.yaml)")
	flag.Parse()

	if cfgPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		cfgPath = filepath.Join(home, ".config", "paisa-agent", "paisa-agent.yaml")
	}

	cfg, err := agentconfig.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	log.Infof("paisa-agent starting, paisa URL: %s", cfg.Paisa.URL)
	// goroutines added in Task 10
	select {}
}
```

- [ ] **Step 3: Verify it builds**

```bash
go build ./cmd/paisa-agent/
```

Expected: produces `paisa-agent` binary in current directory with no errors.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum cmd/paisa-agent/main.go
git commit -m "feat(agent): scaffold paisa-agent binary + add Gmail/PDF deps"
```

---

## Task 2: Config

**Files:**
- Create: `internal/agent/config/config.go`
- Create: `internal/agent/config/config_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/agent/config/config_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/config/ -v
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement config**

Create `internal/agent/config/config.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/config/ -v
```

Expected: PASS — 2 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/config/
git commit -m "feat(agent): config loading with defaults"
```

---

## Task 3: Database models + migrations

**Files:**
- Create: `internal/agent/db/models.go`
- Create: `internal/agent/db/db.go`
- Create: `internal/agent/db/db_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/agent/db/db_test.go`:

```go
package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenAndMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")
	gdb, err := Open(path)
	assert.NoError(t, err)
	assert.NotNil(t, gdb)

	// Verify tables exist by inserting and reading back
	ref := ImportedRef{RefID: "REF001", Date: "2025-05-14", Amount: 2450, Account: "HDFC:1234", Source: "sms"}
	assert.NoError(t, gdb.Create(&ref).Error)

	var got ImportedRef
	assert.NoError(t, gdb.First(&got, "ref_id = ?", "REF001").Error)
	assert.Equal(t, "2025-05-14", got.Date)

	rule := MerchantRule{Merchant: "Swiggy", Account: "Expenses:Food:Dining", ApproveCount: 3, AutoApprove: true}
	assert.NoError(t, gdb.Create(&rule).Error)

	var gotRule MerchantRule
	assert.NoError(t, gdb.First(&gotRule, "merchant = ?", "Swiggy").Error)
	assert.True(t, gotRule.AutoApprove)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/db/ -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement models**

Create `internal/agent/db/models.go`:

```go
package db

import "gorm.io/gorm"

// ImportedRef tracks every transaction the agent has processed.
// Used for deduplication across sources.
type ImportedRef struct {
	gorm.Model
	RefID   string  `gorm:"index"`
	Date    string
	Amount  float64
	Account string
	Source  string // sms | gmail_alert | gmail_statement | skipped
}

// MerchantRule maps a merchant name to a ledger account and tracks
// how many times the user has manually approved it.
type MerchantRule struct {
	Merchant     string `gorm:"primaryKey"`
	Account      string
	ApproveCount int
	AutoApprove  bool
}
```

- [ ] **Step 4: Implement db.go**

Create `internal/agent/db/db.go`:

```go
package db

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	return db, db.AutoMigrate(&ImportedRef{}, &MerchantRule{})
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/agent/db/ -v
```

Expected: PASS — 1 test.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/db/
git commit -m "feat(agent): SQLite models and migrations for imported_refs and merchant_rules"
```

---

## Task 4: Ollama parser

**Files:**
- Create: `internal/agent/parser/parser.go`
- Create: `internal/agent/parser/parser_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/agent/parser/parser_test.go`:

```go
package parser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse_ExtractsTransaction(t *testing.T) {
	// Mock Ollama server returning a structured response
	mockResp := ParsedTransaction{
		Date:                   "2025-05-14",
		Amount:                 -2450.00,
		Currency:               "INR",
		Merchant:               "Swiggy",
		AccountLast4:           "1234",
		Bank:                   "HDFC",
		RefID:                  "47291830",
		TxType:                 "debit",
		SuggestedLedgerAccount: "Expenses:Food:Dining",
		Confidence:             0.95,
	}
	content, _ := json.Marshal(mockResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": map[string]string{"content": string(content)},
		})
	}))
	defer srv.Close()

	p := New(srv.URL, "gemma3:12b")
	tx, err := p.Parse("HDFC Bank: Rs.2,450.00 debited from a/c XX1234 at SWIGGY Ref 47291830", []string{"Expenses:Food:Dining"})

	assert.NoError(t, err)
	assert.Equal(t, "Swiggy", tx.Merchant)
	assert.Equal(t, -2450.00, tx.Amount)
	assert.Equal(t, "Expenses:Food:Dining", tx.SuggestedLedgerAccount)
	assert.Equal(t, 0.95, tx.Confidence)
}

func TestParse_HandlesOllamaError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	p := New(srv.URL, "gemma3:12b")
	_, err := p.Parse("some text", nil)
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/parser/ -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement parser**

Create `internal/agent/parser/parser.go`:

```go
package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type ParsedTransaction struct {
	Date                   string  `json:"date"`
	Amount                 float64 `json:"amount"`
	Currency               string  `json:"currency"`
	Merchant               string  `json:"merchant"`
	AccountLast4           string  `json:"account_last4"`
	Bank                   string  `json:"bank"`
	RefID                  string  `json:"ref_id"`
	TxType                 string  `json:"tx_type"`
	SuggestedLedgerAccount string  `json:"suggested_ledger_account"`
	Confidence             float64 `json:"confidence"`
}

type Parser struct {
	url   string
	model string
}

func New(url, model string) *Parser {
	return &Parser{url: url, model: model}
}

var responseSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"date":                     map[string]string{"type": "string"},
		"amount":                   map[string]string{"type": "number"},
		"currency":                 map[string]string{"type": "string"},
		"merchant":                 map[string]string{"type": "string"},
		"account_last4":            map[string]string{"type": "string"},
		"bank":                     map[string]string{"type": "string"},
		"ref_id":                   map[string]string{"type": "string"},
		"tx_type":                  map[string]string{"type": "string"},
		"suggested_ledger_account": map[string]string{"type": "string"},
		"confidence":               map[string]string{"type": "number"},
	},
	"required": []string{"date", "amount", "currency", "merchant", "tx_type", "confidence"},
}

func (p *Parser) Parse(rawText string, knownAccounts []string) (ParsedTransaction, error) {
	accountHint := ""
	if len(knownAccounts) > 0 {
		accountHint = fmt.Sprintf("\nKnown ledger accounts (choose suggested_ledger_account from these): %s",
			strings.Join(knownAccounts, ", "))
	}

	prompt := fmt.Sprintf(`Extract transaction details from this bank notification as JSON.
amount: negative for debits (money leaving account), positive for credits.
confidence: 0.0–1.0 certainty of extraction.
date format: YYYY-MM-DD.%s

Text: %s`, accountHint, rawText)

	body := map[string]interface{}{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"format": responseSchema,
		"stream": false,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return ParsedTransaction{}, err
	}

	resp, err := http.Post(p.url+"/api/chat", "application/json", bytes.NewReader(data))
	if err != nil {
		return ParsedTransaction{}, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ParsedTransaction{}, fmt.Errorf("ollama returned %d", resp.StatusCode)
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ParsedTransaction{}, fmt.Errorf("decode ollama response: %w", err)
	}

	var tx ParsedTransaction
	return tx, json.Unmarshal([]byte(result.Message.Content), &tx)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/parser/ -v
```

Expected: PASS — 2 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/parser/
git commit -m "feat(agent): Ollama parser with JSON schema enforcement"
```

---

## Task 5: Dedup engine

**Files:**
- Create: `internal/agent/dedup/dedup.go`
- Create: `internal/agent/dedup/dedup_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/agent/dedup/dedup_test.go`:

```go
package dedup

import (
	"path/filepath"
	"testing"

	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"github.com/stretchr/testify/assert"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := agentdb.Open(filepath.Join(t.TempDir(), "test.db"))
	assert.NoError(t, err)
	return db
}

func TestCheck_DuplicateByRefID(t *testing.T) {
	db := openTestDB(t)
	_ = Record(db, "REF001", "2025-05-14", "HDFC:1234", "sms", 2450)

	result := Check(db, "REF001", "2025-05-14", "HDFC:1234", 2450)
	assert.Equal(t, Duplicate, result)
}

func TestCheck_FuzzyByAmountAndDate(t *testing.T) {
	db := openTestDB(t)
	_ = Record(db, "", "2025-05-14", "HDFC:1234", "sms", 2450)

	// Same amount, same account, one day later → fuzzy
	result := Check(db, "", "2025-05-15", "HDFC:1234", 2450)
	assert.Equal(t, Fuzzy, result)
}

func TestCheck_NewTransaction(t *testing.T) {
	db := openTestDB(t)
	result := Check(db, "REF999", "2025-05-14", "HDFC:1234", 2450)
	assert.Equal(t, New, result)
}

func TestCheck_DifferentAmountIsNew(t *testing.T) {
	db := openTestDB(t)
	_ = Record(db, "", "2025-05-14", "HDFC:1234", "sms", 2450)

	result := Check(db, "", "2025-05-14", "HDFC:1234", 3000)
	assert.Equal(t, New, result)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/dedup/ -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement dedup**

Create `internal/agent/dedup/dedup.go`:

```go
package dedup

import (
	"math"
	"time"

	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"gorm.io/gorm"
)

type Result int

const (
	New       Result = iota
	Duplicate        // exact ref_id match
	Fuzzy            // same account+amount within ±1 day
)

func Check(db *gorm.DB, refID, date, account string, amount float64) Result {
	if refID != "" {
		var count int64
		db.Model(&agentdb.ImportedRef{}).Where("ref_id = ?", refID).Count(&count)
		if count > 0 {
			return Duplicate
		}
	}

	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return New
	}

	var refs []agentdb.ImportedRef
	db.Where("account = ? AND date >= ? AND date <= ?",
		account,
		t.AddDate(0, 0, -1).Format("2006-01-02"),
		t.AddDate(0, 0, 1).Format("2006-01-02"),
	).Find(&refs)

	for _, r := range refs {
		if math.Abs(r.Amount-amount) < 0.01 {
			return Fuzzy
		}
	}

	return New
}

func Record(db *gorm.DB, refID, date, account, source string, amount float64) error {
	return db.Create(&agentdb.ImportedRef{
		RefID:   refID,
		Date:    date,
		Amount:  amount,
		Account: account,
		Source:  source,
	}).Error
}
```

- [ ] **Step 4: Fix test import (add gorm import)**

Update `internal/agent/dedup/dedup_test.go` — add the gorm import that `openTestDB` needs:

```go
package dedup

import (
	"path/filepath"
	"testing"

	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/agent/dedup/ -v
```

Expected: PASS — 4 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/dedup/
git commit -m "feat(agent): dedup engine — ref-ID exact match + fuzzy date+amount"
```

---

## Task 6: Merchant rule engine

**Files:**
- Create: `internal/agent/merchant/merchant.go`
- Create: `internal/agent/merchant/merchant_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/agent/merchant/merchant_test.go`:

```go
package merchant

import (
	"os"
	"path/filepath"
	"testing"

	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := agentdb.Open(filepath.Join(t.TempDir(), "test.db"))
	assert.NoError(t, err)
	return db
}

func TestGate_UnknownMerchant(t *testing.T) {
	db := openTestDB(t)
	assert.Equal(t, NeedsApproval, Gate(db, "NewShop", 500, 0.9, 10000, 3))
}

func TestGate_KnownMerchantAutoApprove(t *testing.T) {
	db := openTestDB(t)
	db.Create(&agentdb.MerchantRule{Merchant: "Swiggy", Account: "Expenses:Food:Dining", ApproveCount: 5, AutoApprove: true})
	assert.Equal(t, AutoPost, Gate(db, "Swiggy", 500, 0.9, 10000, 3))
}

func TestGate_HighValueAlwaysApproval(t *testing.T) {
	db := openTestDB(t)
	db.Create(&agentdb.MerchantRule{Merchant: "Amazon", Account: "Expenses:Shopping", ApproveCount: 10, AutoApprove: true})
	assert.Equal(t, NeedsApproval, Gate(db, "Amazon", 15000, 0.9, 10000, 3))
}

func TestGate_LowConfidenceAlwaysApproval(t *testing.T) {
	db := openTestDB(t)
	db.Create(&agentdb.MerchantRule{Merchant: "Swiggy", Account: "Expenses:Food:Dining", ApproveCount: 5, AutoApprove: true})
	assert.Equal(t, NeedsApproval, Gate(db, "Swiggy", 500, 0.5, 10000, 3))
}

func TestRecordApproval_PromotesAfterThreshold(t *testing.T) {
	db := openTestDB(t)
	for i := 0; i < 3; i++ {
		_ = RecordApproval(db, "NewShop", "Expenses:Shopping", 3)
	}
	var rule agentdb.MerchantRule
	db.First(&rule, "merchant = ?", "NewShop")
	assert.True(t, rule.AutoApprove)
	assert.Equal(t, 3, rule.ApproveCount)
}

func TestBootstrap_SeedsFromJournal(t *testing.T) {
	dir := t.TempDir()
	journal := `
2025/04/01 Swiggy
    Expenses:Food:Dining    INR 350.00
    Assets:HDFC:Savings

2025/04/02 Amazon
    Expenses:Shopping    INR 1200.00
    Assets:HDFC:Savings
`
	journalPath := filepath.Join(dir, "main.journal")
	_ = os.WriteFile(journalPath, []byte(journal), 0644)

	db := openTestDB(t)
	err := Bootstrap(db, journalPath, 3)
	assert.NoError(t, err)

	var swiggy agentdb.MerchantRule
	assert.NoError(t, db.First(&swiggy, "merchant = ?", "Swiggy").Error)
	assert.Equal(t, "Expenses:Food:Dining", swiggy.Account)
	assert.True(t, swiggy.AutoApprove)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/merchant/ -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement merchant rule engine**

Create `internal/agent/merchant/merchant.go`:

```go
package merchant

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Decision int

const (
	AutoPost      Decision = iota
	NeedsApproval
)

func Gate(db *gorm.DB, merchantName string, amount, confidence, threshold float64, promoteAfter int) Decision {
	if confidence < 0.7 || amount >= threshold {
		return NeedsApproval
	}
	var rule agentdb.MerchantRule
	if err := db.First(&rule, "merchant = ?", merchantName).Error; err != nil {
		return NeedsApproval
	}
	if rule.AutoApprove {
		return AutoPost
	}
	return NeedsApproval
}

func RecordApproval(db *gorm.DB, merchantName, account string, promoteAfter int) error {
	var rule agentdb.MerchantRule
	db.Where(agentdb.MerchantRule{Merchant: merchantName}).FirstOrCreate(&rule)
	rule.Account = account
	rule.ApproveCount++
	if rule.ApproveCount >= promoteAfter {
		rule.AutoApprove = true
	}
	return db.Save(&rule).Error
}

var txHeaderRe = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2})\s+(.+)$`)
var postingRe = regexp.MustCompile(`^\s{4}(\S[^;]+?)\s{2,}`)

// Bootstrap scans an hledger journal file and pre-populates merchant rules
// for all payees that have expense/liability postings.
func Bootstrap(db *gorm.DB, journalPath string, promoteAfter int) error {
	f, err := os.Open(journalPath)
	if err != nil {
		return err
	}
	defer f.Close()

	type pair struct{ payee, account string }
	var pairs []pair
	var currentPayee string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := txHeaderRe.FindStringSubmatch(line); m != nil {
			currentPayee = strings.TrimSpace(m[2])
			continue
		}
		if currentPayee == "" {
			continue
		}
		if strings.TrimSpace(line) == "" {
			currentPayee = ""
			continue
		}
		if m := postingRe.FindStringSubmatch(line); m != nil {
			account := strings.Fields(strings.TrimSpace(m[1]))[0]
			if strings.HasPrefix(account, "Expenses:") || strings.HasPrefix(account, "Liabilities:") {
				pairs = append(pairs, pair{currentPayee, account})
			}
		}
	}

	for _, p := range pairs {
		rule := agentdb.MerchantRule{
			Merchant:     p.payee,
			Account:      p.account,
			ApproveCount: promoteAfter,
			AutoApprove:  true,
		}
		db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule)
	}
	return scanner.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/merchant/ -v
```

Expected: PASS — 6 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/merchant/
git commit -m "feat(agent): merchant rule engine with journal bootstrap"
```

---

## Task 7: Journal writer + Paisa sync

**Files:**
- Create: `internal/agent/journal/journal.go`
- Create: `internal/agent/journal/sync.go`
- Create: `internal/agent/journal/journal_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/agent/journal/journal_test.go`:

```go
package journal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

func TestFormat_DebitTransaction(t *testing.T) {
	tx := parser.ParsedTransaction{
		Date:                   "2025-05-14",
		Amount:                 -2450.00,
		Currency:               "INR",
		Merchant:               "Swiggy",
		AccountLast4:           "1234",
		Bank:                   "HDFC",
		RefID:                  "47291830",
		SuggestedLedgerAccount: "Expenses:Food:Dining",
	}
	entry := Format(tx, "sms", "Assets:HDFC:Savings")
	assert.Contains(t, entry, "2025/05/14 Swiggy")
	assert.Contains(t, entry, "Expenses:Food:Dining")
	assert.Contains(t, entry, "INR 2450.00")
	assert.Contains(t, entry, "Assets:HDFC:Savings")
	assert.Contains(t, entry, "; ref: 47291830")
	assert.Contains(t, entry, "; source: sms")
}

func TestFormat_CreditTransaction(t *testing.T) {
	tx := parser.ParsedTransaction{
		Date:                   "2025-05-01",
		Amount:                 50000.00,
		Currency:               "INR",
		Merchant:               "Employer",
		AccountLast4:           "1234",
		Bank:                   "HDFC",
		SuggestedLedgerAccount: "Income:Salary",
	}
	entry := Format(tx, "gmail_alert", "Assets:HDFC:Savings")
	assert.Contains(t, entry, "Income:Salary")
	assert.Contains(t, entry, "Assets:HDFC:Savings")
	assert.Contains(t, entry, "INR 50000.00")
}

func TestAppend_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	entry := "2025/05/14 Swiggy\n    Expenses:Food:Dining    INR 2450.00\n    Assets:HDFC:Savings\n\n"
	err := Append(dir, entry)
	assert.NoError(t, err)

	data, _ := os.ReadFile(filepath.Join(dir, "auto-imported.journal"))
	assert.Contains(t, string(data), "Swiggy")

	// Append again — both entries should be present
	_ = Append(dir, "2025/05/15 Amazon\n    Expenses:Shopping    INR 500.00\n    Assets:HDFC:Savings\n\n")
	data, _ = os.ReadFile(filepath.Join(dir, "auto-imported.journal"))
	assert.Contains(t, string(data), "Swiggy")
	assert.Contains(t, string(data), "Amazon")
}

func TestTriggerSync_CallsPaisaSync(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/sync" && r.Method == "POST" {
			called = true
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	err := TriggerSync(srv.URL, "")
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestFormat_DateConvertedToSlashFormat(t *testing.T) {
	tx := parser.ParsedTransaction{
		Date: "2025-05-14", Amount: -100, Currency: "INR",
		Merchant: "Test", SuggestedLedgerAccount: "Expenses:Test",
	}
	entry := Format(tx, "sms", "Assets:HDFC:Savings")
	// hledger uses YYYY/MM/DD
	assert.True(t, strings.Contains(entry, "2025/05/14"), "date must use slash format")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/journal/ -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement journal.go**

Create `internal/agent/journal/journal.go`:

```go
package journal

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/ananthakumaran/paisa/internal/agent/parser"
)

// Format produces an hledger journal entry for a parsed transaction.
// debitAccount is the bank account to use as the balancing entry (e.g. "Assets:HDFC:Savings").
func Format(tx parser.ParsedTransaction, source, debitAccount string) string {
	absAmount := math.Abs(tx.Amount)
	// Convert ISO date YYYY-MM-DD to hledger YYYY/MM/DD
	date := strings.ReplaceAll(tx.Date, "-", "/")

	var entry strings.Builder
	entry.WriteString(fmt.Sprintf("; source: %s\n", source))
	if tx.RefID != "" {
		entry.WriteString(fmt.Sprintf("; ref: %s\n", tx.RefID))
	}
	entry.WriteString(fmt.Sprintf("%s %s\n", date, tx.Merchant))

	if tx.Amount < 0 {
		// Debit: money leaves bank account → expense/liability increases
		entry.WriteString(fmt.Sprintf("    %s    %s %.2f\n", tx.SuggestedLedgerAccount, tx.Currency, absAmount))
		entry.WriteString(fmt.Sprintf("    %s\n", debitAccount))
	} else {
		// Credit: money enters bank account ← income source
		entry.WriteString(fmt.Sprintf("    %s    %s %.2f\n", debitAccount, tx.Currency, absAmount))
		entry.WriteString(fmt.Sprintf("    %s\n", tx.SuggestedLedgerAccount))
	}
	entry.WriteString("\n")
	return entry.String()
}

// Append writes entry to <journalDir>/auto-imported.journal (creates if absent).
func Append(journalDir, entry string) error {
	path := filepath.Join(journalDir, "auto-imported.journal")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(entry)
	return err
}
```

- [ ] **Step 4: Implement sync.go**

Create `internal/agent/journal/sync.go`:

```go
package journal

import (
	"fmt"
	"net/http"
)

func TriggerSync(paisaURL, apiToken string) error {
	req, err := http.NewRequest("POST", paisaURL+"/api/sync", nil)
	if err != nil {
		return err
	}
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sync request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("sync returned %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/agent/journal/ -v
```

Expected: PASS — 5 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/journal/
git commit -m "feat(agent): journal writer (hledger format) + Paisa sync trigger"
```

---

## Task 8: Telegram bot

**Files:**
- Create: `internal/agent/telegram/telegram.go`
- Create: `internal/agent/telegram/telegram_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/agent/telegram/telegram_test.go`:

```go
package telegram

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

func TestGetUpdates_ParsesMessages(t *testing.T) {
	updates := []Update{
		{UpdateID: 1, Message: &Message{Text: "hello", Chat: Chat{ID: 123}}},
		{UpdateID: 2, CallbackQuery: &CallbackQuery{ID: "cb1", Data: "approve:REF001", Message: Message{Chat: Chat{ID: 123}}}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"result": updates})
	}))
	defer srv.Close()

	b := newWithBaseURL("token123", 123, srv.URL)
	got, err := b.GetUpdates()
	assert.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, "hello", got[0].Message.Text)
	assert.Equal(t, "approve:REF001", got[1].CallbackQuery.Data)
}

func TestGetUpdates_AdvancesOffset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "offset=3")
		json.NewEncoder(w).Encode(map[string]interface{}{"result": []Update{}})
	}))
	defer srv.Close()

	b := newWithBaseURL("token123", 123, srv.URL)
	b.offset = 3
	_, _ = b.GetUpdates()
}

func TestSendApprovalCard_SendsMessage(t *testing.T) {
	var sent map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	b := newWithBaseURL("token123", 123, srv.URL)
	tx := parser.ParsedTransaction{
		Merchant: "Swiggy", Bank: "HDFC", AccountLast4: "1234",
		Amount: -2450, Currency: "INR", Date: "2025-05-14",
		SuggestedLedgerAccount: "Expenses:Food:Dining", RefID: "REF001",
	}
	err := b.SendApprovalCard(tx)
	assert.NoError(t, err)
	assert.NotNil(t, sent["reply_markup"])
	assert.Contains(t, sent["text"].(string), "Swiggy")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/telegram/ -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement Telegram bot**

Create `internal/agent/telegram/telegram.go`:

```go
package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ananthakumaran/paisa/internal/agent/parser"
)

type Chat struct {
	ID int64 `json:"id"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	Text      string `json:"text"`
	Chat      Chat   `json:"chat"`
}

type CallbackQuery struct {
	ID      string  `json:"id"`
	Data    string  `json:"data"`
	Message Message `json:"message"`
}

type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Bot struct {
	token   string
	chatID  int64
	offset  int
	baseURL string
}

func New(token string, chatID int64) *Bot {
	return &Bot{token: token, chatID: chatID, baseURL: "https://api.telegram.org"}
}

func newWithBaseURL(token string, chatID int64, baseURL string) *Bot {
	return &Bot{token: token, chatID: chatID, baseURL: baseURL}
}

func (b *Bot) GetUpdates() ([]Update, error) {
	url := fmt.Sprintf("%s/bot%s/getUpdates?offset=%d&timeout=30", b.baseURL, b.token, b.offset)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Result []Update `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	for _, u := range result.Result {
		if u.UpdateID >= b.offset {
			b.offset = u.UpdateID + 1
		}
	}
	return result.Result, nil
}

func (b *Bot) SendApprovalCard(tx parser.ParsedTransaction) error {
	absAmount := tx.Amount
	if absAmount < 0 {
		absAmount = -absAmount
	}
	text := fmt.Sprintf("*%s*  ·  %s XX%s\n%.2f %s  ·  %s\n→ _%s_ (suggested)",
		tx.Merchant, tx.Bank, tx.AccountLast4,
		absAmount, tx.Currency, tx.Date,
		tx.SuggestedLedgerAccount)

	keyboard := map[string]interface{}{
		"inline_keyboard": [][]map[string]string{{
			{"text": "✓ Post", "callback_data": "approve:" + tx.RefID},
			{"text": "✎ Edit", "callback_data": "edit:" + tx.RefID},
			{"text": "✗ Skip", "callback_data": "skip:" + tx.RefID},
		}},
	}
	return b.sendMessage(text, keyboard)
}

func (b *Bot) SendText(text string) error {
	return b.sendMessage(text, nil)
}

func (b *Bot) AnswerCallback(callbackID string) error {
	body := map[string]string{"callback_query_id": callbackID}
	data, _ := json.Marshal(body)
	resp, err := http.Post(
		fmt.Sprintf("%s/bot%s/answerCallbackQuery", b.baseURL, b.token),
		"application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (b *Bot) sendMessage(text string, keyboard interface{}) error {
	body := map[string]interface{}{
		"chat_id":    b.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	if keyboard != nil {
		body["reply_markup"] = keyboard
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(
		fmt.Sprintf("%s/bot%s/sendMessage", b.baseURL, b.token),
		"application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram sendMessage: status %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/telegram/ -v
```

Expected: PASS — 3 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/telegram/
git commit -m "feat(agent): Telegram bot — long-poll, approval cards, callback ACK"
```

---

## Task 9: Gmail adapter

**Files:**
- Create: `internal/agent/gmail/auth.go`
- Create: `internal/agent/gmail/classify.go`
- Create: `internal/agent/gmail/pdf.go`
- Create: `internal/agent/gmail/gmail.go`
- Create: `internal/agent/gmail/gmail_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/agent/gmail/gmail_test.go`:

```go
package gmail

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/api/gmail/v1"
)

func TestClassify_AlertEmail(t *testing.T) {
	msg := &gmail.Message{
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "HDFC Bank: Rs.2450 debited from your account"},
			},
		},
	}
	assert.Equal(t, AlertEmail, Classify(msg))
}

func TestClassify_StatementEmail(t *testing.T) {
	msg := &gmail.Message{
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "Your HDFC Bank e-Statement for April 2025"},
			},
			Parts: []*gmail.MessagePart{
				{MimeType: "application/pdf", Filename: "statement.pdf"},
			},
		},
	}
	assert.Equal(t, StatementEmail, Classify(msg))
}

func TestClassify_UnrelatedEmail(t *testing.T) {
	msg := &gmail.Message{
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "Your order has shipped"},
			},
		},
	}
	assert.Equal(t, UnknownEmail, Classify(msg))
}

func TestGetSubject(t *testing.T) {
	msg := &gmail.Message{
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "Test Subject"},
				{Name: "From", Value: "bank@hdfc.com"},
			},
		},
	}
	assert.Equal(t, "Test Subject", getSubject(msg))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/gmail/ -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement classify.go**

Create `internal/agent/gmail/classify.go`:

```go
package gmail

import (
	"strings"

	"google.golang.org/api/gmail/v1"
)

type EmailType int

const (
	UnknownEmail   EmailType = iota
	AlertEmail               // transaction notification email, parse body
	StatementEmail           // monthly statement with PDF attachment
)

var alertKeywords = []string{
	"debited", "credited", "transaction alert", "payment", "spent",
	"rs.", "inr", "amount", "debit", "credit",
}

var statementKeywords = []string{"statement", "e-statement", "account summary", "monthly statement"}

func Classify(msg *gmail.Message) EmailType {
	subject := strings.ToLower(getSubject(msg))

	hasPDF := false
	if msg.Payload != nil {
		for _, part := range msg.Payload.Parts {
			if part.MimeType == "application/pdf" || strings.HasSuffix(strings.ToLower(part.Filename), ".pdf") {
				hasPDF = true
				break
			}
		}
	}

	for _, kw := range statementKeywords {
		if strings.Contains(subject, kw) && hasPDF {
			return StatementEmail
		}
	}

	for _, kw := range alertKeywords {
		if strings.Contains(subject, kw) {
			return AlertEmail
		}
	}

	return UnknownEmail
}

func getSubject(msg *gmail.Message) string {
	if msg.Payload == nil {
		return ""
	}
	for _, h := range msg.Payload.Headers {
		if h.Name == "Subject" {
			return h.Value
		}
	}
	return ""
}
```

- [ ] **Step 4: Implement auth.go**

Create `internal/agent/gmail/auth.go`:

```go
package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googlemail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// NewService creates an authenticated Gmail service. On first run, opens
// a browser for OAuth2 consent and saves the token to tokenFile.
func NewService(ctx context.Context, credentialsFile, tokenFile string) (*googlemail.Service, error) {
	creds, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	oauthConfig, err := google.ConfigFromJSON(creds, googlemail.GmailReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	tok, err := loadToken(tokenFile)
	if err != nil {
		tok, err = getTokenFromWeb(oauthConfig, tokenFile)
		if err != nil {
			return nil, err
		}
	}

	client := oauthConfig.Client(ctx, tok)
	return googlemail.NewService(ctx, option.WithHTTPClient(client))
}

func loadToken(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	return tok, json.NewDecoder(f).Decode(tok)
}

func getTokenFromWeb(config *oauth2.Config, tokenFile string) (*oauth2.Token, error) {
	config.RedirectURL = "urn:ietf:wg:oauth:2.0:oob"
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Open this URL in your browser:\n%v\n\nEnter authorization code: ", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, fmt.Errorf("read auth code: %w", err)
	}

	tok, err := config.Exchange(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("exchange token: %w", err)
	}

	f, err := os.OpenFile(tokenFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return tok, json.NewEncoder(f).Encode(tok)
}
```

- [ ] **Step 5: Implement pdf.go**

Create `internal/agent/gmail/pdf.go`:

```go
package gmail

import (
	"bytes"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractPDFText extracts plain text from PDF bytes.
func ExtractPDFText(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		content, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
```

- [ ] **Step 6: Implement gmail.go**

Create `internal/agent/gmail/gmail.go`:

```go
package gmail

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/api/gmail/v1"
)

type RawEmail struct {
	ID          string
	Subject     string
	Body        string       // plain text body for AlertEmail
	PDFText     string       // extracted PDF text for StatementEmail
	Type        EmailType
	ReceivedAt  time.Time
}

type Poller struct {
	svc          *gmail.Service
	labels       []string
	lastChecked  int64 // Unix milliseconds
}

func NewPoller(svc *gmail.Service, labels []string) *Poller {
	return &Poller{
		svc:         svc,
		labels:      labels,
		lastChecked: time.Now().Add(-24 * time.Hour).UnixMilli(),
	}
}

func (p *Poller) Poll(ctx context.Context) ([]RawEmail, error) {
	query := buildQuery(p.labels, p.lastChecked)
	listRes, err := p.svc.Users.Messages.List("me").Q(query).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	var emails []RawEmail
	for _, m := range listRes.Messages {
		msg, err := p.svc.Users.Messages.Get("me", m.Id).Format("full").Context(ctx).Do()
		if err != nil {
			log.Warnf("gmail: get message %s: %v", m.Id, err)
			continue
		}

		emailType := Classify(msg)
		if emailType == UnknownEmail {
			continue
		}

		raw := RawEmail{
			ID:         msg.Id,
			Subject:    getSubject(msg),
			Type:       emailType,
			ReceivedAt: time.UnixMilli(msg.InternalDate),
		}

		if emailType == AlertEmail {
			raw.Body = extractBody(msg)
		} else {
			raw.PDFText = extractPDFAttachment(msg)
		}

		if raw.Body != "" || raw.PDFText != "" {
			emails = append(emails, raw)
		}
	}

	p.lastChecked = time.Now().UnixMilli()
	return emails, nil
}

func buildQuery(labels []string, afterMs int64) string {
	after := time.UnixMilli(afterMs).Format("2006/01/02")
	q := "after:" + after
	if len(labels) > 0 {
		labelParts := make([]string, len(labels))
		for i, l := range labels {
			labelParts[i] = "label:" + l
		}
		q = "(" + strings.Join(labelParts, " OR ") + ") " + q
	}
	return q
}

func extractBody(msg *gmail.Message) string {
	if msg.Payload == nil {
		return ""
	}
	return extractPartText(msg.Payload)
}

func extractPartText(part *gmail.MessagePart) string {
	if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			return ""
		}
		return string(data)
	}
	for _, p := range part.Parts {
		if t := extractPartText(p); t != "" {
			return t
		}
	}
	return ""
}

func extractPDFAttachment(msg *gmail.Message) string {
	if msg.Payload == nil {
		return ""
	}
	for _, part := range msg.Payload.Parts {
		if part.MimeType == "application/pdf" || strings.HasSuffix(strings.ToLower(part.Filename), ".pdf") {
			if part.Body == nil || part.Body.Data == "" {
				continue
			}
			data, err := base64.URLEncoding.DecodeString(part.Body.Data)
			if err != nil {
				continue
			}
			text, err := ExtractPDFText(data)
			if err != nil {
				log.Warnf("gmail: PDF extract failed for %s: %v", part.Filename, err)
				continue
			}
			return text
		}
	}
	return ""
}
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
go test ./internal/agent/gmail/ -v
```

Expected: PASS — 4 tests (classify + getSubject tests; auth and poller require live credentials so not unit-tested).

- [ ] **Step 8: Commit**

```bash
git add internal/agent/gmail/
git commit -m "feat(agent): Gmail adapter — OAuth2 auth, email classify, PDF text extraction"
```

---

## Task 10: Pipeline orchestration

**Files:**
- Create: `internal/agent/pipeline/pipeline.go`
- Create: `internal/agent/pipeline/pipeline_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/agent/pipeline/pipeline_test.go`:

```go
package pipeline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	agentconfig "github.com/ananthakumaran/paisa/internal/agent/config"
	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

func setup(t *testing.T) (*Pipeline, string, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	db, _ := agentdb.Open(filepath.Join(dir, "agent.db"))

	syncCalled := false
	paisaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/sync" {
			syncCalled = true
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(func() {
		paisaSrv.Close()
		_ = syncCalled // suppress unused warning
	})

	// Mock Ollama returns a known transaction
	mockTx := parser.ParsedTransaction{
		Date: "2025-05-14", Amount: -2450, Currency: "INR",
		Merchant: "Swiggy", Bank: "HDFC", AccountLast4: "1234",
		RefID: "REF001", TxType: "debit",
		SuggestedLedgerAccount: "Expenses:Food:Dining",
		Confidence: 0.95,
	}
	ollamaContent, _ := json.Marshal(mockTx)
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": map[string]string{"content": string(ollamaContent)},
		})
	}))
	t.Cleanup(ollamaSrv.Close)

	cfg := agentconfig.DefaultConfig()
	cfg.Paisa.URL = paisaSrv.URL
	cfg.Paisa.JournalDir = dir
	cfg.Ollama.URL = ollamaSrv.URL
	cfg.Accounts = map[string]string{"HDFC:1234": "Assets:HDFC:Savings"}

	p := New(db, cfg)
	return p, dir, paisaSrv
}

func TestProcess_KnownMerchantAutoPost(t *testing.T) {
	p, dir, _ := setup(t)
	// Pre-populate merchant as known
	p.db.Create(&agentdb.MerchantRule{Merchant: "Swiggy", Account: "Expenses:Food:Dining", ApproveCount: 5, AutoApprove: true})

	action, err := p.Process("HDFC: Rs.2450 debited at SWIGGY Ref REF001", "sms")
	assert.NoError(t, err)
	assert.Equal(t, ActionPosted, action)

	data, _ := os.ReadFile(filepath.Join(dir, "auto-imported.journal"))
	assert.Contains(t, string(data), "Swiggy")
}

func TestProcess_UnknownMerchantNeedsApproval(t *testing.T) {
	p, _, _ := setup(t)
	// No merchant rule → needs approval; Telegram send will fail (no real bot) but we check action
	action, err := p.Process("HDFC: Rs.2450 debited at SWIGGY Ref REF001", "sms")
	// Telegram send fails (no real bot configured in test) but pipeline returns NeedsApproval
	assert.Equal(t, ActionPendingApproval, action)
	_ = err // Telegram send error is expected in test
}

func TestProcess_DuplicateSkipped(t *testing.T) {
	p, dir, _ := setup(t)
	p.db.Create(&agentdb.MerchantRule{Merchant: "Swiggy", Account: "Expenses:Food:Dining", ApproveCount: 5, AutoApprove: true})

	_, _ = p.Process("HDFC: Rs.2450 debited at SWIGGY Ref REF001", "sms")
	action, err := p.Process("HDFC: Rs.2450 debited at SWIGGY Ref REF001", "gmail_alert")
	assert.NoError(t, err)
	assert.Equal(t, ActionDuplicate, action)

	// Only one entry in journal (not two)
	data, _ := os.ReadFile(filepath.Join(dir, "auto-imported.journal"))
	assert.Equal(t, 1, strings.Count(string(data), "2025/05/14 Swiggy"))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/pipeline/ -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement pipeline**

Create `internal/agent/pipeline/pipeline.go`:

```go
package pipeline

import (
	"fmt"
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
	ActionPendingApproval
	ActionDuplicate
	ActionSkipped
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

// Process parses rawText from source, deduplicates, gates on merchant rules,
// and either auto-posts or sends a Telegram approval card.
func (p *Pipeline) Process(rawText, source string) (Action, error) {
	// Build known account list for LLM hint
	accounts := make([]string, 0, len(p.cfg.Accounts))
	for _, v := range p.cfg.Accounts {
		accounts = append(accounts, v)
	}

	tx, err := p.parser.Parse(rawText, accounts)
	if err != nil {
		return ActionSkipped, fmt.Errorf("parse: %w", err)
	}

	// Dedup check
	result := dedup.Check(p.db, tx.RefID, tx.Date, tx.Bank+":"+tx.AccountLast4, tx.Amount)
	if result == dedup.Duplicate {
		log.Infof("pipeline: duplicate skipped ref=%s", tx.RefID)
		return ActionDuplicate, nil
	}
	if result == dedup.Fuzzy {
		log.Warnf("pipeline: fuzzy duplicate, sending for approval ref=%s", tx.RefID)
		return ActionPendingApproval, p.sendApproval(tx)
	}

	// Merchant gate
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

// HandleCallback processes a Telegram inline button callback.
// data format: "approve:<refID>" | "skip:<refID>" | "edit:<refID>"
func (p *Pipeline) HandleCallback(callbackID, data string) {
	_ = p.bot.AnswerCallback(callbackID)

	var action, refID string
	fmt.Sscanf(data, "%s", &data) // trim whitespace
	if len(data) > 7 {
		action = data[:data[7:][0]&0] // placeholder — parse below
	}
	// Parse "action:refID"
	for i, c := range data {
		if c == ':' {
			action = data[:i]
			refID = data[i+1:]
			break
		}
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
		// The next message from the user will be handled as a free-text reply.
		// Store pending edit so the next plain-text message is treated as account correction.
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/pipeline/ -v
```

Expected: PASS — 3 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/pipeline/
git commit -m "feat(agent): pipeline orchestration — parse, dedup, merchant gate, post/approve"
```

---

## Task 11: Wire main.go + goroutines

**Files:**
- Modify: `cmd/paisa-agent/main.go`

- [ ] **Step 1: Replace main.go with full wiring**

Overwrite `cmd/paisa-agent/main.go`:

```go
package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"time"

	agentconfig "github.com/ananthakumaran/paisa/internal/agent/config"
	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"github.com/ananthakumaran/paisa/internal/agent/gmail"
	"github.com/ananthakumaran/paisa/internal/agent/merchant"
	"github.com/ananthakumaran/paisa/internal/agent/pipeline"
	log "github.com/sirupsen/logrus"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "", "path to paisa-agent.yaml")
	flag.Parse()

	if cfgPath == "" {
		home, _ := os.UserHomeDir()
		cfgPath = filepath.Join(home, ".config", "paisa-agent", "paisa-agent.yaml")
	}

	cfg, err := agentconfig.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	dbPath := filepath.Join(os.Getenv("HOME"), ".local", "share", "paisa-agent", "agent.db")
	_ = os.MkdirAll(filepath.Dir(dbPath), 0755)

	db, err := agentdb.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	// Bootstrap merchant rules from main journal on first run
	if cfg.Paisa.JournalDir != "" {
		journalPath := filepath.Join(cfg.Paisa.JournalDir, "main.journal")
		if err := merchant.Bootstrap(db, journalPath, cfg.MerchantRules.PromoteAfterApprovals); err != nil {
			log.Warnf("bootstrap merchant rules: %v", err)
		}
	}

	pipe := pipeline.New(db, cfg)

	// Goroutine 1: Telegram long-poll (SMS relay + callback handling)
	go func() {
		log.Info("telegram poller started")
		for {
			updates, err := pipe.Bot().GetUpdates()
			if err != nil {
				log.Warnf("telegram getUpdates: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			for _, u := range updates {
				if u.CallbackQuery != nil {
					pipe.HandleCallback(u.CallbackQuery.ID, u.CallbackQuery.Data)
				} else if u.Message != nil && u.Message.Text != "" {
					action, err := pipe.Process(u.Message.Text, "sms")
					if err != nil {
						log.Warnf("pipeline sms: %v", err)
					}
					log.Infof("telegram msg processed: action=%d", action)
				}
			}
		}
	}()

	// Goroutine 2: Gmail poller
	if cfg.Gmail.CredentialsFile != "" {
		go func() {
			log.Info("gmail poller started")
			tokenFile := filepath.Join(filepath.Dir(cfgPath), "gmail-token.json")
			svc, err := gmail.NewService(context.Background(), cfg.Gmail.CredentialsFile, tokenFile)
			if err != nil {
				log.Fatalf("gmail auth: %v", err)
			}
			poller := gmail.NewPoller(svc, cfg.Gmail.Labels)
			interval := time.Duration(cfg.Gmail.PollIntervalSeconds) * time.Second

			for {
				emails, err := poller.Poll(context.Background())
				if err != nil {
					log.Warnf("gmail poll: %v", err)
				} else {
					for _, email := range emails {
						rawText := email.Body
						source := "gmail_alert"
						if email.Type == gmail.StatementEmail {
							rawText = email.PDFText
							source = "gmail_statement"
						}
						if rawText == "" {
							continue
						}
						if _, err := pipe.Process(rawText, source); err != nil {
							log.Warnf("pipeline gmail: %v", err)
						}
					}
				}
				time.Sleep(interval)
			}
		}()
	}

	log.Info("paisa-agent running")
	select {}
}
```

- [ ] **Step 2: Add Bot() accessor to Pipeline**

Add this method to `internal/agent/pipeline/pipeline.go`:

```go
// Bot returns the Telegram bot for use by the main loop.
func (p *Pipeline) Bot() *telegram.Bot {
	return p.bot
}
```

- [ ] **Step 3: Build and verify**

```bash
go build ./cmd/paisa-agent/
```

Expected: builds cleanly.

- [ ] **Step 4: Run all agent tests**

```bash
go test ./internal/agent/... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/paisa-agent/main.go internal/agent/pipeline/pipeline.go
git commit -m "feat(agent): wire main loop — Gmail + Telegram goroutines + merchant bootstrap"
```

---

## Task 12: launchd service + setup docs

**Files:**
- Create: `scripts/paisa-agent-install.sh`
- Create: `scripts/paisa-agent.plist.tmpl`

- [ ] **Step 1: Create the plist template**

Create `scripts/paisa-agent.plist.tmpl`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.paisa.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>AGENT_BINARY_PATH</string>
        <string>--config</string>
        <string>AGENT_CONFIG_PATH</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>HOME_DIR/.local/share/paisa-agent/agent.log</string>
    <key>StandardErrorPath</key>
    <string>HOME_DIR/.local/share/paisa-agent/agent.log</string>
</dict>
</plist>
```

- [ ] **Step 2: Create install script**

Create `scripts/paisa-agent-install.sh`:

```bash
#!/bin/bash
set -e

BINARY=$(which paisa-agent 2>/dev/null || echo "$(pwd)/paisa-agent")
CONFIG="$HOME/.config/paisa-agent/paisa-agent.yaml"
PLIST="$HOME/Library/LaunchAgents/com.paisa.agent.plist"
LOG_DIR="$HOME/.local/share/paisa-agent"

if [ ! -f "$BINARY" ]; then
  echo "paisa-agent binary not found. Run: go build ./cmd/paisa-agent/ first."
  exit 1
fi

mkdir -p "$LOG_DIR"
mkdir -p "$(dirname "$PLIST")"

TMPL="$(dirname "$0")/paisa-agent.plist.tmpl"
sed \
  -e "s|AGENT_BINARY_PATH|$BINARY|g" \
  -e "s|AGENT_CONFIG_PATH|$CONFIG|g" \
  -e "s|HOME_DIR|$HOME|g" \
  "$TMPL" > "$PLIST"

launchctl load "$PLIST"
echo "paisa-agent installed and started."
echo "Logs: $LOG_DIR/agent.log"
echo ""
echo "Next steps:"
echo "  1. Edit $CONFIG with your Paisa URL, Ollama model, Telegram token, and journal_dir"
echo "  2. Run: paisa-agent --config $CONFIG  (first run will prompt Gmail OAuth2)"
echo "  3. Add 'include auto-imported.journal' to your main Paisa journal file"
echo "  4. Set up iPhone Shortcut: Automation → 'When message from [HDFC-xxxx, ICICI-xxxx]'"
echo "     Action: URL Session POST to https://api.telegram.org/bot<TOKEN>/sendMessage"
echo "     Body: {\"chat_id\": <YOUR_CHAT_ID>, \"text\": \"Shortcut.input.Messages.last\"}"
```

- [ ] **Step 3: Make it executable**

```bash
chmod +x scripts/paisa-agent-install.sh
```

- [ ] **Step 4: Commit**

```bash
git add scripts/paisa-agent-install.sh scripts/paisa-agent.plist.tmpl
git commit -m "feat(agent): launchd install script and plist template"
```

---

## Task 13: Fix HandleCallback data parsing bug

The `HandleCallback` implementation in Task 10 has a bug — the action:refID parsing uses dead code. Fix it.

**Files:**
- Modify: `internal/agent/pipeline/pipeline.go`

- [ ] **Step 1: Write a test for the callback parser**

Add to `internal/agent/pipeline/pipeline_test.go`:

```go
func TestParseCallback(t *testing.T) {
	action, refID := parseCallback("approve:REF001")
	assert.Equal(t, "approve", action)
	assert.Equal(t, "REF001", refID)

	action, refID = parseCallback("skip:REF-XYZ-99")
	assert.Equal(t, "skip", action)
	assert.Equal(t, "REF-XYZ-99", refID)

	action, refID = parseCallback("badformat")
	assert.Equal(t, "", action)
	assert.Equal(t, "", refID)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/pipeline/ -run TestParseCallback -v
```

Expected: FAIL.

- [ ] **Step 3: Replace the buggy parsing in pipeline.go**

Replace the `HandleCallback` method and add `parseCallback`:

```go
func parseCallback(data string) (action, refID string) {
	for i, c := range data {
		if c == ':' {
			return data[:i], data[i+1:]
		}
	}
	return "", ""
}

func (p *Pipeline) HandleCallback(callbackID, data string) {
	_ = p.bot.AnswerCallback(callbackID)
	action, refID := parseCallback(data)
	if action == "" {
		log.Warnf("pipeline: malformed callback data: %s", data)
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
		_ = p.bot.SendText("✓ Posted")

	case "skip":
		_ = dedup.Record(p.db, tx.RefID, tx.Date, tx.Bank+":"+tx.AccountLast4, "skipped", tx.Amount)
		p.pending.Delete(refID)
		_ = p.bot.SendText("✗ Skipped")

	case "edit":
		_ = p.bot.SendText("Send the correct account name (e.g. Expenses:Transport:Fuel):")
		p.pending.Store(refID+"_edit", tx)
	}
}
```

- [ ] **Step 4: Run all pipeline tests**

```bash
go test ./internal/agent/pipeline/ -v
```

Expected: PASS — all tests including TestParseCallback.

- [ ] **Step 5: Run full test suite**

```bash
go test ./internal/agent/... -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/pipeline/pipeline.go internal/agent/pipeline/pipeline_test.go
git commit -m "fix(agent): callback data parser — replace dead code with clean parseCallback()"
```

---

## Task 14: Statement bulk parsing + reconciliation report

A PDF statement has many rows. The `Process()` function takes one raw text blob and calls `Parse()` which returns one transaction. For statements, we need `ParseMultiple()` and a reconciliation summary.

**Files:**
- Modify: `internal/agent/parser/parser.go`
- Modify: `internal/agent/parser/parser_test.go`
- Modify: `internal/agent/pipeline/pipeline.go`
- Modify: `internal/agent/pipeline/pipeline_test.go`

- [ ] **Step 1: Write failing test for ParseMultiple**

Add to `internal/agent/parser/parser_test.go`:

```go
func TestParseMultiple_ExtractsManyTransactions(t *testing.T) {
	mockTxns := []ParsedTransaction{
		{Date: "2025-05-01", Amount: -500, Currency: "INR", Merchant: "Swiggy", RefID: "R1", TxType: "debit", Confidence: 0.9},
		{Date: "2025-05-03", Amount: -1200, Currency: "INR", Merchant: "Amazon", RefID: "R2", TxType: "debit", Confidence: 0.95},
	}
	content, _ := json.Marshal(mockTxns)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": map[string]string{"content": string(content)},
		})
	}))
	defer srv.Close()

	p := New(srv.URL, "gemma3:12b")
	txns, err := p.ParseMultiple("... PDF statement text ...", nil)
	assert.NoError(t, err)
	assert.Len(t, txns, 2)
	assert.Equal(t, "Swiggy", txns[0].Merchant)
	assert.Equal(t, "Amazon", txns[1].Merchant)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/parser/ -run TestParseMultiple -v
```

Expected: FAIL.

- [ ] **Step 3: Add ParseMultiple to parser.go**

Add this method after `Parse()` in `internal/agent/parser/parser.go`:

```go
var multiSchema = map[string]interface{}{
	"type": "array",
	"items": responseSchema,
}

// ParseMultiple extracts all transactions from a multi-row text (e.g. PDF statement).
func (p *Parser) ParseMultiple(rawText string, knownAccounts []string) ([]ParsedTransaction, error) {
	accountHint := ""
	if len(knownAccounts) > 0 {
		accountHint = fmt.Sprintf("\nKnown ledger accounts: %s", strings.Join(knownAccounts, ", "))
	}

	prompt := fmt.Sprintf(`Extract ALL transactions from this bank statement as a JSON array.
Each element: date (YYYY-MM-DD), amount (negative=debit, positive=credit), currency, merchant, account_last4, bank, ref_id, tx_type, suggested_ledger_account, confidence.%s

Statement text:
%s`, accountHint, rawText)

	body := map[string]interface{}{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"format": multiSchema,
		"stream": false,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(p.url+"/api/chat", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ollama returned %d", resp.StatusCode)
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}

	var txns []ParsedTransaction
	return txns, json.Unmarshal([]byte(result.Message.Content), &txns)
}
```

- [ ] **Step 4: Add ProcessStatement to pipeline.go**

Add this method to `internal/agent/pipeline/pipeline.go`:

```go
// ProcessStatement runs a full statement through the pipeline:
// parses all rows, deduplicates, posts gaps, returns reconciliation counts.
func (p *Pipeline) ProcessStatement(pdfText string) (gaps, duplicates int, err error) {
	accounts := make([]string, 0, len(p.cfg.Accounts))
	for _, v := range p.cfg.Accounts {
		accounts = append(accounts, v)
	}

	txns, err := p.parser.ParseMultiple(pdfText, accounts)
	if err != nil {
		return 0, 0, fmt.Errorf("parse statement: %w", err)
	}

	for _, tx := range txns {
		result := dedup.Check(p.db, tx.RefID, tx.Date, tx.Bank+":"+tx.AccountLast4, tx.Amount)
		if result == dedup.Duplicate {
			duplicates++
			continue
		}
		if err := p.post(tx, "gmail_statement"); err != nil {
			log.Warnf("pipeline: statement post failed: %v", err)
			continue
		}
		gaps++
	}

	summary := fmt.Sprintf("Statement reconciled: %d gaps filled · %d already imported", gaps, duplicates)
	_ = p.bot.SendText(summary)
	return gaps, duplicates, nil
}
```

- [ ] **Step 5: Update Gmail goroutine in main.go to call ProcessStatement for statement emails**

In `cmd/paisa-agent/main.go`, update the email processing in the Gmail goroutine:

```go
for _, email := range emails {
    if email.Type == gmail.StatementEmail {
        gaps, dups, err := pipe.ProcessStatement(email.PDFText)
        if err != nil {
            log.Warnf("pipeline statement: %v", err)
        } else {
            log.Infof("statement processed: %d gaps, %d dups", gaps, dups)
        }
    } else {
        if email.Body == "" {
            continue
        }
        if _, err := pipe.Process(email.Body, "gmail_alert"); err != nil {
            log.Warnf("pipeline gmail: %v", err)
        }
    }
}
```

- [ ] **Step 6: Run all tests**

```bash
go test ./internal/agent/... -v
```

Expected: all tests pass.

- [ ] **Step 7: Build final binary**

```bash
go build ./cmd/paisa-agent/
```

Expected: builds cleanly.

- [ ] **Step 8: Commit**

```bash
git add internal/agent/parser/ internal/agent/pipeline/ cmd/paisa-agent/main.go
git commit -m "feat(agent): statement bulk parsing + reconciliation summary via Telegram"
```

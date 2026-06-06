# Regex Parser Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a deterministic regex-based SMS parser that runs before Ollama and handles all known bank message formats via YAML-configurable rules; falls through to LLM for anything it can't match.

**Architecture:** `RegexParse(msg, rules)` tries 7 date patterns + Rs/INR amount extraction + YAML-driven source/merchant matching. `Parser.Parse()` calls `RegexParse` first; on miss falls through to Ollama unchanged. `Config.Accounts` map is removed — all source account knowledge lives in `parser_rules.sources`.

**Tech Stack:** Go stdlib `regexp`, `strings`, `strconv`, `time`; `gopkg.in/yaml.v3` (already in use); `testify/assert` for tests. Worktree: `.worktrees/feat/ingestion-agent/`.

---

### Task 1: Update Config + fix pipeline compile

**Files:**
- Modify: `internal/agent/config/config.go`
- Modify: `internal/agent/pipeline/pipeline.go`
- Modify: `internal/agent/pipeline/pipeline_test.go`

- [ ] **Step 1: Replace Accounts map with ParserRules in config.go**

Full replacement of `internal/agent/config/config.go`:

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
	ParserRules   ParserRules         `yaml:"parser_rules"`
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
```

- [ ] **Step 2: Fix pipeline.go compile errors (cfg.Accounts removed)**

In `internal/agent/pipeline/pipeline.go`, make these three minimal fixes so it compiles:

Replace the accounts-building block in `Process()`:
```go
// was: accounts := make([]string, 0, len(p.cfg.Accounts))
//      for _, v := range p.cfg.Accounts { accounts = append(accounts, v) }
accounts := make([]string, 0, len(p.cfg.ParserRules.Sources))
for _, s := range p.cfg.ParserRules.Sources {
    accounts = append(accounts, s.Account)
}
```

Same replacement in `ProcessStatement()`.

Replace `resolveAccount`:
```go
func (p *Pipeline) resolveAccount(bank, last4 string) string {
	if last4 != "" {
		for _, s := range p.cfg.ParserRules.Sources {
			for _, c := range s.Contains {
				if strings.HasSuffix(c, last4) {
					return s.Account
				}
			}
		}
	}
	return "Assets:" + bank + ":XX" + last4
}
```

- [ ] **Step 3: Fix pipeline_test.go compile error**

In `internal/agent/pipeline/pipeline_test.go`, remove line 54:
```go
// remove this line — Accounts field no longer exists:
cfg.Accounts = map[string]string{"HDFC:1234": "Assets:HDFC:Savings"}
```

- [ ] **Step 4: Verify everything compiles and tests still pass**

```bash
cd .worktrees/feat/ingestion-agent && go test ./internal/agent/... 2>&1
```
Expected: all packages pass (no compile errors, no test failures).

- [ ] **Step 5: Commit**

```bash
git add internal/agent/config/config.go internal/agent/pipeline/pipeline.go internal/agent/pipeline/pipeline_test.go
git commit -m "feat(config): replace accounts map with ParserRules; fix pipeline compile"
```

---

### Task 2: Add SourceAccount to ParsedTransaction

**Files:**
- Modify: `internal/agent/parser/parser.go` (one field only)

- [ ] **Step 1: Add the field to ParsedTransaction**

In `internal/agent/parser/parser.go`, add `SourceAccount` to the struct:

```go
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
	SourceAccount          string  `json:"source_account,omitempty"`
}
```

- [ ] **Step 2: Verify compile**

```bash
cd .worktrees/feat/ingestion-agent && go build ./... 2>&1
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/agent/parser/parser.go
git commit -m "feat(parser): add SourceAccount field to ParsedTransaction"
```

---

### Task 3: Write failing tests for regex_parser

**Files:**
- Create: `internal/agent/parser/regex_parser_test.go`

- [ ] **Step 1: Create the test file**

```go
package parser

import (
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/stretchr/testify/assert"
)

var testRules = config.ParserRules{
	DayParts: config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20},
	Merchants: []config.MerchantPattern{
		{Keyword: "swiggy", Description: "Food: {day_part}", Account: "Expenses:Food:Hyd:Swiggy"},
		{Keyword: "zomato", Description: "Food: {day_part}", Account: "Expenses:Food:Hyd:Zomato"},
		{Keyword: "zepto", Description: "Groceries", Account: "Expenses:Groceries:Hyd"},
		{Keyword: "", Description: "Misc", Account: "Expenses:Misc:Hyd"},
	},
	Sources: []config.SourceRule{
		{ID: "fi_upi", Contains: []string{"debited via UPI on"}, Account: "Assets:Checking:FI5687"},
		{ID: "axis", Contains: []string{"A/c no. XX6386"}, Account: "Assets:Checking:AXIS6386"},
		{ID: "cc_fk", Contains: []string{"Card no. XX8860"}, Account: "Liabilities:CreditCard:FK8860"},
		{
			ID:          "canara_loan",
			Contains:    []string{"XXXXX21343", "Canara Bank", "Loan Drawdown"},
			Account:     "Assets:Checking:CANA1343",
			DestAccount: "Liabilities:Loan:CANAHL1090",
			Description: "EMI: Home Loan",
		},
	},
}

// --- parseDate ---

func TestParseDate_Pattern1_DMYYYY(t *testing.T) {
	date, ok := parseDate("INR 100 debited via UPI on 14-05-2025 at Swiggy")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_Pattern2_DMYY(t *testing.T) {
	date, ok := parseDate("Card no. XX8860 INR 500.00 on 14-05-25")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_Pattern3_MonNameDY(t *testing.T) {
	date, ok := parseDate("on May 14, 2025 amount INR 100")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_Pattern4_DDMONYY(t *testing.T) {
	date, ok := parseDate("A/c no. XX6386 debited Rs.500 on 14MAY25")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_Pattern5_DMonY(t *testing.T) {
	date, ok := parseDate("on 14 MAY,2025 Rs 100 debited")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_Pattern6_SlashDMYYYY(t *testing.T) {
	date, ok := parseDate("XXXXX21343 Canara Bank Loan Drawdown INR 45000 on 01/05/2025")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-01", date)
}

func TestParseDate_Pattern7_FullMonthDY(t *testing.T) {
	date, ok := parseDate("Debited May 14, 2025 INR 100")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_NoMatch(t *testing.T) {
	_, ok := parseDate("no date in this message")
	assert.False(t, ok)
}

// --- parseAmount ---

func TestParseAmount_RsWithComma(t *testing.T) {
	amt, ok := parseAmount("Rs 2,450.00 debited via UPI")
	assert.True(t, ok)
	assert.Equal(t, 2450.0, amt)
}

func TestParseAmount_RsDot(t *testing.T) {
	amt, ok := parseAmount("Rs.500.00 debited")
	assert.True(t, ok)
	assert.Equal(t, 500.0, amt)
}

func TestParseAmount_INRWithComma(t *testing.T) {
	amt, ok := parseAmount("INR 1,200.50 spent on card")
	assert.True(t, ok)
	assert.Equal(t, 1200.50, amt)
}

func TestParseAmount_INRNoDecimal(t *testing.T) {
	amt, ok := parseAmount("INR 45000 on 01/05/2025")
	assert.True(t, ok)
	assert.Equal(t, 45000.0, amt)
}

func TestParseAmount_NoMatch(t *testing.T) {
	_, ok := parseAmount("no amount here")
	assert.False(t, ok)
}

// --- parseDayPart ---

func TestParseDayPart_Breakfast(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Breakfast", parseDayPart("transaction at 10:59:00", dp))
}

func TestParseDayPart_LunchBoundaryLow(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Lunch", parseDayPart("transaction at 11:00", dp))
}

func TestParseDayPart_LunchBoundaryHigh(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Lunch", parseDayPart("transaction at 14:59", dp))
}

func TestParseDayPart_DinnerBoundaryLow(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Dinner", parseDayPart("transaction at 15:00", dp))
}

func TestParseDayPart_DinnerBoundaryHigh(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Dinner", parseDayPart("transaction at 19:59", dp))
}

func TestParseDayPart_EveningSnack(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Evening Snack", parseDayPart("transaction at 20:00", dp))
}

func TestParseDayPart_NoTime(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Meal", parseDayPart("no timestamp in message", dp))
}

// --- matchMerchant ---

func TestMatchMerchant_Swiggy_DayPart(t *testing.T) {
	desc, acct := matchMerchant("debited at SWIGGY 13:30", testRules.Merchants, "Lunch")
	assert.Equal(t, "Food: Lunch", desc)
	assert.Equal(t, "Expenses:Food:Hyd:Swiggy", acct)
}

func TestMatchMerchant_CaseInsensitive(t *testing.T) {
	desc, _ := matchMerchant("ZOMATO ORDER", testRules.Merchants, "Dinner")
	assert.Equal(t, "Food: Dinner", desc)
}

func TestMatchMerchant_Fallback(t *testing.T) {
	desc, acct := matchMerchant("debited at Unknown Store 13:00", testRules.Merchants, "Lunch")
	assert.Equal(t, "Misc", desc)
	assert.Equal(t, "Expenses:Misc:Hyd", acct)
}

// --- matchSource ---

func TestMatchSource_SingleContains(t *testing.T) {
	src, ok := matchSource("INR 100 debited via UPI on 14-05-2025", testRules.Sources)
	assert.True(t, ok)
	assert.Equal(t, "fi_upi", src.ID)
}

func TestMatchSource_MultiContainsAllMatch(t *testing.T) {
	src, ok := matchSource("XXXXX21343 Canara Bank Loan Drawdown INR 45000 on 01/05/2025", testRules.Sources)
	assert.True(t, ok)
	assert.Equal(t, "canara_loan", src.ID)
}

func TestMatchSource_MultiContainsPartialMiss(t *testing.T) {
	// Has two of three required strings — must not match
	_, ok := matchSource("XXXXX21343 Canara Bank credit INR 45000", testRules.Sources)
	assert.False(t, ok)
}

func TestMatchSource_NoMatch(t *testing.T) {
	_, ok := matchSource("some unrecognised bank message", testRules.Sources)
	assert.False(t, ok)
}

// --- RegexParse integration ---

func TestRegexParse_FiUPI_Swiggy_Lunch(t *testing.T) {
	msg := "INR 2,450.00 debited via UPI on 14-05-2025 13:30 at SWIGGY Ref 12345"
	tx, ok := RegexParse(msg, testRules)
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", tx.Date)
	assert.Equal(t, -2450.0, tx.Amount)
	assert.Equal(t, "INR", tx.Currency)
	assert.Equal(t, "Food: Lunch", tx.Merchant)
	assert.Equal(t, "Expenses:Food:Hyd:Swiggy", tx.SuggestedLedgerAccount)
	assert.Equal(t, "Assets:Checking:FI5687", tx.SourceAccount)
	assert.Equal(t, 1.0, tx.Confidence)
	assert.Equal(t, "debit", tx.TxType)
}

func TestRegexParse_CanaraLoan_DestOverride(t *testing.T) {
	msg := "XXXXX21343 Canara Bank Loan Drawdown INR 45,000.00 on 01/05/2025"
	tx, ok := RegexParse(msg, testRules)
	assert.True(t, ok)
	assert.Equal(t, "2025-05-01", tx.Date)
	assert.Equal(t, "EMI: Home Loan", tx.Merchant)
	assert.Equal(t, "Liabilities:Loan:CANAHL1090", tx.SuggestedLedgerAccount)
	assert.Equal(t, "Assets:Checking:CANA1343", tx.SourceAccount)
}

func TestRegexParse_NoSourceMatch(t *testing.T) {
	_, ok := RegexParse("some random text with no bank markers", testRules)
	assert.False(t, ok)
}

func TestRegexParse_SourceMatchButNoDate(t *testing.T) {
	_, ok := RegexParse("INR 100 debited via UPI on no-date-here at Swiggy", testRules)
	assert.False(t, ok)
}

func TestRegexParse_SourceMatchButNoAmount(t *testing.T) {
	_, ok := RegexParse("debited via UPI on 14-05-2025 at Swiggy no amount present", testRules)
	assert.False(t, ok)
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd .worktrees/feat/ingestion-agent && go test ./internal/agent/parser/... -run TestParseDate -v 2>&1 | head -5
```
Expected: compile error `undefined: parseDate` (functions don't exist yet).

---

### Task 4: Implement regex_parser.go

**Files:**
- Create: `internal/agent/parser/regex_parser.go`

- [ ] **Step 1: Create the file**

```go
package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/config"
)

var (
	reDateDMY4   = regexp.MustCompile(`\bon\s+(\d{2})-(\d{2})-(\d{4})\b`)
	reDateDMY2   = regexp.MustCompile(`\b(\d{2})-(\d{2})-(\d{2})\b`)
	reDateMonDY  = regexp.MustCompile(`(?i)\bon\s+([A-Za-z]{3})\s+(\d{1,2}),\s*(\d{4})\b`)
	reDateDMONY2 = regexp.MustCompile(`(?i)\b(\d{2})(JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC)(\d{2})?\b`)
	reDateDMONY4 = regexp.MustCompile(`(?i)\bon\s+(\d{1,2})\s+(JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC),?\s*(\d{4})?\b`)
	reDateSlash  = regexp.MustCompile(`\bon\s+(\d{2})/(\d{2})/(\d{4})\b`)
	reDateFull   = regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2}),\s*(\d{4})\b`)

	reAmountRs  = regexp.MustCompile(`(?i)Rs\.?\s*([0-9,]+(?:\.[0-9]+)?)`)
	reAmountINR = regexp.MustCompile(`(?i)INR\s*([0-9,]+(?:\.[0-9]+)?)`)
	reTime      = regexp.MustCompile(`\b(\d{2}):(\d{2})(?::\d{2})?\b`)
)

var shortMonths = map[string]string{
	"jan": "01", "feb": "02", "mar": "03", "apr": "04",
	"may": "05", "jun": "06", "jul": "07", "aug": "08",
	"sep": "09", "oct": "10", "nov": "11", "dec": "12",
}

var fullMonths = map[string]string{
	"january": "01", "february": "02", "march": "03", "april": "04",
	"may": "05", "june": "06", "july": "07", "august": "08",
	"september": "09", "october": "10", "november": "11", "december": "12",
}

func centuryPrefix() string {
	return strconv.Itoa(time.Now().Year())[:2]
}

func lpad(s string) string {
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

func parseDate(msg string) (string, bool) {
	pfx := centuryPrefix()

	// Pattern 1: on DD-MM-YYYY
	if m := reDateDMY4.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("%s-%s-%s", m[3], lpad(m[2]), lpad(m[1])), true
	}
	// Pattern 2: DD-MM-YY (2-digit year)
	if m := reDateDMY2.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("%s%s-%s-%s", pfx, m[3], lpad(m[2]), lpad(m[1])), true
	}
	// Pattern 3: on Mon DD, YYYY
	if m := reDateMonDY.FindStringSubmatch(msg); m != nil {
		if mon, ok := shortMonths[strings.ToLower(m[1])]; ok {
			return fmt.Sprintf("%s-%s-%s", m[3], mon, lpad(m[2])), true
		}
	}
	// Pattern 4: DDMONYY (e.g. 14MAY25)
	if m := reDateDMONY2.FindStringSubmatch(msg); m != nil {
		if mon, ok := shortMonths[strings.ToLower(m[2])]; ok {
			year := pfx + m[3]
			if m[3] == "" {
				year = strconv.Itoa(time.Now().Year())
			}
			return fmt.Sprintf("%s-%s-%s", year, mon, lpad(m[1])), true
		}
	}
	// Pattern 5: on D MON,YYYY
	if m := reDateDMONY4.FindStringSubmatch(msg); m != nil {
		if mon, ok := shortMonths[strings.ToLower(m[2])]; ok {
			year := m[3]
			if year == "" {
				year = strconv.Itoa(time.Now().Year())
			}
			return fmt.Sprintf("%s-%s-%s", year, mon, lpad(m[1])), true
		}
	}
	// Pattern 6: on DD/MM/YYYY
	if m := reDateSlash.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("%s-%s-%s", m[3], lpad(m[2]), lpad(m[1])), true
	}
	// Pattern 7: Month D, YYYY (full month name)
	if m := reDateFull.FindStringSubmatch(msg); m != nil {
		if mon, ok := fullMonths[strings.ToLower(m[1])]; ok {
			return fmt.Sprintf("%s-%s-%s", m[3], mon, lpad(m[2])), true
		}
	}
	return "", false
}

func parseAmount(msg string) (float64, bool) {
	for _, re := range []*regexp.Regexp{reAmountRs, reAmountINR} {
		if m := re.FindStringSubmatch(msg); m != nil {
			if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64); err == nil {
				return v, true
			}
		}
	}
	return 0, false
}

func parseDayPart(msg string, dp config.DayPartsConfig) string {
	m := reTime.FindStringSubmatch(msg)
	if m == nil {
		return "Meal"
	}
	h, _ := strconv.Atoi(m[1])
	switch {
	case h < dp.BreakfastEnd:
		return "Breakfast"
	case h < dp.LunchEnd:
		return "Lunch"
	case h < dp.DinnerEnd:
		return "Dinner"
	default:
		return "Evening Snack"
	}
}

func matchMerchant(msg string, merchants []config.MerchantPattern, dayPart string) (string, string) {
	lower := strings.ToLower(msg)
	for _, m := range merchants {
		if m.Keyword == "" || strings.Contains(lower, strings.ToLower(m.Keyword)) {
			return strings.ReplaceAll(m.Description, "{day_part}", dayPart), m.Account
		}
	}
	return "Misc", "Expenses:Misc"
}

func matchSource(msg string, sources []config.SourceRule) (config.SourceRule, bool) {
	for _, s := range sources {
		matched := true
		for _, c := range s.Contains {
			if !strings.Contains(msg, c) {
				matched = false
				break
			}
		}
		if matched {
			return s, true
		}
	}
	return config.SourceRule{}, false
}

// RegexParse attempts rule-based parsing of a bank SMS/alert.
// Returns (zero, false) if no source rule matched or date/amount extraction failed.
func RegexParse(msg string, rules config.ParserRules) (ParsedTransaction, bool) {
	src, ok := matchSource(msg, rules.Sources)
	if !ok {
		return ParsedTransaction{}, false
	}
	date, ok := parseDate(msg)
	if !ok {
		return ParsedTransaction{}, false
	}
	amount, ok := parseAmount(msg)
	if !ok {
		return ParsedTransaction{}, false
	}

	var desc, destAccount string
	if src.DestAccount != "" {
		desc = src.Description
		destAccount = src.DestAccount
	} else {
		dayPart := parseDayPart(msg, rules.DayParts)
		desc, destAccount = matchMerchant(msg, rules.Merchants, dayPart)
	}

	return ParsedTransaction{
		Date:                   date,
		Amount:                 -amount,
		Currency:               "INR",
		Merchant:               desc,
		TxType:                 "debit",
		SuggestedLedgerAccount: destAccount,
		SourceAccount:          src.Account,
		Confidence:             1.0,
	}, true
}
```

- [ ] **Step 2: Run tests**

```bash
cd .worktrees/feat/ingestion-agent && go test ./internal/agent/parser/... -v 2>&1 | tail -30
```
Expected: all regex_parser tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/agent/parser/regex_parser.go internal/agent/parser/regex_parser_test.go
git commit -m "feat(parser): implement regex SMS parser with YAML rules"
```

---

### Task 5: Wire regex into Parser.Parse() + fix parser_test.go

**Files:**
- Modify: `internal/agent/parser/parser.go`
- Modify: `internal/agent/parser/parser_test.go`

- [ ] **Step 1: Update Parser struct and New() to accept rules**

In `internal/agent/parser/parser.go`, add the `rules` field and update `New()`:

```go
// replace the Parser struct and New() function:

type Parser struct {
	url   string
	model string
	rules config.ParserRules
}

func New(url, model string, rules config.ParserRules) *Parser {
	return &Parser{url: url, model: model, rules: rules}
}
```

Add the import for config at the top:
```go
import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ananthakumaran/paisa/internal/agent/config"
)
```

- [ ] **Step 2: Add regex-first path to Parse()**

In `Parse()`, add the regex check as the first thing:

```go
func (p *Parser) Parse(rawText string, knownAccounts []string) (ParsedTransaction, error) {
	if tx, ok := RegexParse(rawText, p.rules); ok {
		return tx, nil
	}

	accountHint := ""
	if len(knownAccounts) > 0 {
		accountHint = fmt.Sprintf("\nKnown ledger accounts (choose suggested_ledger_account from these): %s",
			strings.Join(knownAccounts, ", "))
	}
	// ... rest of existing Ollama code unchanged
```

- [ ] **Step 3: Fix parser_test.go — update New() calls and add import**

In `internal/agent/parser/parser_test.go`, update imports to add config:

```go
import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/stretchr/testify/assert"
)
```

Update every `New(srv.URL, "gemma3:12b")` to:

```go
p := New(srv.URL, "gemma3:12b", config.ParserRules{})
```

(There are 3 occurrences — in `TestParse_ExtractsTransaction`, `TestParse_HandlesOllamaError`, `TestParseMultiple_ExtractsManyTransactions`.)

- [ ] **Step 4: Add test verifying regex path short-circuits Ollama**

Append to `parser_test.go`:

```go
func TestParse_UsesRegexWhenSourceMatches(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(500)
	}))
	defer srv.Close()

	rules := config.ParserRules{
		DayParts: config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20},
		Merchants: []config.MerchantPattern{
			{Keyword: "", Description: "Misc", Account: "Expenses:Misc"},
		},
		Sources: []config.SourceRule{
			{ID: "fi_upi", Contains: []string{"debited via UPI on"}, Account: "Assets:Checking:FI5687"},
		},
	}
	p := New(srv.URL, "gemma3:12b", rules)
	tx, err := p.Parse("INR 100.00 debited via UPI on 14-05-2025 at Store", nil)

	assert.NoError(t, err)
	assert.False(t, called, "Ollama must not be called when regex matches")
	assert.Equal(t, "Assets:Checking:FI5687", tx.SourceAccount)
	assert.Equal(t, -100.0, tx.Amount)
}
```

- [ ] **Step 5: Run all parser tests**

```bash
cd .worktrees/feat/ingestion-agent && go test ./internal/agent/parser/... -v 2>&1 | tail -20
```
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/parser/parser.go internal/agent/parser/parser_test.go
git commit -m "feat(parser): wire regex as primary path; New() accepts ParserRules"
```

---

### Task 6: Complete pipeline.go and add yaml.example

**Files:**
- Modify: `internal/agent/pipeline/pipeline.go`
- Create: `scripts/paisa-agent.yaml.example`

- [ ] **Step 1: Update pipeline.New() to pass ParserRules to parser.New()**

In `pipeline.go`, update the `parser.New()` call:

```go
func New(db *gorm.DB, cfg agentconfig.Config) *Pipeline {
	return &Pipeline{
		db:     db,
		cfg:    cfg,
		parser: parser.New(cfg.Ollama.URL, cfg.Ollama.Model, cfg.ParserRules),
		bot:    telegram.New(cfg.Telegram.BotToken, cfg.Telegram.ChatID),
	}
}
```

- [ ] **Step 2: Use SourceAccount in post()**

Update `post()` to prefer `tx.SourceAccount`:

```go
func (p *Pipeline) post(tx parser.ParsedTransaction, source string) error {
	debitAccount := tx.SourceAccount
	if debitAccount == "" {
		debitAccount = p.resolveAccount(tx.Bank, tx.AccountLast4)
	}
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
```

- [ ] **Step 3: Run all tests**

```bash
cd .worktrees/feat/ingestion-agent && go test ./internal/agent/... 2>&1
```
Expected: all packages pass.

- [ ] **Step 4: Create paisa-agent.yaml.example**

```yaml
# paisa-agent.yaml — copy to ~/.config/paisa-agent/paisa-agent.yaml

paisa:
  url: http://localhost:7500
  journal_dir: /path/to/your/journal
  api_token: ""

ollama:
  url: http://localhost:11434
  model: gemma3:12b

telegram:
  bot_token: "YOUR_BOT_TOKEN"
  chat_id: 123456789

gmail:
  credentials_file: /path/to/credentials.json
  poll_interval_seconds: 300
  labels:
    - INBOX

merchant_rules:
  auto_approve_threshold: 10000
  promote_after_approvals: 3

parser_rules:
  day_parts:
    breakfast_end: 11
    lunch_end: 15
    dinner_end: 20

  # First keyword match wins. Empty keyword = catch-all (must be last).
  merchants:
    - keyword: swiggy
      description: "Food: {day_part}"
      account: "Expenses:Food:Hyd:Swiggy"
    - keyword: zomato
      description: "Food: {day_part}"
      account: "Expenses:Food:Hyd:Zomato"
    - keyword: zepto
      description: "Groceries"
      account: "Expenses:Groceries:Hyd"
    - keyword: food
      description: "Food: {day_part}"
      account: "Expenses:Food:Hyd"
    - keyword: ""
      description: "Misc"
      account: "Expenses:Misc:Hyd"

  # First rule where ALL contains strings match wins.
  # dest_account + description override merchant matching (fixed payees like EMI).
  sources:
    - id: fi_upi
      contains: ["debited via UPI on"]
      account: "Assets:Checking:FI5687"
    - id: fi_upi_alt
      contains: ["debited from your A/c via UPI on"]
      account: "Assets:Checking:FI5687"
    - id: fi_dc
      contains: ["spent@"]
      account: "Assets:Checking:FI5687"
    - id: fi_dc_alt
      contains: ["You've spent INR "]
      account: "Assets:Checking:FI5687"
    - id: axis_upi
      contains: ["A/c no. XX6386"]
      account: "Assets:Checking:AXIS6386"
    - id: cc_fk
      contains: ["Card no. XX8860"]
      account: "Liabilities:CreditCard:FK8860"
    - id: cc_mz
      contains: ["Card no. XX1610"]
      account: "Liabilities:CreditCard:MyZone1610"
    - id: cc_select
      contains: ["Card no. XX6792"]
      account: "Liabilities:CreditCard:SELECT6792"
    - id: food_card
      contains: ["HDFC Bank Prepaid Card 2148"]
      account: "Assets:Checking:FC2148"
    - id: canara_loan
      contains: ["XXXXX21343", "Canara Bank", "Loan Drawdown"]
      account: "Assets:Checking:CANA1343"
      dest_account: "Liabilities:Loan:CANAHL1090"
      description: "EMI: Home Loan"
    - id: jupiter
      contains: ["XX6465"]
      account: "Assets:Checking:JUPI5114"
    - id: term_insurance
      contains: ["Sarath M payment of Rs.1672 for your TATA AIA Life policy"]
      account: "Assets:Checking:AXIS6386"
      dest_account: "Expenses:Insurance:TATA0299"
      description: "Insurance: Term Insurance Premium"
```

- [ ] **Step 5: Build binary and confirm it works**

```bash
cd .worktrees/feat/ingestion-agent && go build -o paisa-agent ./cmd/paisa-agent/ && echo "BUILD OK"
```
Expected: `BUILD OK`

- [ ] **Step 6: Commit and push**

```bash
git add internal/agent/pipeline/pipeline.go scripts/paisa-agent.yaml.example
git commit -m "feat(pipeline): use SourceAccount from regex parse; add yaml.example"
git push origin feat/ingestion-agent
```

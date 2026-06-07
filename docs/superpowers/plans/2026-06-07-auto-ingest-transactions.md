# Auto-Ingest Transactions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `paisa-agent`, a standalone Go sidecar that receives bank SMS messages via Telegram, parses them into ledger entries using regex+LLM fallback, sends a Telegram approval draft, and on approval appends to `auto-import.ledger`.

**Architecture:** Standalone binary at `cmd/paisa-agent/main.go`. All logic lives in `internal/agent/` packages. Polls Telegram long-poll API, dispatches to parser pipeline, manages in-memory approval state, appends approved entries directly to disk.

**Tech Stack:** Go 1.24, `gopkg.in/yaml.v3` (already in go.mod), `github.com/stretchr/testify` (already in go.mod), `github.com/sirupsen/logrus` (already in go.mod), Telegram Bot API (raw HTTP, no new dependency), Ollama HTTP API (raw HTTP).

---

## File Map

| File | Responsibility |
|---|---|
| `internal/agent/ledger/entry.go` | Entry struct + Format() as ledger block |
| `internal/agent/ledger/appender.go` | EnsureFile, IsDuplicate, Append |
| `internal/agent/config/config.go` | Config structs + Load() from YAML |
| `internal/agent/parser/dates.go` | NormaliseDate, ExtractDateFromSMS |
| `internal/agent/parser/amounts.go` | NormaliseAmount, FormatEntryAmount, ExtractAmountFromSMS |
| `internal/agent/parser/banks.go` | Bank-specific regex extractors (6 banks) |
| `internal/agent/parser/merchant.go` | RouteMerchant (keyword → account + description) |
| `internal/agent/parser/parser.go` | Classify (SMS → AccountRule) + Parse (→ Entry) |
| `internal/agent/llm/ollama.go` | FillMissing (Ollama HTTP call for empty Desc/Dest) |
| `internal/agent/approval/state.go` | In-memory Store: pending/editing states per messageID |
| `internal/agent/telegram/format.go` | FormatDraft, FormatEditTemplate, ParseEditReply |
| `internal/agent/telegram/bot.go` | Telegram Bot API client (Poll, SendDraft, EditMessage, etc.) |
| `cmd/paisa-agent/main.go` | Binary entry point: poll loop, event dispatch |

---

## Task 1: Entry struct + ledger block formatter

**Files:**
- Create: `internal/agent/ledger/entry.go`
- Create: `internal/agent/ledger/entry_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/agent/ledger/entry_test.go
package ledger_test

import (
    "testing"
    "github.com/ananthakumaran/paisa/internal/agent/ledger"
    "github.com/stretchr/testify/assert"
)

func TestEntryFormat_Debit(t *testing.T) {
    e := ledger.Entry{
        Date: "2026/06/03",
        Desc: "Food Swiggy",
        Src:  "Assets:Checking:FC2148",
        Amt:  "-215.00 INR",
        Dest: "Expenses:Food:Hyd",
    }
    got := e.Format()
    assert.Contains(t, got, "2026/06/03 Food Swiggy")
    assert.Contains(t, got, "Assets:Checking:FC2148")
    assert.Contains(t, got, "-215.00 INR")
    assert.Contains(t, got, "Expenses:Food:Hyd")
}

func TestEntryFormat_Credit(t *testing.T) {
    e := ledger.Entry{
        Date: "2026/06/01",
        Desc: "Rent from Haritha",
        Src:  "Assets:Checking:AXIS6386",
        Amt:  "30000.00 INR",
        Dest: "Assets:Checking:AXISHARITHA",
    }
    got := e.Format()
    assert.Contains(t, got, "2026/06/01 Rent from Haritha")
    assert.Contains(t, got, "30000.00 INR")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/ledger/... -v
```
Expected: compile error (package does not exist yet)

- [ ] **Step 3: Implement entry.go**

```go
// internal/agent/ledger/entry.go
package ledger

import "fmt"

type Entry struct {
    Date string // "2026/06/03"
    Desc string // "Food Swiggy"
    Src  string // first posting account (= YAML destinations)
    Amt  string // "-215.00 INR"
    Dest string // second posting account, auto-balanced (= YAML src)
}

// Format returns the ledger journal block for this entry.
func (e Entry) Format() string {
    return fmt.Sprintf("%s %s\n    %-44s  %s\n    %s\n",
        e.Date, e.Desc, e.Src, e.Amt, e.Dest)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/agent/ledger/... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/ledger/entry.go internal/agent/ledger/entry_test.go
git commit -m "feat(agent): add Entry struct and ledger block formatter"
```

---

## Task 2: Config struct + YAML loader

**Files:**
- Create: `internal/agent/config/config.go`
- Create: `internal/agent/config/config_test.go`
- Create: `internal/agent/config/testdata/sample.yaml`

- [ ] **Step 1: Create testdata YAML**

```yaml
# internal/agent/config/testdata/sample.yaml
paisa:
  url: http://localhost:7500
  journal_dir: /tmp/journal

ollama:
  url: http://localhost:11434
  model: gemma3:4b

telegram:
  bot_token: "test-token"
  chat_id: 123456

parser_rules:
  accounts:
    - bank: fixed
      identifiers: ["CRD-PMNT", "8860"]
      src: "Assets:Checking:AXIS6386"
      destinations: "Liabilities:CreditCard:FK8860"
      description: "CC Payment"
    - bank: hdfc_debit
      identifiers: ["HDFC Bank Card 2148"]
      destinations: "Assets:Checking:FC2148"
  merchants:
    - keyword: "swiggy"
      account: "Expenses:Food:Hyd"
      description: "Food Swiggy"
```

- [ ] **Step 2: Write failing test**

```go
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
    assert.Equal(t, []string{"CRD-PMNT", "8860"}, cfg.ParserRules.Accounts[0].Identifiers)
    assert.Equal(t, "Assets:Checking:AXIS6386", cfg.ParserRules.Accounts[0].Src)
    assert.Len(t, cfg.ParserRules.Merchants, 1)
    assert.Equal(t, "swiggy", cfg.ParserRules.Merchants[0].Keyword)
}

func TestLoad_FileNotFound(t *testing.T) {
    _, err := config.Load("testdata/nonexistent.yaml")
    assert.Error(t, err)
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/agent/config/... -v
```
Expected: compile error

- [ ] **Step 4: Implement config.go**

```go
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
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/agent/config/... -v
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/agent/config/
git commit -m "feat(agent): add Config struct and YAML loader"
```

---

## Task 3: Date normalisation + date extractor

**Files:**
- Create: `internal/agent/parser/dates.go`
- Create: `internal/agent/parser/dates_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/parser/dates_test.go
package parser_test

import (
    "testing"
    "github.com/ananthakumaran/paisa/internal/agent/parser"
    "github.com/stretchr/testify/assert"
)

func TestNormaliseDate(t *testing.T) {
    cases := []struct {
        raw  string
        want string
    }{
        {"03-Jun-26", "2026/06/03"},   // DD-Mon-YY
        {"15-MAY-26", "2026/05/15"},   // DD-MON-YY uppercase
        {"03 Jun,2026", "2026/06/03"}, // DD Mon,YYYY
        {"31 May,2026", "2026/05/31"}, // DD Mon,YYYY
        {"2026-05-21", "2026/05/21"},  // YYYY-MM-DD
        {"03-06-26", "2026/06/03"},    // DD-MM-YY
        {"09/04/26", "2026/04/09"},    // DD/MM/YY
        {"31/05/26", "2026/05/31"},    // DD/MM/YY
    }
    for _, c := range cases {
        got, err := parser.NormaliseDate(c.raw)
        assert.NoError(t, err, "input: %q", c.raw)
        assert.Equal(t, c.want, got, "input: %q", c.raw)
    }
}

func TestNormaliseDate_Unknown(t *testing.T) {
    _, err := parser.NormaliseDate("not-a-date")
    assert.Error(t, err)
}

func TestExtractDateFromSMS(t *testing.T) {
    cases := []struct {
        sms  string
        want string
    }{
        {"INR 453.00 spent using ICICI Bank Card XX6009 on 03-Jun-26 on AMAZON PAY IN G.", "03-Jun-26"},
        {"Payment of Rs 10,468.87 has been received ... on 15-MAY-26.", "15-MAY-26"},
        {"Spent! INR INR 215 On HDFC Bank Card 2148 At RAZ*Swiggy On 03 Jun,2026 01:18 PM IST", "03 Jun,2026"},
        {"Spent Rs.341 On HDFC Bank Card 2527 At ZEPTO On 2026-05-21:07:32:56.", "2026-05-21"},
        {"INR 1804.05 debited\nA/c no. XX6386\n03-06-26, 10:21:54\nUPI/P2M/...", "03-06-26"},
        {"Spent Rs.473.00 from A/C XX6977 at ZEPTO on 09/04/26.", "09/04/26"},
    }
    for _, c := range cases {
        got, err := parser.ExtractDateFromSMS(c.sms)
        assert.NoError(t, err, "sms: %q", c.sms)
        assert.Equal(t, c.want, got, "sms snippet: %q", c.sms[:30])
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/parser/... -run TestNormaliseDate -v
go test ./internal/agent/parser/... -run TestExtractDateFromSMS -v
```
Expected: compile error

- [ ] **Step 3: Implement dates.go**

```go
// internal/agent/parser/dates.go
package parser

import (
    "fmt"
    "regexp"
    "strings"
    "time"
)

// datePattern pairs a regex that captures a raw date string with a description.
var dateExtractPatterns = []*regexp.Regexp{
    // DD-Mon-YY or DD-MON-YY  (must come before DD-MM-YY to avoid month-abbrev clash)
    regexp.MustCompile(`(?i)\b(\d{2}-(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)-\d{2})\b`),
    // DD Mon,YYYY
    regexp.MustCompile(`(?i)\b(\d{1,2}\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec),\d{4})\b`),
    // YYYY-MM-DD  (must come before DD-MM-YY to avoid 4-digit year ambiguity)
    regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`),
    // DD-MM-YY
    regexp.MustCompile(`\b(\d{2}-\d{2}-\d{2})\b`),
    // DD/MM/YY
    regexp.MustCompile(`\b(\d{2}/\d{2}/\d{2})\b`),
}

// ExtractDateFromSMS finds the first recognisable date in an SMS string.
func ExtractDateFromSMS(sms string) (string, error) {
    for _, re := range dateExtractPatterns {
        if m := re.FindStringSubmatch(sms); m != nil {
            return m[1], nil
        }
    }
    return "", fmt.Errorf("no date found in SMS")
}

// NormaliseDate converts any supported raw date string to YYYY/MM/DD.
// Supported input formats: DD-Mon-YY, DD Mon,YYYY, YYYY-MM-DD, DD-MM-YY, DD/MM/YY.
// Month abbreviations are case-insensitive.
func NormaliseDate(raw string) (string, error) {
    raw = strings.TrimSpace(raw)
    norm := normalizeMonthCase(raw)

    // DD-Mon-YY  (e.g. 03-Jun-26, 15-MAY-26)
    if t, err := time.Parse("02-Jan-06", norm); err == nil {
        return t.Format("2006/01/02"), nil
    }

    // DD Mon,YYYY  (e.g. 03 Jun,2026) — replace comma with space first
    withoutComma := strings.ReplaceAll(norm, ",", " ")
    if t, err := time.Parse("02 Jan 2006", withoutComma); err == nil {
        return t.Format("2006/01/02"), nil
    }

    // YYYY-MM-DD  (e.g. 2026-05-21)
    if t, err := time.Parse("2006-01-02", raw); err == nil {
        return t.Format("2006/01/02"), nil
    }

    // DD-MM-YY  (e.g. 03-06-26)
    if t, err := time.Parse("02-01-06", raw); err == nil {
        return t.Format("2006/01/02"), nil
    }

    // DD/MM/YY  (e.g. 09/04/26)
    if t, err := time.Parse("02/01/06", raw); err == nil {
        return t.Format("2006/01/02"), nil
    }

    return "", fmt.Errorf("unrecognised date format: %q", raw)
}

var monthNormRe = regexp.MustCompile(`(?i)\b(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)\b`)

// normalizeMonthCase title-cases month abbreviations so time.Parse accepts them.
func normalizeMonthCase(s string) string {
    return monthNormRe.ReplaceAllStringFunc(s, func(m string) string {
        lower := strings.ToLower(m)
        return strings.ToUpper(lower[:1]) + lower[1:]
    })
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/parser/... -run "TestNormaliseDate|TestExtractDateFromSMS" -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/parser/dates.go internal/agent/parser/dates_test.go
git commit -m "feat(agent): add date normalisation and SMS date extractor"
```

---

## Task 4: Amount parsing utilities

**Files:**
- Create: `internal/agent/parser/amounts.go`
- Create: `internal/agent/parser/amounts_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/parser/amounts_test.go
package parser_test

import (
    "testing"
    "github.com/ananthakumaran/paisa/internal/agent/parser"
    "github.com/stretchr/testify/assert"
)

func TestNormaliseAmount(t *testing.T) {
    cases := []struct{ raw, want string }{
        {"453.00", "453.00"},
        {"10,468.87", "10468.87"},
        {"215", "215.00"},
        {"11864", "11864.00"},
        {"318.00", "318.00"},
        {"1,50,101.05", "150101.05"},
        {"341", "341.00"},
    }
    for _, c := range cases {
        got, err := parser.NormaliseAmount(c.raw)
        assert.NoError(t, err)
        assert.Equal(t, c.want, got, "raw=%q", c.raw)
    }
}

func TestFormatEntryAmount(t *testing.T) {
    assert.Equal(t, "-215.00 INR", parser.FormatEntryAmount("215.00", true))
    assert.Equal(t, "30000.00 INR", parser.FormatEntryAmount("30000.00", false))
}

func TestExtractAmountFromSMS(t *testing.T) {
    cases := []struct {
        sms     string
        wantAmt string
        wantDeb bool
    }{
        {"Payment of Rs 10,468.87 has been received on your ICICI Bank Credit Card XX6009", "10468.87", false},
        {"Debit INR 1417.00\nAxis Bank A/c XX6386\n02-06-26", "1417.00", true},
        {"INR 30000.00 credited\nA/c no. XX6386", "30000.00", false},
        {"Monthly interest of INR.318.00 earned on your Savings A/c XX6977", "318.00", false},
    }
    for _, c := range cases {
        amt, isDebit, err := parser.ExtractAmountFromSMS(c.sms)
        assert.NoError(t, err, "sms: %q", c.sms[:40])
        assert.Equal(t, c.wantAmt, amt, "sms: %q", c.sms[:40])
        assert.Equal(t, c.wantDeb, isDebit, "sms: %q", c.sms[:40])
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/parser/... -run "TestNormaliseAmount|TestFormatEntryAmount|TestExtractAmountFromSMS" -v
```
Expected: compile error

- [ ] **Step 3: Implement amounts.go**

```go
// internal/agent/parser/amounts.go
package parser

import (
    "fmt"
    "regexp"
    "strings"
)

var amountRe = regexp.MustCompile(`(?i)(?:INR\.?|Rs\.?)\s*([\d,]+(?:\.\d{1,2})?)`)

// NormaliseAmount strips Indian commas and ensures exactly 2 decimal places.
func NormaliseAmount(raw string) (string, error) {
    s := strings.ReplaceAll(raw, ",", "")
    if !strings.Contains(s, ".") {
        return s + ".00", nil
    }
    parts := strings.SplitN(s, ".", 2)
    switch len(parts[1]) {
    case 0:
        return parts[0] + ".00", nil
    case 1:
        return s + "0", nil
    default:
        return parts[0] + "." + parts[1][:2], nil
    }
}

// FormatEntryAmount returns the amount string for an Entry (e.g. "-215.00 INR").
// isDebit=true adds a negative sign; false gives a plain positive amount.
func FormatEntryAmount(normalised string, isDebit bool) string {
    if isDebit {
        return fmt.Sprintf("-%s INR", normalised)
    }
    return fmt.Sprintf("%s INR", normalised)
}

// ExtractAmountFromSMS finds the first INR/Rs amount in an SMS and detects debit/credit.
// Used by fixed-route parser where no bank-specific regex is available.
func ExtractAmountFromSMS(sms string) (normalised string, isDebit bool, err error) {
    m := amountRe.FindStringSubmatch(sms)
    if m == nil {
        return "", false, fmt.Errorf("no INR/Rs amount found in SMS")
    }
    norm, err := NormaliseAmount(m[1])
    if err != nil {
        return "", false, err
    }
    lower := strings.ToLower(sms)
    isDebit = strings.Contains(lower, "debited") ||
        strings.Contains(lower, "debit") ||
        strings.Contains(lower, "spent")
    return norm, isDebit, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/parser/... -run "TestNormaliseAmount|TestFormatEntryAmount|TestExtractAmountFromSMS" -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/parser/amounts.go internal/agent/parser/amounts_test.go
git commit -m "feat(agent): add amount parsing and formatting utilities"
```

---

## Task 5: Bank-specific regex extractors

**Files:**
- Create: `internal/agent/parser/banks.go`
- Create: `internal/agent/parser/banks_test.go`

Each extractor returns `(merchant, rawDate, rawAmt string, isDebit bool, err error)`.
The caller normalises rawDate and rawAmt after extraction.

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/parser/banks_test.go
package parser_test

import (
    "testing"
    "github.com/ananthakumaran/paisa/internal/agent/parser"
    "github.com/stretchr/testify/assert"
)

func TestExtractIciciCC(t *testing.T) {
    sms := "INR 453.00 spent using ICICI Bank Card XX6009 on 03-Jun-26 on AMAZON PAY IN G. Avl Limit: INR 4,59,509.36. If not you, call 1800 2662/SMS BLOCK 6009 to 9215676766"
    m, d, a, debit, err := parser.ExtractIciciCC(sms)
    assert.NoError(t, err)
    assert.Equal(t, "AMAZON PAY IN G", m)
    assert.Equal(t, "03-Jun-26", d)
    assert.Equal(t, "453.00", a)
    assert.True(t, debit)
}

func TestExtractHdfcDebit(t *testing.T) {
    cases := []struct {
        sms      string
        merchant string
        date     string
        amt      string
    }{
        {
            "Spent! INR INR 215 On HDFC Bank Card 2148 At RAZ*Swiggy/Bangalore On 03 Jun,2026 01:18 PM IST Bal INR INR 23216.37 To Block, Call 08069858585",
            "RAZ*Swiggy/Bangalore", "03 Jun,2026", "215",
        },
        {
            "Spent! INR INR 426 On HDFC Bank Card 2148 At BLINK COMMERCE PVT L On 02 Jun,2026 08:32 PM IST Bal INR INR 23431.37 To Block, Call 08069858585",
            "BLINK COMMERCE PVT L", "02 Jun,2026", "426",
        },
        {
            "Spent! INR INR 327.25 On HDFC Bank Card 2148 At ZOMATO/110018759//IN On 31 May,2026 10:28 PM IST Bal INR INR 9534.37 To Block, Call 08069858585",
            "ZOMATO/110018759//IN", "31 May,2026", "327.25",
        },
    }
    for _, c := range cases {
        m, d, a, debit, err := parser.ExtractHdfcDebit(c.sms)
        assert.NoError(t, err)
        assert.Equal(t, c.merchant, m)
        assert.Equal(t, c.date, d)
        assert.Equal(t, c.amt, a)
        assert.True(t, debit)
    }
}

func TestExtractHdfcCC(t *testing.T) {
    sms := "Spent Rs.341 On HDFC Bank Card 2527 At ZEPTO MARKETPLACE PRIV On 2026-05-21:07:32:56.Not You? To Block+Reissue Call 18002586161"
    m, d, a, debit, err := parser.ExtractHdfcCC(sms)
    assert.NoError(t, err)
    assert.Equal(t, "ZEPTO MARKETPLACE PRIV", m)
    assert.Equal(t, "2026-05-21", d)
    assert.Equal(t, "341", a)
    assert.True(t, debit)
}

func TestExtractAxisChecking(t *testing.T) {
    sms := "INR 1804.05 debited\nA/c no. XX6386\n03-06-26, 10:21:54\nUPI/P2M/102154212206/IRCTC Rail Web\nNot you? SMS BLOCKUPI Cust ID to 919951860002\nAxis Bank"
    m, d, a, debit, err := parser.ExtractAxisChecking(sms)
    assert.NoError(t, err)
    assert.Equal(t, "IRCTC Rail Web", m)
    assert.Equal(t, "03-06-26", d)
    assert.Equal(t, "1804.05", a)
    assert.True(t, debit)
}

func TestExtractAxisCC(t *testing.T) {
    cases := []struct {
        sms      string
        merchant string
        date     string
        amt      string
    }{
        {
            "Spent INR 210.12\nAxis Bank Card no. XX1610\n08-05-26 18:44:17 IST\nDISTRICT MO\nAvl Limit: INR 1389000.78\nNot you? SMS BLOCK 1610 to 919951860002",
            "DISTRICT MO", "08-05-26", "210.12",
        },
        {
            "Spent INR 11864\nAxis Bank Card no. XX6792\n23-05-26 23:30:19 IST\nFLIPKART\nAvl Limit: INR 1324182.46\nNot you? SMS BLOCK 6792 to 919951860002",
            "FLIPKART", "23-05-26", "11864",
        },
        {
            "Spent INR 3468\nAxis Bank Card no. XX8860\n01-06-26 15:22:40 IST\nIng*Flipkar\nAvl Limit: INR 1320714.46\nNot you? SMS BLOCK 8860 to 919951860002",
            "Ing*Flipkar", "01-06-26", "3468",
        },
    }
    for _, c := range cases {
        m, d, a, debit, err := parser.ExtractAxisCC(c.sms)
        assert.NoError(t, err)
        assert.Equal(t, c.merchant, m)
        assert.Equal(t, c.date, d)
        assert.Equal(t, c.amt, a)
        assert.True(t, debit)
    }
}

func TestExtractIDFCChecking(t *testing.T) {
    t.Run("spend", func(t *testing.T) {
        sms := "Spent Rs.473.00 from A/C XX6977 at ZEPTO MARKETPLACE PRIV on 09/04/26. Not you? Call 180010888/SMS BLOCK (last 4 digit of card) to 5676732. IDFC FIRST Bank"
        m, d, a, debit, err := parser.ExtractIDFCChecking(sms)
        assert.NoError(t, err)
        assert.Equal(t, "ZEPTO MARKETPLACE PRIV", m)
        assert.Equal(t, "09/04/26", d)
        assert.Equal(t, "473.00", a)
        assert.True(t, debit)
    })
    t.Run("interest", func(t *testing.T) {
        sms := "Monthly interest of INR.318.00 earned on your Savings A/c XX6977 has been credited to your A/C on 31/05/26. New bal: INR.1,50,101.05. IDFC FIRST Bank"
        m, d, a, debit, err := parser.ExtractIDFCChecking(sms)
        assert.NoError(t, err)
        assert.Equal(t, "Monthly interest", m)
        assert.Equal(t, "31/05/26", d)
        assert.Equal(t, "318.00", a)
        assert.False(t, debit)
    })
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/agent/parser/... -run "TestExtract" -v
```
Expected: compile error

- [ ] **Step 3: Implement banks.go**

```go
// internal/agent/parser/banks.go
package parser

import (
    "fmt"
    "regexp"
    "strings"
)

// ── ICICI Credit Card ───────────────────────────────────────────────────────
// "INR 453.00 spent using ICICI Bank Card XX6009 on 03-Jun-26 on AMAZON PAY IN G."

var iciciCCRe = regexp.MustCompile(
    `(?i)INR\s+([\d,]+(?:\.\d{1,2})?)\s+spent using .* on (\d{2}-\w{3}-\d{2}) on ([^.]+)`)

func ExtractIciciCC(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
    m := iciciCCRe.FindStringSubmatch(sms)
    if m == nil {
        return "", "", "", false, fmt.Errorf("icici_cc: no match")
    }
    return strings.TrimSpace(m[3]), m[2], m[1], true, nil
}

// ── HDFC Debit Card ─────────────────────────────────────────────────────────
// "Spent! INR INR 215 On HDFC Bank Card 2148 At RAZ*Swiggy/Bangalore On 03 Jun,2026 01:18 PM IST"

var hdfcDebitRe = regexp.MustCompile(
    `(?i)Spent!\s+INR\s+INR\s+([\d,]+(?:\.\d{1,2})?)\s+On\s+HDFC\s+Bank\s+Card\s+\d+\s+At\s+(.+?)\s+On\s+(\d{1,2}\s+\w{3},\d{4})`)

func ExtractHdfcDebit(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
    m := hdfcDebitRe.FindStringSubmatch(sms)
    if m == nil {
        return "", "", "", false, fmt.Errorf("hdfc_debit: no match")
    }
    return strings.TrimSpace(m[2]), m[3], m[1], true, nil
}

// ── HDFC Credit Card ────────────────────────────────────────────────────────
// "Spent Rs.341 On HDFC Bank Card 2527 At ZEPTO MARKETPLACE PRIV On 2026-05-21:07:32:56."

var hdfcCCRe = regexp.MustCompile(
    `(?i)Spent\s+Rs\.?([\d,]+(?:\.\d{1,2})?)\s+On\s+HDFC\s+Bank\s+Card\s+\d+\s+At\s+(.+?)\s+On\s+(\d{4}-\d{2}-\d{2})`)

func ExtractHdfcCC(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
    m := hdfcCCRe.FindStringSubmatch(sms)
    if m == nil {
        return "", "", "", false, fmt.Errorf("hdfc_cc: no match")
    }
    return strings.TrimSpace(m[2]), m[3], m[1], true, nil
}

// ── Axis Bank Checking ───────────────────────────────────────────────────────
// "INR 1804.05 debited\nA/c no. XX6386\n03-06-26, 10:21:54\nUPI/P2M/102154212206/IRCTC Rail Web\n..."

var axisChkAmtRe = regexp.MustCompile(`(?i)INR\s+([\d,]+(?:\.\d{1,2})?)\s+(debited|credited)`)
var axisChkDateRe = regexp.MustCompile(`\b(\d{2}-\d{2}-\d{2})\b`)
var axisChkUPIRe = regexp.MustCompile(`(?i)(UPI/[^\n]+)`)

func ExtractAxisChecking(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
    amtM := axisChkAmtRe.FindStringSubmatch(sms)
    if amtM == nil {
        return "", "", "", false, fmt.Errorf("axis_checking: no amount")
    }
    dateM := axisChkDateRe.FindStringSubmatch(sms)
    if dateM == nil {
        return "", "", "", false, fmt.Errorf("axis_checking: no date")
    }
    merchantStr := ""
    if upiM := axisChkUPIRe.FindStringSubmatch(sms); upiM != nil {
        merchantStr = extractUPIMerchant(upiM[1])
    }
    return merchantStr, dateM[1], amtM[1], strings.ToLower(amtM[2]) == "debited", nil
}

// extractUPIMerchant returns the human-readable part after the numeric reference ID.
// "UPI/P2M/102154212206/IRCTC Rail Web" → "IRCTC Rail Web"
func extractUPIMerchant(upiRef string) string {
    parts := strings.Split(upiRef, "/")
    for i, p := range parts {
        if isAllDigits(p) && i+1 < len(parts) {
            return strings.Join(parts[i+1:], " ")
        }
    }
    return parts[len(parts)-1]
}

func isAllDigits(s string) bool {
    if len(s) == 0 {
        return false
    }
    for _, c := range s {
        if c < '0' || c > '9' {
            return false
        }
    }
    return true
}

// ── Axis Bank Credit Card ────────────────────────────────────────────────────
// "Spent INR 210.12\nAxis Bank Card no. XX1610\n08-05-26 18:44:17 IST\nDISTRICT MO\nAvl Limit: ..."

var axisSpentRe = regexp.MustCompile(`(?i)^Spent\s+INR\s+([\d,]+(?:\.\d{1,2})?)`)
var axisDateLineRe = regexp.MustCompile(`^(\d{2}-\d{2}-\d{2})\s`)

func ExtractAxisCC(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
    amtM := axisSpentRe.FindStringSubmatch(sms)
    if amtM == nil {
        return "", "", "", false, fmt.Errorf("axis_cc: no amount")
    }

    lines := strings.Split(strings.TrimSpace(sms), "\n")
    merchantLineIdx := -1
    for i, line := range lines {
        if m := axisDateLineRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
            rawDate = m[1]
            merchantLineIdx = i + 1
            break
        }
    }
    if rawDate == "" || merchantLineIdx < 0 || merchantLineIdx >= len(lines) {
        return "", "", "", false, fmt.Errorf("axis_cc: no date or merchant line")
    }

    return strings.TrimSpace(lines[merchantLineIdx]), rawDate, amtM[1], true, nil
}

// ── IDFC FIRST Bank Checking ─────────────────────────────────────────────────
// Spend:    "Spent Rs.473.00 from A/C XX6977 at ZEPTO MARKETPLACE PRIV on 09/04/26."
// Interest: "Monthly interest of INR.318.00 earned on your Savings A/c XX6977 ... on 31/05/26."

var idfcSpendRe = regexp.MustCompile(
    `(?i)Spent\s+Rs\.?([\d,]+(?:\.\d{1,2})?)\s+from\s+A/C\s+\S+\s+at\s+(.+?)\s+on\s+(\d{2}/\d{2}/\d{2})`)
var idfcInterestRe = regexp.MustCompile(
    `(?i)INR\.?([\d,]+(?:\.\d{1,2})?)\s+earned.*?(\d{2}/\d{2}/\d{2})`)

func ExtractIDFCChecking(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
    if m := idfcSpendRe.FindStringSubmatch(sms); m != nil {
        return strings.TrimSpace(m[2]), m[3], m[1], true, nil
    }
    if m := idfcInterestRe.FindStringSubmatch(sms); m != nil {
        return "Monthly interest", m[2], m[1], false, nil
    }
    return "", "", "", false, fmt.Errorf("idfc_checking: no match")
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/parser/... -run "TestExtract" -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/parser/banks.go internal/agent/parser/banks_test.go
git commit -m "feat(agent): add bank-specific SMS regex extractors (6 banks)"
```

---

## Task 6: Merchant routing

**Files:**
- Create: `internal/agent/parser/merchant.go`
- Create: `internal/agent/parser/merchant_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/parser/merchant_test.go
package parser_test

import (
    "testing"
    "github.com/ananthakumaran/paisa/internal/agent/config"
    "github.com/ananthakumaran/paisa/internal/agent/parser"
    "github.com/stretchr/testify/assert"
)

var testMerchantRules = []config.MerchantRule{
    {Keyword: "swiggy", Account: "Expenses:Food:Hyd", Description: "Food Swiggy"},
    {Keyword: "zomato", Account: "Expenses:Food:Hyd", Description: "Food Zomato"},
    {Keyword: "blink commerce", Account: "Expenses:Groceries:Hyd", Description: "Groceries Blink"},
    {Keyword: "zepto", Account: "Expenses:Groceries:Hyd", Description: "Groceries ZEPTO"},
    {Keyword: "flipkart", Account: "Expenses:Utils:Hyd", Description: "Utils: Flipkart"},
    {Keyword: "district", Account: "Expenses:Entertainment:Hyd", Description: "Entertainment: DISTRICT"},
    {Keyword: "irctc", Account: "Expenses:Travel:Hyd", Description: "Travel"},
    {Keyword: "rail web", Account: "Expenses:Travel:Hyd", Description: "Travel"},
    {Keyword: "amazon", Account: "Expenses:Utils:Hyd", Description: "Utils: Amazon Pay"},
    {Keyword: "monthly interest", Account: "Income:Interest:IDFC6977", Description: "Bank Interest"},
}

func TestRouteMerchant(t *testing.T) {
    cases := []struct {
        merchant    string
        wantAccount string
        wantDesc    string
    }{
        {"RAZ*Swiggy/Bangalore", "Expenses:Food:Hyd", "Food Swiggy"},
        {"ZOMATO/110018759//IN", "Expenses:Food:Hyd", "Food Zomato"},
        {"BLINK COMMERCE PVT L", "Expenses:Groceries:Hyd", "Groceries Blink"},
        {"ZEPTO MARKETPLACE PRIV", "Expenses:Groceries:Hyd", "Groceries ZEPTO"},
        {"FLIPKART", "Expenses:Utils:Hyd", "Utils: Flipkart"},
        {"Ing*Flipkar", "Expenses:Utils:Hyd", "Utils: Flipkart"},
        {"DISTRICT MO", "Expenses:Entertainment:Hyd", "Entertainment: DISTRICT"},
        {"IRCTC Rail Web", "Expenses:Travel:Hyd", "Travel"},
        {"AMAZON PAY IN G", "Expenses:Utils:Hyd", "Utils: Amazon Pay"},
        {"Monthly interest", "Income:Interest:IDFC6977", "Bank Interest"},
    }
    for _, c := range cases {
        acct, desc := parser.RouteMerchant(c.merchant, testMerchantRules)
        assert.Equal(t, c.wantAccount, acct, "merchant=%q", c.merchant)
        assert.Equal(t, c.wantDesc, desc, "merchant=%q", c.merchant)
    }
}

func TestRouteMerchant_NoMatch(t *testing.T) {
    acct, desc := parser.RouteMerchant("UNKNOWN VENDOR", testMerchantRules)
    assert.Equal(t, "", acct)
    assert.Equal(t, "", desc)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/parser/... -run "TestRouteMerchant" -v
```
Expected: compile error

- [ ] **Step 3: Implement merchant.go**

```go
// internal/agent/parser/merchant.go
package parser

import (
    "strings"
    "github.com/ananthakumaran/paisa/internal/agent/config"
)

// RouteMerchant finds the first MerchantRule whose keyword is a case-insensitive
// substring of merchant. Returns the account and description, or empty strings if
// no rule matches (caller should try LLM fallback).
func RouteMerchant(merchant string, rules []config.MerchantRule) (account, description string) {
    lower := strings.ToLower(merchant)
    for _, r := range rules {
        if strings.Contains(lower, strings.ToLower(r.Keyword)) {
            return r.Account, r.Description
        }
    }
    return "", ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/parser/... -run "TestRouteMerchant" -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/parser/merchant.go internal/agent/parser/merchant_test.go
git commit -m "feat(agent): add merchant keyword routing"
```

---

## Task 7: Full parser pipeline

**Files:**
- Create: `internal/agent/parser/parser.go`
- Create: `internal/agent/parser/parser_test.go`

- [ ] **Step 1: Write failing tests covering all 16 example messages**

```go
// internal/agent/parser/parser_test.go
package parser_test

import (
    "testing"
    "github.com/ananthakumaran/paisa/internal/agent/config"
    "github.com/ananthakumaran/paisa/internal/agent/parser"
    "github.com/stretchr/testify/assert"
)

var testAccounts = []config.AccountRule{
    // Fixed routes first
    {Bank: "fixed", Identifiers: []string{"CRD-PMNT", "8860"}, Src: "Assets:Checking:AXIS6386", Destinations: "Liabilities:CreditCard:FK8860", Description: "CC Payment"},
    {Bank: "fixed", Identifiers: []string{"CRD-PMNT", "1610"}, Src: "Assets:Checking:AXIS6386", Destinations: "Liabilities:CreditCard:MyZone1610", Description: "CC Payment"},
    {Bank: "fixed", Identifiers: []string{"CRD-PMNT", "6792"}, Src: "Assets:Checking:AXIS6386", Destinations: "Liabilities:CreditCard:SELECT6792", Description: "CC Payment"},
    {Bank: "fixed", Identifiers: []string{"has been received on your ICICI Bank Credit Card XX6009"}, Src: "Assets:Checking:AXIS6386", Destinations: "Liabilities:CreditCard:ICIC6009", Description: "CC Payment"},
    {Bank: "fixed", Identifiers: []string{"KONDAVEET"}, Src: "Assets:Checking:AXISHARITHA", Destinations: "Assets:Checking:AXIS6386", Description: "Rent from Haritha"},
    // Format routes
    {Bank: "icici_cc", Identifiers: []string{"ICICI Bank Card XX6009"}, Destinations: "Liabilities:CreditCard:ICICI6009"},
    {Bank: "hdfc_debit", Identifiers: []string{"HDFC Bank Card 2148"}, Destinations: "Assets:Checking:FC2148"},
    {Bank: "hdfc_cc", Identifiers: []string{"HDFC Bank Card 2527"}, Destinations: "Liabilities:CreditCard:HDFC2527"},
    {Bank: "axis_checking", Identifiers: []string{"A/c no. XX6386"}, Destinations: "Assets:Checking:AXIS6386"},
    {Bank: "axis_cc", Identifiers: []string{"Card no. XX1610"}, Destinations: "Liabilities:CreditCard:MyZone1610"},
    {Bank: "axis_cc", Identifiers: []string{"Card no. XX6792"}, Destinations: "Liabilities:CreditCard:SELECT6792"},
    {Bank: "axis_cc", Identifiers: []string{"Card no. XX8860"}, Destinations: "Liabilities:CreditCard:FK8860"},
    {Bank: "idfc_checking", Identifiers: []string{"XX6977"}, Destinations: "Assets:Checking:IDFC6977"},
}

var testMerchants = []config.MerchantRule{
    {Keyword: "swiggy", Account: "Expenses:Food:Hyd", Description: "Food Swiggy"},
    {Keyword: "zomato", Account: "Expenses:Food:Hyd", Description: "Food Zomato"},
    {Keyword: "blink commerce", Account: "Expenses:Groceries:Hyd", Description: "Groceries Blink"},
    {Keyword: "zepto", Account: "Expenses:Groceries:Hyd", Description: "Groceries ZEPTO"},
    {Keyword: "flipkart", Account: "Expenses:Utils:Hyd", Description: "Utils: Flipkart"},
    {Keyword: "ing*flipkar", Account: "Expenses:Utils:Hyd", Description: "Utils: Flipkart"},
    {Keyword: "district", Account: "Expenses:Entertainment:Hyd", Description: "Entertainment: DISTRICT"},
    {Keyword: "irctc", Account: "Expenses:Travel:Hyd", Description: "Travel"},
    {Keyword: "rail web", Account: "Expenses:Travel:Hyd", Description: "Travel"},
    {Keyword: "amazon", Account: "Expenses:Utils:Hyd", Description: "Utils: Amazon Pay"},
    {Keyword: "monthly interest", Account: "Income:Interest:IDFC6977", Description: "Bank Interest"},
}

type wantEntry struct{ date, desc, src, amt, dest string }

var parseTests = []struct {
    name string
    sms  string
    want wantEntry
}{
    {
        "icici_cc spend",
        "INR 453.00 spent using ICICI Bank Card XX6009 on 03-Jun-26 on AMAZON PAY IN G. Avl Limit: INR 4,59,509.36. If not you, call 1800 2662/SMS BLOCK 6009 to 9215676766",
        wantEntry{"2026/06/03", "Utils: Amazon Pay", "Liabilities:CreditCard:ICICI6009", "-453.00 INR", "Expenses:Utils:Hyd"},
    },
    {
        "icici_cc payment (fixed)",
        "Payment of Rs 10,468.87 has been received on your ICICI Bank Credit Card XX6009 through Bharat Bill Payment System on 15-MAY-26.",
        wantEntry{"2026/05/15", "CC Payment", "Liabilities:CreditCard:ICIC6009", "10468.87 INR", "Assets:Checking:AXIS6386"},
    },
    {
        "hdfc_debit swiggy",
        "Spent! INR INR 215 On HDFC Bank Card 2148 At RAZ*Swiggy/Bangalore On 03 Jun,2026 01:18 PM IST Bal INR INR 23216.37 To Block, Call 08069858585/SMS BLKGP3 Last4Digits to 5676712",
        wantEntry{"2026/06/03", "Food Swiggy", "Assets:Checking:FC2148", "-215.00 INR", "Expenses:Food:Hyd"},
    },
    {
        "hdfc_debit blink",
        "Spent! INR INR 426 On HDFC Bank Card 2148 At BLINK COMMERCE PVT L On 02 Jun,2026 08:32 PM IST Bal INR INR 23431.37 To Block, Call 08069858585",
        wantEntry{"2026/06/02", "Groceries Blink", "Assets:Checking:FC2148", "-426.00 INR", "Expenses:Groceries:Hyd"},
    },
    {
        "hdfc_debit zomato",
        "Spent! INR INR 327.25 On HDFC Bank Card 2148 At ZOMATO/110018759//IN On 31 May,2026 10:28 PM IST Bal INR INR 9534.37 To Block, Call 08069858585",
        wantEntry{"2026/05/31", "Food Zomato", "Assets:Checking:FC2148", "-327.25 INR", "Expenses:Food:Hyd"},
    },
    {
        "axis_checking irctc",
        "INR 1804.05 debited\nA/c no. XX6386\n03-06-26, 10:21:54\nUPI/P2M/102154212206/IRCTC Rail Web\nNot you? SMS BLOCKUPI Cust ID to 919951860002\nAxis Bank",
        wantEntry{"2026/06/03", "Travel", "Assets:Checking:AXIS6386", "-1804.05 INR", "Expenses:Travel:Hyd"},
    },
    {
        "kondaveet fixed",
        "INR 30000.00 credited\nA/c no. XX6386\n01-06-26, 09:36:03 IST\nUPI/P2A/067278619954/KONDAVEET/UTIB/Paym - Axis Bank",
        wantEntry{"2026/06/01", "Rent from Haritha", "Assets:Checking:AXIS6386", "30000.00 INR", "Assets:Checking:AXISHARITHA"},
    },
    {
        "idfc interest",
        "Monthly interest of INR.318.00 earned on your Savings A/c XX6977 has been credited to your A/C on 31/05/26. New bal: INR.1,50,101.05. IDFC FIRST Bank",
        wantEntry{"2026/05/31", "Bank Interest", "Assets:Checking:IDFC6977", "318.00 INR", "Income:Interest:IDFC6977"},
    },
    {
        "idfc zepto",
        "Spent Rs.473.00 from A/C XX6977 at ZEPTO MARKETPLACE PRIV on 09/04/26. Not you? Call 180010888/SMS BLOCK (last 4 digit of card) to 5676732. IDFC FIRST Bank",
        wantEntry{"2026/04/09", "Groceries ZEPTO", "Assets:Checking:IDFC6977", "-473.00 INR", "Expenses:Groceries:Hyd"},
    },
    {
        "hdfc_cc zepto",
        "Spent Rs.341 On HDFC Bank Card 2527 At ZEPTO MARKETPLACE PRIV On 2026-05-21:07:32:56.Not You? To Block+Reissue Call 18002586161/SMS BLOCK CC 2527 to 7308080808",
        wantEntry{"2026/05/21", "Groceries ZEPTO", "Liabilities:CreditCard:HDFC2527", "-341.00 INR", "Expenses:Groceries:Hyd"},
    },
    {
        "axis_cc district (1610)",
        "Spent INR 210.12\nAxis Bank Card no. XX1610\n08-05-26 18:44:17 IST\nDISTRICT MO\nAvl Limit: INR 1389000.78\nNot you? SMS BLOCK 1610 to 919951860002",
        wantEntry{"2026/05/08", "Entertainment: DISTRICT", "Liabilities:CreditCard:MyZone1610", "-210.12 INR", "Expenses:Entertainment:Hyd"},
    },
    {
        "axis_cc flipkart (6792)",
        "Spent INR 11864\nAxis Bank Card no. XX6792\n23-05-26 23:30:19 IST\nFLIPKART\nAvl Limit: INR 1324182.46\nNot you? SMS BLOCK 6792 to 919951860002",
        wantEntry{"2026/05/23", "Utils: Flipkart", "Liabilities:CreditCard:SELECT6792", "-11864.00 INR", "Expenses:Utils:Hyd"},
    },
    {
        "axis_cc flipkart (8860)",
        "Spent INR 3468\nAxis Bank Card no. XX8860\n01-06-26 15:22:40 IST\nIng*Flipkar\nAvl Limit: INR 1320714.46\nNot you? SMS BLOCK 8860 to 919951860002",
        wantEntry{"2026/06/01", "Utils: Flipkart", "Liabilities:CreditCard:FK8860", "-3468.00 INR", "Expenses:Utils:Hyd"},
    },
    {
        "crd-pmnt fk8860 (fixed)",
        "Debit INR 1417.00\nAxis Bank A/c XX6386\n02-06-26 10:47:13\nCRD-PMNT-533467****8860\nWhatsApp BAL to 917036165000\nNot You? SMS BLOCKALL CustID to 919951860002",
        wantEntry{"2026/06/02", "CC Payment", "Liabilities:CreditCard:FK8860", "1417.00 INR", "Assets:Checking:AXIS6386"},
    },
    {
        "crd-pmnt myzone1610 (fixed)",
        "Debit INR 1417.00\nAxis Bank A/c XX6386\n02-06-26 10:47:13\nCRD-PMNT-530562****1610\nWhatsApp BAL to 917036165000\nNot You? SMS BLOCKALL CustID to 919951860002",
        wantEntry{"2026/06/02", "CC Payment", "Liabilities:CreditCard:MyZone1610", "1417.00 INR", "Assets:Checking:AXIS6386"},
    },
    {
        "crd-pmnt select6792 (fixed)",
        "Debit INR 1417.00\nAxis Bank A/c XX6386\n02-06-26 10:47:13\nCRD-PMNT-411146****6792\nWhatsApp BAL to 917036165000\nNot You? SMS BLOCKALL CustID to 919951860002",
        wantEntry{"2026/06/02", "CC Payment", "Liabilities:CreditCard:SELECT6792", "1417.00 INR", "Assets:Checking:AXIS6386"},
    },
}

func TestParse_AllExamples(t *testing.T) {
    for _, tc := range parseTests {
        t.Run(tc.name, func(t *testing.T) {
            rule, err := parser.Classify(tc.sms, testAccounts)
            assert.NoError(t, err, "classify failed")
            entry, err := parser.Parse(tc.sms, rule, testMerchants)
            assert.NoError(t, err, "parse failed")
            assert.Equal(t, tc.want.date, entry.Date, "date")
            assert.Equal(t, tc.want.desc, entry.Desc, "desc")
            assert.Equal(t, tc.want.src, entry.Src, "src")
            assert.Equal(t, tc.want.amt, entry.Amt, "amt")
            assert.Equal(t, tc.want.dest, entry.Dest, "dest")
        })
    }
}

func TestClassify_NoMatch(t *testing.T) {
    _, err := parser.Classify("unrelated text", testAccounts)
    assert.Error(t, err)
}

func TestClassify_FixedBeforeFormat(t *testing.T) {
    // CRD-PMNT + 8860 matches fixed route, not axis_checking
    sms := "Debit INR 1417.00\nAxis Bank A/c XX6386\n02-06-26 10:47:13\nCRD-PMNT-533467****8860"
    rule, err := parser.Classify(sms, testAccounts)
    assert.NoError(t, err)
    assert.Equal(t, "fixed", rule.Bank)
    assert.Equal(t, "Liabilities:CreditCard:FK8860", rule.Destinations)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/agent/parser/... -run "TestParse|TestClassify" -v
```
Expected: compile error

- [ ] **Step 3: Implement parser.go**

```go
// internal/agent/parser/parser.go
package parser

import (
    "fmt"
    "strings"
    "github.com/ananthakumaran/paisa/internal/agent/config"
    "github.com/ananthakumaran/paisa/internal/agent/ledger"
)

// Classify scans the accounts list top-to-bottom and returns the first rule
// where ALL identifiers appear in the SMS. Fixed routes win because they are
// listed first in the YAML.
func Classify(sms string, accounts []config.AccountRule) (*config.AccountRule, error) {
    for i, rule := range accounts {
        if matchesAll(sms, rule.Identifiers) {
            return &accounts[i], nil
        }
    }
    return nil, fmt.Errorf("no matching account rule for SMS")
}

func matchesAll(sms string, identifiers []string) bool {
    for _, id := range identifiers {
        if !strings.Contains(sms, id) {
            return false
        }
    }
    return true
}

// Parse builds a ledger Entry from an SMS given the matched AccountRule.
// Fixed routes derive all fields from the YAML rule + generic amount/date extractor.
// Format routes call the bank-specific extractor then apply merchant routing.
func Parse(sms string, rule *config.AccountRule, merchants []config.MerchantRule) (*ledger.Entry, error) {
    if rule.Bank == "fixed" {
        return parseFixed(sms, rule)
    }
    return parseFormat(sms, rule, merchants)
}

func parseFixed(sms string, rule *config.AccountRule) (*ledger.Entry, error) {
    rawAmt, _, err := ExtractAmountFromSMS(sms)
    if err != nil {
        return nil, fmt.Errorf("fixed route: %w", err)
    }
    norm, err := NormaliseAmount(rawAmt)
    if err != nil {
        return nil, err
    }
    rawDate, err := ExtractDateFromSMS(sms)
    if err != nil {
        return nil, fmt.Errorf("fixed route: %w", err)
    }
    date, err := NormaliseDate(rawDate)
    if err != nil {
        return nil, err
    }
    // Fixed routes always use positive amount on destinations:
    // the sign is implied by the account pair (e.g. CC gets payment = positive).
    return &ledger.Entry{
        Date: date,
        Desc: rule.Description,
        Src:  rule.Destinations,
        Amt:  FormatEntryAmount(norm, false),
        Dest: rule.Src,
    }, nil
}

func parseFormat(sms string, rule *config.AccountRule, merchants []config.MerchantRule) (*ledger.Entry, error) {
    var merchant, rawDate, rawAmt string
    var isDebit bool
    var err error

    switch rule.Bank {
    case "icici_cc":
        merchant, rawDate, rawAmt, isDebit, err = ExtractIciciCC(sms)
    case "hdfc_debit":
        merchant, rawDate, rawAmt, isDebit, err = ExtractHdfcDebit(sms)
    case "hdfc_cc":
        merchant, rawDate, rawAmt, isDebit, err = ExtractHdfcCC(sms)
    case "axis_checking":
        merchant, rawDate, rawAmt, isDebit, err = ExtractAxisChecking(sms)
    case "axis_cc":
        merchant, rawDate, rawAmt, isDebit, err = ExtractAxisCC(sms)
    case "idfc_checking":
        merchant, rawDate, rawAmt, isDebit, err = ExtractIDFCChecking(sms)
    default:
        // Unknown bank: use generic extractor, LLM will fill merchant/dest
        rawAmt, isDebit, err = ExtractAmountFromSMS(sms)
        if err != nil {
            return nil, fmt.Errorf("unknown bank %q: %w", rule.Bank, err)
        }
        rawDate, err = ExtractDateFromSMS(sms)
        if err != nil {
            return nil, fmt.Errorf("unknown bank %q date: %w", rule.Bank, err)
        }
    }
    if err != nil {
        return nil, err
    }

    norm, err := NormaliseAmount(rawAmt)
    if err != nil {
        return nil, err
    }
    date, err := NormaliseDate(rawDate)
    if err != nil {
        return nil, err
    }

    account, desc := RouteMerchant(merchant, merchants)

    return &ledger.Entry{
        Date: date,
        Desc: desc,
        Src:  rule.Destinations,
        Amt:  FormatEntryAmount(norm, isDebit),
        Dest: account,
    }, nil
}
```

- [ ] **Step 4: Run all parser tests**

```bash
go test ./internal/agent/parser/... -v
```
Expected: all PASS (includes dates, amounts, banks, merchant, parser tests)

- [ ] **Step 5: Commit**

```bash
git add internal/agent/parser/parser.go internal/agent/parser/parser_test.go
git commit -m "feat(agent): add full SMS parser pipeline — Classify + Parse, all 16 examples pass"
```

---

## Task 8: LLM Ollama fallback

**Files:**
- Create: `internal/agent/llm/ollama.go`
- Create: `internal/agent/llm/ollama_test.go`

- [ ] **Step 1: Write failing test with mock HTTP server**

```go
// internal/agent/llm/ollama_test.go
package llm_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "github.com/ananthakumaran/paisa/internal/agent/config"
    "github.com/ananthakumaran/paisa/internal/agent/ledger"
    "github.com/ananthakumaran/paisa/internal/agent/llm"
    "github.com/stretchr/testify/assert"
)

func TestFillMissing_FillsBothFields(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]string{
            "response": `{"desc": "Grocery Shopping", "dest": "Expenses:Groceries:Hyd"}`,
        })
    }))
    defer srv.Close()

    entry := &ledger.Entry{Date: "2026/06/03", Src: "Assets:Checking:FC2148", Amt: "-500.00 INR"}
    cfg := config.OllamaConfig{URL: srv.URL, Model: "test-model"}
    err := llm.FillMissing("some unknown SMS", entry, cfg)
    assert.NoError(t, err)
    assert.Equal(t, "Grocery Shopping", entry.Desc)
    assert.Equal(t, "Expenses:Groceries:Hyd", entry.Dest)
}

func TestFillMissing_PreservesExistingFields(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]string{
            "response": `{"desc": "LLM guess", "dest": "Expenses:Unknown"}`,
        })
    }))
    defer srv.Close()

    entry := &ledger.Entry{Desc: "Already set", Dest: "Already:Set"}
    cfg := config.OllamaConfig{URL: srv.URL, Model: "test-model"}
    err := llm.FillMissing("sms", entry, cfg)
    assert.NoError(t, err)
    // Existing non-empty fields must not be overwritten
    assert.Equal(t, "Already set", entry.Desc)
    assert.Equal(t, "Already:Set", entry.Dest)
}

func TestFillMissing_LLMMarkdownWrapped(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]string{
            "response": "```json\n{\"desc\": \"Travel\", \"dest\": \"Expenses:Travel:Hyd\"}\n```",
        })
    }))
    defer srv.Close()

    entry := &ledger.Entry{}
    cfg := config.OllamaConfig{URL: srv.URL, Model: "test-model"}
    err := llm.FillMissing("sms", entry, cfg)
    assert.NoError(t, err)
    assert.Equal(t, "Travel", entry.Desc)
    assert.Equal(t, "Expenses:Travel:Hyd", entry.Dest)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/llm/... -v
```
Expected: compile error

- [ ] **Step 3: Implement ollama.go**

```go
// internal/agent/llm/ollama.go
package llm

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"

    "github.com/ananthakumaran/paisa/internal/agent/config"
    "github.com/ananthakumaran/paisa/internal/agent/ledger"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type generateRequest struct {
    Model  string `json:"model"`
    Prompt string `json:"prompt"`
    Stream bool   `json:"stream"`
}

type generateResponse struct {
    Response string `json:"response"`
}

type fillResult struct {
    Desc string `json:"desc"`
    Dest string `json:"dest"`
}

// FillMissing calls Ollama to populate empty Desc and/or Dest fields in the entry.
// Only overwrites fields that are currently empty.
func FillMissing(sms string, entry *ledger.Entry, cfg config.OllamaConfig) error {
    var missing []string
    if entry.Desc == "" {
        missing = append(missing, "desc (short payee description, e.g. \"Food Swiggy\")")
    }
    if entry.Dest == "" {
        missing = append(missing, "dest (ledger account, e.g. \"Expenses:Food:Hyd\")")
    }
    if len(missing) == 0 {
        return nil
    }

    prompt := fmt.Sprintf(
        "You are a personal finance assistant. Given this bank SMS, extract only the missing fields.\n\nSMS: %s\n\nAlready known: date=%s, src=%s, amt=%s\n\nMissing: %s\n\nReply with ONLY a JSON object like {\"desc\": \"...\", \"dest\": \"...\"} containing the missing fields. No explanation.",
        sms, entry.Date, entry.Src, entry.Amt, strings.Join(missing, ", "),
    )

    body, _ := json.Marshal(generateRequest{Model: cfg.Model, Prompt: prompt, Stream: false})
    resp, err := httpClient.Post(cfg.URL+"/api/generate", "application/json", bytes.NewReader(body))
    if err != nil {
        return fmt.Errorf("ollama request: %w", err)
    }
    defer resp.Body.Close()
    data, _ := io.ReadAll(resp.Body)

    var gr generateResponse
    if err := json.Unmarshal(data, &gr); err != nil {
        return fmt.Errorf("ollama response parse: %w", err)
    }

    jsonStr := extractJSON(gr.Response)
    var result fillResult
    if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
        return fmt.Errorf("ollama result parse: %w", err)
    }

    if entry.Desc == "" && result.Desc != "" {
        entry.Desc = result.Desc
    }
    if entry.Dest == "" && result.Dest != "" {
        entry.Dest = result.Dest
    }
    return nil
}

// extractJSON finds the first {...} JSON object in a string (handles markdown code blocks).
func extractJSON(s string) string {
    start := strings.Index(s, "{")
    end := strings.LastIndex(s, "}")
    if start < 0 || end <= start {
        return "{}"
    }
    return s[start : end+1]
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/llm/... -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/llm/ollama.go internal/agent/llm/ollama_test.go
git commit -m "feat(agent): add Ollama LLM fallback for missing Desc/Dest fields"
```

---

## Task 9: Ledger file appender

**Files:**
- Create: `internal/agent/ledger/appender.go`
- Create: `internal/agent/ledger/appender_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/ledger/appender_test.go
package ledger_test

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
    "github.com/ananthakumaran/paisa/internal/agent/ledger"
    "github.com/stretchr/testify/assert"
)

func TestEnsureFile_CreatesFile(t *testing.T) {
    dir := t.TempDir()
    err := ledger.EnsureFile(dir)
    assert.NoError(t, err)
    _, err = os.Stat(filepath.Join(dir, "auto-import.ledger"))
    assert.NoError(t, err, "auto-import.ledger should exist")
}

func TestEnsureFile_AddsInclude(t *testing.T) {
    dir := t.TempDir()
    mainJournal := filepath.Join(dir, "main.ledger")
    err := os.WriteFile(mainJournal, []byte("; main journal\n"), 0644)
    assert.NoError(t, err)

    err = ledger.EnsureFile(dir)
    assert.NoError(t, err)

    content, _ := os.ReadFile(mainJournal)
    assert.Contains(t, string(content), "include auto-import.ledger")
}

func TestEnsureFile_IdempotentInclude(t *testing.T) {
    dir := t.TempDir()
    mainJournal := filepath.Join(dir, "main.ledger")
    err := os.WriteFile(mainJournal, []byte("include auto-import.ledger\n"), 0644)
    assert.NoError(t, err)

    err = ledger.EnsureFile(dir)
    assert.NoError(t, err)

    content, _ := os.ReadFile(mainJournal)
    assert.Equal(t, 1, strings.Count(string(content), "include auto-import.ledger"))
}

func TestAppend_WritesLedgerBlock(t *testing.T) {
    dir := t.TempDir()
    err := ledger.EnsureFile(dir)
    assert.NoError(t, err)

    e := &ledger.Entry{
        Date: "2026/06/03",
        Desc: "Food Swiggy",
        Src:  "Assets:Checking:FC2148",
        Amt:  "-215.00 INR",
        Dest: "Expenses:Food:Hyd",
    }
    err = ledger.Append(dir, e)
    assert.NoError(t, err)

    content, _ := os.ReadFile(filepath.Join(dir, "auto-import.ledger"))
    s := string(content)
    assert.Contains(t, s, "2026/06/03 Food Swiggy")
    assert.Contains(t, s, "Assets:Checking:FC2148")
    assert.Contains(t, s, "-215.00 INR")
    assert.Contains(t, s, "Expenses:Food:Hyd")
}

func TestIsDuplicate_DetectsSameEntry(t *testing.T) {
    dir := t.TempDir()
    ledger.EnsureFile(dir)
    e := &ledger.Entry{Date: "2026/06/03", Desc: "Food Swiggy", Src: "Assets:Checking:FC2148", Amt: "-215.00 INR", Dest: "Expenses:Food:Hyd"}
    ledger.Append(dir, e)

    dup, err := ledger.IsDuplicate(dir, e)
    assert.NoError(t, err)
    assert.True(t, dup)
}

func TestIsDuplicate_DifferentEntry(t *testing.T) {
    dir := t.TempDir()
    ledger.EnsureFile(dir)
    e1 := &ledger.Entry{Date: "2026/06/03", Desc: "Food Swiggy", Src: "Assets:Checking:FC2148", Amt: "-215.00 INR", Dest: "Expenses:Food:Hyd"}
    e2 := &ledger.Entry{Date: "2026/06/04", Desc: "Food Zomato", Src: "Assets:Checking:FC2148", Amt: "-327.25 INR", Dest: "Expenses:Food:Hyd"}
    ledger.Append(dir, e1)

    dup, err := ledger.IsDuplicate(dir, e2)
    assert.NoError(t, err)
    assert.False(t, dup)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/ledger/... -run "TestEnsure|TestAppend|TestIsDuplicate" -v
```
Expected: compile error

- [ ] **Step 3: Implement appender.go**

```go
// internal/agent/ledger/appender.go
package ledger

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "sync"
)

const autoImportFile = "auto-import.ledger"
const dupScanLines = 500

var appendMu sync.Mutex

// EnsureFile creates auto-import.ledger if absent, then ensures the main
// journal in journalDir has an include directive for it.
func EnsureFile(journalDir string) error {
    autoPath := filepath.Join(journalDir, autoImportFile)
    if _, err := os.Stat(autoPath); os.IsNotExist(err) {
        if err := os.WriteFile(autoPath, []byte("; Auto-imported transactions\n\n"), 0644); err != nil {
            return fmt.Errorf("create %s: %w", autoImportFile, err)
        }
    }
    return ensureInclude(journalDir)
}

func ensureInclude(journalDir string) error {
    entries, err := os.ReadDir(journalDir)
    if err != nil {
        return err
    }
    for _, e := range entries {
        name := e.Name()
        if name == autoImportFile {
            continue
        }
        if strings.HasSuffix(name, ".ledger") || strings.HasSuffix(name, ".journal") {
            mainPath := filepath.Join(journalDir, name)
            content, err := os.ReadFile(mainPath)
            if err != nil {
                return err
            }
            includeLine := fmt.Sprintf("include %s", autoImportFile)
            if strings.Contains(string(content), includeLine) {
                return nil
            }
            f, err := os.OpenFile(mainPath, os.O_APPEND|os.O_WRONLY, 0644)
            if err != nil {
                return err
            }
            defer f.Close()
            _, err = fmt.Fprintf(f, "\n%s\n", includeLine)
            return err
        }
    }
    return nil
}

// IsDuplicate scans the last dupScanLines lines of auto-import.ledger for an
// entry with the same date, src account, and amount.
func IsDuplicate(journalDir string, e *Entry) (bool, error) {
    autoPath := filepath.Join(journalDir, autoImportFile)
    data, err := os.ReadFile(autoPath)
    if os.IsNotExist(err) {
        return false, nil
    }
    if err != nil {
        return false, err
    }
    lines := strings.Split(string(data), "\n")
    start := 0
    if len(lines) > dupScanLines {
        start = len(lines) - dupScanLines
    }
    for i, line := range lines[start:] {
        if strings.HasPrefix(line, e.Date) {
            end := i + 4
            if end > len(lines[start:]) {
                end = len(lines[start:])
            }
            for _, nextLine := range lines[start:][i+1 : end] {
                if strings.Contains(nextLine, e.Src) && strings.Contains(nextLine, e.Amt) {
                    return true, nil
                }
            }
        }
    }
    return false, nil
}

// Append writes the ledger entry block to auto-import.ledger under a mutex.
func Append(journalDir string, e *Entry) error {
    appendMu.Lock()
    defer appendMu.Unlock()
    autoPath := filepath.Join(journalDir, autoImportFile)
    f, err := os.OpenFile(autoPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
    if err != nil {
        return fmt.Errorf("open %s: %w", autoImportFile, err)
    }
    defer f.Close()
    _, err = fmt.Fprintf(f, "\n%s\n", e.Format())
    return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/ledger/... -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/ledger/appender.go internal/agent/ledger/appender_test.go
git commit -m "feat(agent): add ledger file appender with duplicate detection"
```

---

## Task 10: Approval state machine

**Files:**
- Create: `internal/agent/approval/state.go`
- Create: `internal/agent/approval/state_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/approval/state_test.go
package approval_test

import (
    "sync"
    "testing"
    "github.com/ananthakumaran/paisa/internal/agent/approval"
    "github.com/ananthakumaran/paisa/internal/agent/ledger"
    "github.com/stretchr/testify/assert"
)

func TestStore_SetAndGet(t *testing.T) {
    s := approval.NewStore()
    p := &approval.Pending{
        Entry:     ledger.Entry{Desc: "Food Swiggy"},
        ChatID:    42,
        MessageID: 100,
        Status:    approval.StatusPending,
    }
    s.Set(p)
    got := s.Get(100)
    assert.NotNil(t, got)
    assert.Equal(t, "Food Swiggy", got.Entry.Desc)
}

func TestStore_GetMissing(t *testing.T) {
    s := approval.NewStore()
    assert.Nil(t, s.Get(999))
}

func TestStore_SetEditing(t *testing.T) {
    s := approval.NewStore()
    s.Set(&approval.Pending{MessageID: 1, ChatID: 42, Status: approval.StatusPending})
    s.SetEditing(1)
    assert.Equal(t, approval.StatusEditing, s.Get(1).Status)
}

func TestStore_GetEditingByChatID(t *testing.T) {
    s := approval.NewStore()
    s.Set(&approval.Pending{MessageID: 1, ChatID: 42, Status: approval.StatusPending})
    s.SetEditing(1)
    p := s.GetEditingByChatID(42)
    assert.NotNil(t, p)
    assert.Equal(t, 1, p.MessageID)
    // Different chatID returns nil
    assert.Nil(t, s.GetEditingByChatID(99))
}

func TestStore_Delete(t *testing.T) {
    s := approval.NewStore()
    s.Set(&approval.Pending{MessageID: 1})
    s.Delete(1)
    assert.Nil(t, s.Get(1))
}

func TestStore_ConcurrentAccess(t *testing.T) {
    s := approval.NewStore()
    var wg sync.WaitGroup
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            s.Set(&approval.Pending{MessageID: id})
            s.Get(id)
            s.Delete(id)
        }(i)
    }
    wg.Wait()
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/approval/... -v
```
Expected: compile error

- [ ] **Step 3: Implement state.go**

```go
// internal/agent/approval/state.go
package approval

import (
    "sync"
    "github.com/ananthakumaran/paisa/internal/agent/ledger"
)

type Status string

const (
    StatusPending Status = "pending"
    StatusEditing Status = "editing"
)

type Pending struct {
    Entry     ledger.Entry
    ChatID    int64
    MessageID int
    Status    Status
}

type Store struct {
    mu    sync.Mutex
    items map[int]*Pending
}

func NewStore() *Store {
    return &Store{items: make(map[int]*Pending)}
}

func (s *Store) Set(p *Pending) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.items[p.MessageID] = p
}

func (s *Store) Get(messageID int) *Pending {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.items[messageID]
}

func (s *Store) SetEditing(messageID int) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if p, ok := s.items[messageID]; ok {
        p.Status = StatusEditing
    }
}

// GetEditingByChatID finds the pending entry for a chat that is in editing state.
// Used to route the user's text reply to the correct pending entry.
func (s *Store) GetEditingByChatID(chatID int64) *Pending {
    s.mu.Lock()
    defer s.mu.Unlock()
    for _, p := range s.items {
        if p.ChatID == chatID && p.Status == StatusEditing {
            return p
        }
    }
    return nil
}

func (s *Store) Delete(messageID int) {
    s.mu.Lock()
    defer s.mu.Unlock()
    delete(s.items, messageID)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/approval/... -race -v
```
Expected: all PASS (including race detector)

- [ ] **Step 5: Commit**

```bash
git add internal/agent/approval/state.go internal/agent/approval/state_test.go
git commit -m "feat(agent): add in-memory approval state machine"
```

---

## Task 11: Telegram formatting + edit reply parsing

**Files:**
- Create: `internal/agent/telegram/format.go`
- Create: `internal/agent/telegram/format_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/telegram/format_test.go
package telegram_test

import (
    "testing"
    "github.com/ananthakumaran/paisa/internal/agent/ledger"
    "github.com/ananthakumaran/paisa/internal/agent/telegram"
    "github.com/stretchr/testify/assert"
)

var testEntry = ledger.Entry{
    Date: "2026/06/03",
    Desc: "Food Swiggy",
    Src:  "Assets:Checking:FC2148",
    Amt:  "-215.00 INR",
    Dest: "Expenses:Food:Hyd",
}

func TestFormatDraft(t *testing.T) {
    msg := telegram.FormatDraft(testEntry)
    assert.Contains(t, msg, "📨 New Transaction")
    assert.Contains(t, msg, "desc: Food Swiggy")
    assert.Contains(t, msg, "date: 2026/06/03")
    assert.Contains(t, msg, "src:  Assets:Checking:FC2148")
    assert.Contains(t, msg, "amt:  -215.00 INR")
    assert.Contains(t, msg, "dest: Expenses:Food:Hyd")
}

func TestFormatEditTemplate(t *testing.T) {
    msg := telegram.FormatEditTemplate(testEntry)
    assert.Contains(t, msg, "Edit and send back")
    assert.Contains(t, msg, "desc: Food Swiggy")
}

func TestParseEditReply_PartialUpdate(t *testing.T) {
    reply := "desc: Groceries Zepto\ndest: Expenses:Groceries:Hyd"
    result := telegram.ParseEditReply(reply, testEntry)
    assert.Equal(t, "Groceries Zepto", result.Desc)
    assert.Equal(t, "Expenses:Groceries:Hyd", result.Dest)
    // Unchanged fields preserved
    assert.Equal(t, "2026/06/03", result.Date)
    assert.Equal(t, "Assets:Checking:FC2148", result.Src)
    assert.Equal(t, "-215.00 INR", result.Amt)
}

func TestParseEditReply_AllFields(t *testing.T) {
    reply := "desc: New Desc\ndate: 2026/06/10\nsrc: Liabilities:CC:HDFC2527\namt: -999.00 INR\ndest: Expenses:Utils:Hyd"
    result := telegram.ParseEditReply(reply, testEntry)
    assert.Equal(t, "New Desc", result.Desc)
    assert.Equal(t, "2026/06/10", result.Date)
    assert.Equal(t, "Liabilities:CC:HDFC2527", result.Src)
    assert.Equal(t, "-999.00 INR", result.Amt)
    assert.Equal(t, "Expenses:Utils:Hyd", result.Dest)
}

func TestParseEditReply_IgnoresBlankLines(t *testing.T) {
    reply := "\ndesc: Updated\n\n"
    result := telegram.ParseEditReply(reply, testEntry)
    assert.Equal(t, "Updated", result.Desc)
    assert.Equal(t, testEntry.Date, result.Date)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/telegram/... -run "TestFormat|TestParseEdit" -v
```
Expected: compile error

- [ ] **Step 3: Implement format.go**

```go
// internal/agent/telegram/format.go
package telegram

import (
    "fmt"
    "strings"
    "github.com/ananthakumaran/paisa/internal/agent/ledger"
)

// FormatDraft renders an Entry as the Telegram approval draft message.
func FormatDraft(e ledger.Entry) string {
    return fmt.Sprintf("📨 New Transaction\n\ndesc: %s\ndate: %s\nsrc:  %s\namt:  %s\ndest: %s",
        e.Desc, e.Date, e.Src, e.Amt, e.Dest)
}

// FormatEditTemplate renders the editable 5-field block sent after ✏️ Edit is tapped.
func FormatEditTemplate(e ledger.Entry) string {
    return fmt.Sprintf("Edit and send back:\n\ndesc: %s\ndate: %s\nsrc:  %s\namt:  %s\ndest: %s",
        e.Desc, e.Date, e.Src, e.Amt, e.Dest)
}

// ParseEditReply merges changed key:value lines from a Telegram reply into an existing Entry.
// Only lines with a recognised key are applied; unrecognised lines are ignored.
// Existing fields are preserved if not present in the reply.
func ParseEditReply(text string, existing ledger.Entry) ledger.Entry {
    result := existing
    for _, line := range strings.Split(text, "\n") {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        idx := strings.Index(line, ":")
        if idx < 0 {
            continue
        }
        key := strings.TrimSpace(line[:idx])
        val := strings.TrimSpace(line[idx+1:])
        switch strings.ToLower(key) {
        case "desc":
            result.Desc = val
        case "date":
            result.Date = val
        case "src":
            result.Src = val
        case "amt":
            result.Amt = val
        case "dest":
            result.Dest = val
        }
    }
    return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/agent/telegram/... -run "TestFormat|TestParseEdit" -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/telegram/format.go internal/agent/telegram/format_test.go
git commit -m "feat(agent): add Telegram draft formatting and edit reply parsing"
```

---

## Task 12: Telegram bot HTTP client

**Files:**
- Create: `internal/agent/telegram/bot.go`

No unit tests for this file — it wraps the external Telegram Bot API. Verify manually after Task 13.

- [ ] **Step 1: Implement bot.go**

```go
// internal/agent/telegram/bot.go
package telegram

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

// Bot wraps the Telegram Bot API using raw HTTP (no external dependency).
type Bot struct {
    Token  string
    ChatID int64
    offset int
    client *http.Client
}

func NewBot(token string, chatID int64) *Bot {
    return &Bot{
        Token:  token,
        ChatID: chatID,
        client: &http.Client{Timeout: 40 * time.Second},
    }
}

// Telegram API types

type Update struct {
    UpdateID      int            `json:"update_id"`
    Message       *Message       `json:"message"`
    CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Message struct {
    MessageID int    `json:"message_id"`
    Chat      Chat   `json:"chat"`
    Text      string `json:"text"`
}

type Chat struct {
    ID int64 `json:"id"`
}

type CallbackQuery struct {
    ID      string   `json:"id"`
    Message *Message `json:"message"`
    Data    string   `json:"data"`
}

type apiResponse struct {
    OK     bool            `json:"ok"`
    Result json.RawMessage `json:"result"`
}

type inlineKeyboard struct {
    InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

type inlineButton struct {
    Text         string `json:"text"`
    CallbackData string `json:"callback_data"`
}

func (b *Bot) apiURL(method string) string {
    return fmt.Sprintf("https://api.telegram.org/bot%s/%s", b.Token, method)
}

func (b *Bot) post(method string, payload any) (json.RawMessage, error) {
    body, _ := json.Marshal(payload)
    resp, err := b.client.Post(b.apiURL(method), "application/json", bytes.NewReader(body))
    if err != nil {
        return nil, fmt.Errorf("%s: %w", method, err)
    }
    defer resp.Body.Close()
    data, _ := io.ReadAll(resp.Body)
    var ar apiResponse
    if err := json.Unmarshal(data, &ar); err != nil {
        return nil, fmt.Errorf("%s parse response: %w", method, err)
    }
    if !ar.OK {
        return nil, fmt.Errorf("%s api error: %s", method, string(data))
    }
    return ar.Result, nil
}

// Poll fetches pending updates via long-polling (30s timeout). Advances offset.
func (b *Bot) Poll() ([]Update, error) {
    result, err := b.post("getUpdates", map[string]any{
        "offset":  b.offset,
        "timeout": 30,
    })
    if err != nil {
        return nil, err
    }
    var updates []Update
    if err := json.Unmarshal(result, &updates); err != nil {
        return nil, err
    }
    for _, u := range updates {
        if u.UpdateID >= b.offset {
            b.offset = u.UpdateID + 1
        }
    }
    return updates, nil
}

// SendDraft sends the approval draft with ✅ Approve / ✏️ Edit / ⏭ Skip buttons.
// Returns the sent message's ID for later editing.
func (b *Bot) SendDraft(text string) (int, error) {
    return b.sendWithKeyboard(text, [][]inlineButton{{
        {Text: "✅ Approve", CallbackData: "approve"},
        {Text: "✏️ Edit", CallbackData: "edit"},
        {Text: "⏭ Skip", CallbackData: "skip"},
    }})
}

// SendDraftDuplicate sends the duplicate-warning draft with ✅ Post anyway / ⏭ Skip.
func (b *Bot) SendDraftDuplicate(text string) (int, error) {
    warning := "⚠️ Possible duplicate — matching entry exists. Post anyway?\n\n" + text
    return b.sendWithKeyboard(warning, [][]inlineButton{{
        {Text: "✅ Post anyway", CallbackData: "approve"},
        {Text: "⏭ Skip", CallbackData: "skip"},
    }})
}

func (b *Bot) sendWithKeyboard(text string, buttons [][]inlineButton) (int, error) {
    result, err := b.post("sendMessage", map[string]any{
        "chat_id":      b.ChatID,
        "text":         text,
        "reply_markup": inlineKeyboard{InlineKeyboard: buttons},
    })
    if err != nil {
        return 0, err
    }
    var msg Message
    if err := json.Unmarshal(result, &msg); err != nil {
        return 0, err
    }
    return msg.MessageID, nil
}

// SendText sends a plain text message (used for edit template and error messages).
func (b *Bot) SendText(text string) error {
    _, err := b.post("sendMessage", map[string]any{
        "chat_id": b.ChatID,
        "text":    text,
    })
    return err
}

// EditMessage replaces the text of an existing message (removes inline keyboard).
func (b *Bot) EditMessage(messageID int, text string) error {
    _, err := b.post("editMessageText", map[string]any{
        "chat_id":    b.ChatID,
        "message_id": messageID,
        "text":       text,
    })
    return err
}

// AnswerCallback acknowledges a callback query (removes the loading spinner on the button).
func (b *Bot) AnswerCallback(callbackID string) error {
    _, err := b.post("answerCallbackQuery", map[string]any{
        "callback_query_id": callbackID,
    })
    return err
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/agent/telegram/...
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/agent/telegram/bot.go
git commit -m "feat(agent): add Telegram Bot API client (raw HTTP, no dependency)"
```

---

## Task 13: Main loop + binary entry point

**Files:**
- Create: `cmd/paisa-agent/main.go`

- [ ] **Step 1: Implement main.go**

```go
// cmd/paisa-agent/main.go
package main

import (
    "flag"
    "fmt"
    "strings"

    "github.com/ananthakumaran/paisa/internal/agent/approval"
    "github.com/ananthakumaran/paisa/internal/agent/config"
    agentledger "github.com/ananthakumaran/paisa/internal/agent/ledger"
    "github.com/ananthakumaran/paisa/internal/agent/llm"
    "github.com/ananthakumaran/paisa/internal/agent/parser"
    "github.com/ananthakumaran/paisa/internal/agent/telegram"
    log "github.com/sirupsen/logrus"
)

func main() {
    cfgPath := flag.String("config", "paisa-agent.yaml", "path to paisa-agent.yaml")
    flag.Parse()

    cfg, err := config.Load(*cfgPath)
    if err != nil {
        log.Fatalf("load config: %v", err)
    }

    if err := agentledger.EnsureFile(cfg.Paisa.JournalDir); err != nil {
        log.Warnf("ensure auto-import.ledger: %v", err)
    }

    bot := telegram.NewBot(cfg.Telegram.BotToken, cfg.Telegram.ChatID)
    store := approval.NewStore()

    log.Infof("paisa-agent started — polling Telegram (chat_id=%d)", cfg.Telegram.ChatID)

    for {
        updates, err := bot.Poll()
        if err != nil {
            log.Errorf("poll error: %v", err)
            continue
        }
        for _, u := range updates {
            switch {
            case u.CallbackQuery != nil:
                handleCallback(u.CallbackQuery, bot, store, cfg)
            case u.Message != nil:
                handleMessage(u.Message, bot, store, cfg)
            }
        }
    }
}

func handleMessage(msg *telegram.Message, bot *telegram.Bot, store *approval.Store, cfg *config.Config) {
    // If this chat has an entry in editing state, route as an edit reply.
    if pending := store.GetEditingByChatID(msg.Chat.ID); pending != nil {
        updated := telegram.ParseEditReply(msg.Text, pending.Entry)
        store.Delete(pending.MessageID)
        sendDraft(updated, bot, store, cfg)
        return
    }

    // Otherwise treat the message text as a new bank SMS.
    entry, err := parseAndFill(msg.Text, cfg)
    if err != nil {
        log.Errorf("parse: %v", err)
        bot.SendText(fmt.Sprintf("❌ Could not parse: %v", err))
        return
    }
    sendDraft(*entry, bot, store, cfg)
}

func parseAndFill(sms string, cfg *config.Config) (*agentledger.Entry, error) {
    rule, err := parser.Classify(sms, cfg.ParserRules.Accounts)
    if err != nil {
        return nil, err
    }
    entry, err := parser.Parse(sms, rule, cfg.ParserRules.Merchants)
    if err != nil {
        return nil, err
    }
    if entry.Dest == "" || entry.Desc == "" {
        if llmErr := llm.FillMissing(sms, entry, cfg.Ollama); llmErr != nil {
            log.Warnf("llm fallback: %v", llmErr)
        }
    }
    return entry, nil
}

func sendDraft(entry agentledger.Entry, bot *telegram.Bot, store *approval.Store, cfg *config.Config) {
    draftText := telegram.FormatDraft(entry)

    dup, err := agentledger.IsDuplicate(cfg.Paisa.JournalDir, &entry)
    if err != nil {
        log.Warnf("duplicate check: %v", err)
    }

    var msgID int
    if dup {
        msgID, err = bot.SendDraftDuplicate(draftText)
    } else {
        msgID, err = bot.SendDraft(draftText)
    }
    if err != nil {
        log.Errorf("send draft: %v", err)
        return
    }

    store.Set(&approval.Pending{
        Entry:     entry,
        ChatID:    cfg.Telegram.ChatID,
        MessageID: msgID,
        Status:    approval.StatusPending,
    })
}

func handleCallback(cb *telegram.CallbackQuery, bot *telegram.Bot, store *approval.Store, cfg *config.Config) {
    bot.AnswerCallback(cb.ID)

    if cb.Message == nil {
        return
    }
    msgID := cb.Message.MessageID
    pending := store.Get(msgID)
    if pending == nil {
        log.Debugf("callback for unknown messageID %d (agent may have restarted)", msgID)
        return
    }

    switch strings.ToLower(cb.Data) {
    case "approve":
        if err := agentledger.Append(cfg.Paisa.JournalDir, &pending.Entry); err != nil {
            log.Errorf("append entry: %v", err)
            bot.EditMessage(msgID, "❌ Failed to post: "+err.Error())
            return
        }
        bot.EditMessage(msgID, "✅ Posted\n\n"+telegram.FormatDraft(pending.Entry))
        store.Delete(msgID)
        log.Infof("posted: %s %s %s", pending.Entry.Date, pending.Entry.Desc, pending.Entry.Amt)

    case "edit":
        store.SetEditing(msgID)
        bot.SendText(telegram.FormatEditTemplate(pending.Entry))

    case "skip":
        bot.EditMessage(msgID, "⏭ Skipped\n\n"+telegram.FormatDraft(pending.Entry))
        store.Delete(msgID)
    }
}
```

- [ ] **Step 2: Build the binary**

```bash
go build ./cmd/paisa-agent/
```
Expected: `paisa-agent` binary in project root, no errors

- [ ] **Step 3: Run all agent tests**

```bash
go test ./internal/agent/... -v
```
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/paisa-agent/main.go
git commit -m "feat(agent): add main loop — Telegram poll, parse, approve, append"
```

---

## Task 14: Update paisa-agent.yaml with per-bank interest rules

**Files:**
- Modify: `~/Documents/paisa/paisa-agent.yaml`

- [ ] **Step 1: Add per-bank interest and missing merchant rules to the `merchants` section**

In `~/Documents/paisa/paisa-agent.yaml`, update the `merchants` section. Add before the generic `interest` entry:

```yaml
merchants:
  # Food
  - keyword: "swiggy"
    account: "Expenses:Food:Hyd"
    description: "Food Swiggy"
  - keyword: "zomato"
    account: "Expenses:Food:Hyd"
    description: "Food Zomato"

  # Groceries
  - keyword: "blink commerce"
    account: "Expenses:Groceries:Hyd"
    description: "Groceries Blink"
  - keyword: "blinkit"
    account: "Expenses:Groceries:Hyd"
    description: "Groceries Blink"
  - keyword: "zepto"
    account: "Expenses:Groceries:Hyd"
    description: "Groceries ZEPTO"
  - keyword: "bigbasket"
    account: "Expenses:Groceries:Hyd"
    description: "Groceries BigBasket"
  - keyword: "dmart"
    account: "Expenses:Groceries:Hyd"
    description: "Groceries DMart"

  # Entertainment
  - keyword: "district"
    account: "Expenses:Entertainment:Hyd"
    description: "Entertainment: DISTRICT"
  - keyword: "netflix"
    account: "Expenses:Entertainment:Hyd"
    description: "Entertainment: Netflix"
  - keyword: "bookmyshow"
    account: "Expenses:Entertainment:Hyd"
    description: "Entertainment: BookMyShow"

  # Travel
  - keyword: "irctc"
    account: "Expenses:Travel:Hyd"
    description: "Travel"
  - keyword: "rail web"
    account: "Expenses:Travel:Hyd"
    description: "Travel"
  - keyword: "makemytrip"
    account: "Expenses:Travel:Hyd"
    description: "Travel: MakeMyTrip"

  # Commute
  - keyword: "uber"
    account: "Expenses:Commute:Hyd"
    description: "Commute: Uber"
  - keyword: "rapido"
    account: "Expenses:Commute:Hyd"
    description: "Commute: Rapido"
  - keyword: "ola"
    account: "Expenses:Commute:Hyd"
    description: "Commute: Ola"

  # Utils
  - keyword: "ing*flipkar"
    account: "Expenses:Utils:Hyd"
    description: "Utils: Flipkart"
  - keyword: "flipkart"
    account: "Expenses:Utils:Hyd"
    description: "Utils: Flipkart"
  - keyword: "amazon"
    account: "Expenses:Utils:Hyd"
    description: "Utils: Amazon Pay"
  - keyword: "airtel"
    account: "Expenses:Utils:Hyd"
    description: "Utils: Airtel"

  # Income — per-bank interest rules (specific before generic)
  - keyword: "monthly interest"
    account: "Income:Interest:IDFC6977"
    description: "Bank Interest"
  - keyword: "interest"
    account: "Income:Interest"
    description: "Bank Interest"

  # Fuel
  - keyword: "hpcl"
    account: "Expenses:Fuel:Hyd"
    description: "Fuel: HPCL"
  - keyword: "iocl"
    account: "Expenses:Fuel:Hyd"
    description: "Fuel: IOCL"

  # Health
  - keyword: "apollo"
    account: "Expenses:Health:Hyd"
    description: "Health: Apollo"
  - keyword: "medplus"
    account: "Expenses:Health:Hyd"
    description: "Health: MedPlus"
```

- [ ] **Step 2: Smoke-test the binary with a sample message**

Run the binary with the updated config and forward one of these test messages via Telegram to verify the pipeline end-to-end:

```
INR 1804.05 debited
A/c no. XX6386
03-06-26, 10:21:54
UPI/P2M/102154212206/IRCTC Rail Web
Not you? SMS BLOCKUPI Cust ID to 919951860002
Axis Bank
```

Expected Telegram draft:
```
📨 New Transaction

desc: Travel
date: 2026/06/03
src:  Assets:Checking:AXIS6386
amt:  -1804.05 INR
dest: Expenses:Travel:Hyd
```

- [ ] **Step 3: Commit plan completion**

```bash
git add docs/superpowers/plans/2026-06-07-auto-ingest-transactions.md
git commit -m "docs: add auto-ingest transactions implementation plan"
```

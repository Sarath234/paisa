# Statement Reconciliation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When an Axis Bank statement email arrives in Gmail, parse the PDF, compare transactions against the ledger, and report missing/extra entries via Telegram + the paisa doctor page.

**Architecture:** Gmail is polled every 5 minutes; a matching statement PDF is extracted to text using `ledongthuc/pdf`, parsed by an Axis-specific parser, reconciled against ledger postings fetched from the paisa API, and results persisted to `reconciliation.json` in the journal directory — which the paisa server's `/api/diagnosis` endpoint reads to surface issues on the doctor page.

**Tech Stack:** Go 1.24, `github.com/ledongthuc/pdf` (PDF text extraction), `golang.org/x/oauth2` + `google.golang.org/api/gmail/v1` (Gmail API), stdlib `encoding/json`, `net/http`.

**Prerequisites (manual setup before running):**
1. Go to https://console.cloud.google.com — create a project, enable Gmail API.
2. Create OAuth 2.0 credentials → Desktop app type. Download `client_id` and `client_secret`.
3. Add to `paisa-agent.yaml`:
   ```yaml
   gmail:
     client_id: "YOUR_CLIENT_ID"
     client_secret: "YOUR_CLIENT_SECRET"
     token_file: "/Users/YOUR_USER/.paisa-agent/gmail-token.json"
     statement_accounts:
       - subject_match: "6386"
         ledger_account: "Assets:Checking:AXIS6386"
   ```

---

## File Map

| File | Status | Responsibility |
|------|--------|---------------|
| `internal/agent/config/config.go` | Modify | Add `GmailConfig`, `StatementAccount` struct fields |
| `internal/agent/paisaclient/client.go` | Modify | Add `Postings()` method |
| `internal/agent/statement/parser.go` | Create | `Transaction`, `ParseResult`, `Parser` interface |
| `internal/agent/statement/axis.go` | Create | Axis savings account PDF parser |
| `internal/agent/statement/axis_test.go` | Create | Golden text fixture test |
| `internal/agent/reconcile/reconcile.go` | Create | `LedgerEntry`, `Diff`, `Compare` |
| `internal/agent/reconcile/reconcile_test.go` | Create | Table-driven Compare tests |
| `internal/agent/reconcile/store.go` | Create | JSON persistence to `reconciliation.json` |
| `internal/agent/reconcile/store_test.go` | Create | Write/read round-trip test |
| `internal/agent/gmail/client.go` | Create | OAuth2 client, Search, Download, MarkRead |
| `internal/agent/gmail/poller.go` | Create | 5-min polling loop |
| `internal/server/doctor.go` | Modify | New rule: read `reconciliation.json`, emit issues |
| `cmd/paisa-agent/main.go` | Modify | Wire Gmail poller, handle StatementEmail events, OAuth flow |

---

### Task 1: Add Gmail config types

**Files:**
- Modify: `internal/agent/config/config.go`

- [ ] **Step 1: Add `GmailConfig` and `StatementAccount` to `config.go`**

Open `internal/agent/config/config.go`. Add the new types and update `Config`:

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
	Gmail       *GmailConfig   `yaml:"gmail,omitempty"`
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

// AccountRule, MerchantRule, Load remain unchanged — do not remove them.
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

- [ ] **Step 2: Confirm it compiles**

```bash
go build ./internal/agent/config/
```
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/agent/config/config.go
git commit -m "feat(config): add GmailConfig and StatementAccount types"
```

---

### Task 2: Add `Postings()` to paisaclient

**Files:**
- Modify: `internal/agent/paisaclient/client.go`

The existing `Posting` struct already has `Date`, `Account`, `Amount` (float64), and `Payee`. We just need to add the API call.

- [ ] **Step 1: Add the `Postings` method to `internal/agent/paisaclient/client.go`**

Append after the existing `BudgetForMonth` function:

```go
// Postings returns all postings from the ledger (GET /api/ledger).
func (c *Client) Postings() ([]Posting, error) {
	var r struct {
		Postings []Posting `json:"postings"`
	}
	if err := c.get("/api/ledger", &r); err != nil {
		return nil, err
	}
	return r.Postings, nil
}
```

- [ ] **Step 2: Build to confirm**

```bash
go build ./internal/agent/paisaclient/
```
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add internal/agent/paisaclient/client.go
git commit -m "feat(paisaclient): add Postings() method for /api/ledger"
```

---

### Task 3: Statement types + Parser interface

**Files:**
- Create: `internal/agent/statement/parser.go`

- [ ] **Step 1: Create the file**

```go
// internal/agent/statement/parser.go
package statement

import "time"

// Transaction is one row from a bank statement.
type Transaction struct {
	Date        time.Time
	Description string
	Debit       float64
	Credit      float64
}

// ParseResult is the output of parsing a statement PDF.
type ParseResult struct {
	Transactions   []Transaction
	ClosingBalance float64
	Account        string     // detected from statement (e.g. "AXIS6386")
	Month          time.Month // statement period month
	Year           int        // statement period year
}

// Parser extracts transactions from a bank statement PDF.
type Parser interface {
	Name() string
	// Detect returns true when emailSubject belongs to this bank/account.
	Detect(emailSubject string) bool
	// Parse extracts transactions from raw PDF bytes.
	Parse(pdfBytes []byte) (ParseResult, error)
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/agent/statement/
```
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add internal/agent/statement/parser.go
git commit -m "feat(statement): add Transaction, ParseResult, Parser types"
```

---

### Task 4: Reconcile Compare algorithm

**Files:**
- Create: `internal/agent/reconcile/reconcile.go`
- Create: `internal/agent/reconcile/reconcile_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/agent/reconcile/reconcile_test.go`:

```go
// internal/agent/reconcile/reconcile_test.go
package reconcile

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/statement"
)

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func TestCompare_allMatch(t *testing.T) {
	result := statement.ParseResult{
		Transactions: []statement.Transaction{
			{Date: date(2026, 5, 1), Description: "SWIGGY", Debit: 500, Credit: 0},
			{Date: date(2026, 5, 2), Description: "UPI CREDIT", Debit: 0, Credit: 1000},
		},
		ClosingBalance: 9000,
		Month:          time.May,
		Year:           2026,
	}
	ledger := []LedgerEntry{
		{Date: date(2026, 5, 1), Description: "Food Swiggy", Amount: -500},
		{Date: date(2026, 5, 2), Description: "UPI credit", Amount: 1000},
	}
	diff := Compare(result, ledger)
	if len(diff.Missing) != 0 {
		t.Errorf("Missing=%d want 0", len(diff.Missing))
	}
	if len(diff.Extra) != 0 {
		t.Errorf("Extra=%d want 0", len(diff.Extra))
	}
}

func TestCompare_missingFromLedger(t *testing.T) {
	result := statement.ParseResult{
		Transactions: []statement.Transaction{
			{Date: date(2026, 5, 1), Description: "SWIGGY", Debit: 500},
			{Date: date(2026, 5, 3), Description: "AMAZON", Debit: 1200},
		},
		ClosingBalance: 8000,
		Month:          time.May,
		Year:           2026,
	}
	ledger := []LedgerEntry{
		{Date: date(2026, 5, 1), Description: "Food Swiggy", Amount: -500},
	}
	diff := Compare(result, ledger)
	if len(diff.Missing) != 1 {
		t.Fatalf("Missing=%d want 1", len(diff.Missing))
	}
	if diff.Missing[0].Description != "AMAZON" {
		t.Errorf("Missing[0].Description=%q want AMAZON", diff.Missing[0].Description)
	}
}

func TestCompare_extraInLedger(t *testing.T) {
	result := statement.ParseResult{
		Transactions: []statement.Transaction{
			{Date: date(2026, 5, 1), Description: "SWIGGY", Debit: 500},
		},
		ClosingBalance: 9500,
		Month:          time.May,
		Year:           2026,
	}
	ledger := []LedgerEntry{
		{Date: date(2026, 5, 1), Description: "Food Swiggy", Amount: -500},
		{Date: date(2026, 5, 5), Description: "Extra phantom", Amount: -200},
	}
	diff := Compare(result, ledger)
	if len(diff.Extra) != 1 {
		t.Fatalf("Extra=%d want 1", len(diff.Extra))
	}
	if diff.Extra[0].Description != "Extra phantom" {
		t.Errorf("Extra[0].Description=%q want 'Extra phantom'", diff.Extra[0].Description)
	}
}

func TestCompare_closingBalance(t *testing.T) {
	result := statement.ParseResult{
		ClosingBalance: 12345.67,
		Month:          time.May,
		Year:           2026,
	}
	diff := Compare(result, nil)
	if diff.StatementClose != 12345.67 {
		t.Errorf("StatementClose=%.2f want 12345.67", diff.StatementClose)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/agent/reconcile/ -run TestCompare -v 2>&1 | head -8
```
Expected: `undefined: Compare` or `cannot find package`.

- [ ] **Step 3: Create `reconcile.go`**

```go
// internal/agent/reconcile/reconcile.go
package reconcile

import (
	"math"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/statement"
)

// LedgerEntry is a posting from the paisa ledger relevant to reconciliation.
type LedgerEntry struct {
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	Amount      float64   `json:"amount"` // negative = debit, positive = credit
}

// Diff is the result of comparing a statement against the ledger.
type Diff struct {
	Account        string                `json:"account"`
	Month          int                   `json:"month"` // 1-12
	Year           int                   `json:"year"`
	StatementClose float64               `json:"statement_close"`
	Missing        []statement.Transaction `json:"missing"` // in statement, not in ledger
	Extra          []LedgerEntry         `json:"extra"`   // in ledger, not in statement
}

const amountEpsilon = 0.01

// Compare matches statement transactions against ledger entries by date + |amount|.
// A statement debit of 500 matches a ledger entry with Amount -500 on the same date.
func Compare(result statement.ParseResult, ledger []LedgerEntry) Diff {
	diff := Diff{
		Month:          int(result.Month),
		Year:           result.Year,
		StatementClose: result.ClosingBalance,
	}

	// Build a consumed-flag map for ledger entries to support multi-match avoidance.
	ledgerUsed := make([]bool, len(ledger))

	for _, tx := range result.Transactions {
		txAmt := tx.Credit - tx.Debit // net: credit positive, debit negative
		matched := false
		for i, le := range ledger {
			if ledgerUsed[i] {
				continue
			}
			if !sameDay(tx.Date, le.Date) {
				continue
			}
			if math.Abs(le.Amount-txAmt) <= amountEpsilon {
				ledgerUsed[i] = true
				matched = true
				break
			}
		}
		if !matched {
			diff.Missing = append(diff.Missing, tx)
		}
	}

	for i, le := range ledger {
		if !ledgerUsed[i] {
			diff.Extra = append(diff.Extra, le)
		}
	}

	return diff
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/agent/reconcile/ -run TestCompare -v
```
Expected: all 4 `TestCompare_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/reconcile/reconcile.go internal/agent/reconcile/reconcile_test.go
git commit -m "feat(reconcile): add Compare algorithm with LedgerEntry and Diff types"
```

---

### Task 5: Reconcile JSON store

**Files:**
- Create: `internal/agent/reconcile/store.go`
- Create: `internal/agent/reconcile/store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/agent/reconcile/store_test.go`:

```go
// internal/agent/reconcile/store_test.go
package reconcile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/statement"
)

func TestStore_WriteRead(t *testing.T) {
	dir := t.TempDir()

	rec := Record{
		Period:      "2026-05",
		GeneratedAt: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
		Diff: Diff{
			Account:        "Assets:Checking:AXIS6386",
			Month:          5,
			Year:           2026,
			StatementClose: 9000.00,
			Missing: []statement.Transaction{
				{Date: date(2026, 5, 3), Description: "AMAZON", Debit: 1200},
			},
			Extra: nil,
		},
	}

	if err := Write(dir, rec); err != nil {
		t.Fatalf("Write: %v", err)
	}

	records, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len=%d want 1", len(records))
	}
	if records[0].Period != "2026-05" {
		t.Errorf("Period=%q want 2026-05", records[0].Period)
	}
	if records[0].Diff.StatementClose != 9000.00 {
		t.Errorf("StatementClose=%.2f want 9000.00", records[0].Diff.StatementClose)
	}
	if len(records[0].Diff.Missing) != 1 {
		t.Errorf("Missing=%d want 1", len(records[0].Diff.Missing))
	}
}

func TestStore_UpsertReplacesExistingPeriod(t *testing.T) {
	dir := t.TempDir()

	r1 := Record{Period: "2026-05", Diff: Diff{StatementClose: 1000}}
	r2 := Record{Period: "2026-05", Diff: Diff{StatementClose: 2000}}
	r3 := Record{Period: "2026-04", Diff: Diff{StatementClose: 500}}

	_ = Write(dir, r1)
	_ = Write(dir, r3)
	_ = Write(dir, r2) // overwrites r1

	records, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len=%d want 2", len(records))
	}
	var found bool
	for _, r := range records {
		if r.Period == "2026-05" {
			if r.Diff.StatementClose != 2000 {
				t.Errorf("upsert: StatementClose=%.0f want 2000", r.Diff.StatementClose)
			}
			found = true
		}
	}
	if !found {
		t.Error("2026-05 record not found after upsert")
	}
}

func TestStore_ReadAll_missingFile(t *testing.T) {
	dir := t.TempDir()
	// file doesn't exist — should return empty slice, not error
	records, err := ReadAll(dir)
	if err != nil {
		t.Errorf("ReadAll on missing file: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("len=%d want 0", len(records))
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/agent/reconcile/ -run TestStore -v 2>&1 | head -8
```
Expected: `undefined: Record` or similar.

- [ ] **Step 3: Create `store.go`**

```go
// internal/agent/reconcile/store.go
package reconcile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const storeFile = "reconciliation.json"

// Record is one reconciliation result, keyed by account+period.
type Record struct {
	Period      string    `json:"period"`       // "YYYY-MM"
	GeneratedAt time.Time `json:"generated_at"`
	Diff        Diff      `json:"diff"`
}

// Write upserts rec into <journalDir>/reconciliation.json keyed by Period.
func Write(journalDir string, rec Record) error {
	path := filepath.Join(journalDir, storeFile)
	records, err := ReadAll(journalDir)
	if err != nil {
		return err
	}

	replaced := false
	for i, r := range records {
		if r.Period == rec.Period {
			records[i] = rec
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, rec)
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// ReadAll reads all reconciliation records from <journalDir>/reconciliation.json.
// Returns an empty slice (no error) if the file does not exist.
func ReadAll(journalDir string) ([]Record, error) {
	path := filepath.Join(journalDir, storeFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}
```

- [ ] **Step 4: Run all reconcile tests**

```bash
go test ./internal/agent/reconcile/ -v
```
Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/reconcile/store.go internal/agent/reconcile/store_test.go
git commit -m "feat(reconcile): add JSON store with upsert by period"
```

---

### Task 6: Add external Go dependencies

**Files:**
- `go.mod`, `go.sum` (auto-updated)

- [ ] **Step 1: Add the three new libraries**

```bash
cd /Users/sarath.m/workspace/work/paisa
go get github.com/ledongthuc/pdf@latest
go get golang.org/x/oauth2@latest
go get google.golang.org/api/gmail/v1@latest
```

Each command prints `go: added ...`. All three must succeed.

- [ ] **Step 2: Tidy**

```bash
go mod tidy
```

- [ ] **Step 3: Verify build still passes**

```bash
go build ./internal/agent/... ./cmd/paisa-agent/
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): add ledongthuc/pdf, oauth2, gmail/v1"
```

---

### Task 7: Axis Bank PDF parser

**Files:**
- Create: `internal/agent/statement/axis.go`
- Create: `internal/agent/statement/axis_test.go`

The Axis savings account statement has this column layout:
`Date | Tran Date | Transaction Particulars | Chq no. | Withdrawal(Dr) | Deposit(Cr) | Balance`

When `ledongthuc/pdf` extracts text, each row appears on one or two lines anchored by the `DD-MM-YYYY` pattern. Amounts use Indian number format (commas as thousands separators).

- [ ] **Step 1: Write the failing test**

Create `internal/agent/statement/axis_test.go`:

```go
// internal/agent/statement/axis_test.go
package statement

import (
	"strings"
	"testing"
	"time"
)

// sampleAxisText is representative extracted-PDF text for an Axis savings statement.
// Real statements will be processed the same way; tune regexes if columns shift.
const sampleAxisText = `
Statement for the period from 01-05-2026 to 31-05-2026

Savings account(s)
Account number: XXXXXXXXXX6386

Date         Tran Date   Transaction Particulars                       Chq no.  Withdrawal(Dr)  Deposit(Cr)  Balance

01-05-2026  01-05-2026  UPI/UPICQR/SWIGGY FOOD                                 500.00           0.00    9,24,764.20
05-05-2026  05-05-2026  UPI/P2P/SALARY CREDIT                                  0.00          50,000.00    9,74,764.20
12-05-2026  12-05-2026  IRCTC RAIL BOOKING                                    1,200.00          0.00    9,73,564.20
31-05-2026  31-05-2026  INTEREST CREDITED                                       0.00            120.50    9,73,684.70

Credit Cards
`

func TestAxisParser_Parse(t *testing.T) {
	p := &AxisParser{}
	result, err := p.parseText(sampleAxisText)
	if err != nil {
		t.Fatalf("parseText: %v", err)
	}

	if len(result.Transactions) != 4 {
		t.Fatalf("Transactions=%d want 4", len(result.Transactions))
	}

	tx0 := result.Transactions[0]
	if tx0.Debit != 500.00 {
		t.Errorf("tx0.Debit=%.2f want 500.00", tx0.Debit)
	}
	if tx0.Credit != 0 {
		t.Errorf("tx0.Credit=%.2f want 0.00", tx0.Credit)
	}
	wantDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if !tx0.Date.Equal(wantDate) {
		t.Errorf("tx0.Date=%v want %v", tx0.Date, wantDate)
	}

	tx1 := result.Transactions[1]
	if tx1.Credit != 50000.00 {
		t.Errorf("tx1.Credit=%.2f want 50000.00", tx1.Credit)
	}

	// Closing balance = last row's balance
	if result.ClosingBalance != 9_73_684.70 {
		t.Errorf("ClosingBalance=%.2f want 973684.70", result.ClosingBalance)
	}

	if result.Month != time.May {
		t.Errorf("Month=%v want May", result.Month)
	}
	if result.Year != 2026 {
		t.Errorf("Year=%d want 2026", result.Year)
	}
}

func TestAxisParser_Detect(t *testing.T) {
	p := &AxisParser{}
	cases := []struct {
		subject string
		want    bool
	}{
		{"Account Statement for XXXXXXXXXX6386", true},
		{"Axis Bank statement 6386", true},
		{"HDFC statement", false},
		{"", false},
	}
	for _, c := range cases {
		if got := p.Detect(c.subject); got != c.want {
			t.Errorf("Detect(%q)=%v want %v", c.subject, got, c.want)
		}
	}
}

// Ensure AxisParser implements Parser.
var _ Parser = &AxisParser{}

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/agent/statement/ -run TestAxis -v 2>&1 | head -8
```
Expected: `undefined: AxisParser`.

- [ ] **Step 3: Create `axis.go`**

```go
// internal/agent/statement/axis.go
package statement

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// AxisParser parses Axis Bank savings account statement PDFs.
type AxisParser struct{}

func (p *AxisParser) Name() string { return "axis_savings" }

func (p *AxisParser) Detect(subject string) bool {
	s := strings.ToLower(subject)
	return strings.Contains(s, "axis") || strings.Contains(s, "6386")
}

func (p *AxisParser) Parse(pdfBytes []byte) (ParseResult, error) {
	text, err := extractPDFText(pdfBytes)
	if err != nil {
		return ParseResult{}, fmt.Errorf("axis: extract text: %w", err)
	}
	return p.parseText(text)
}

// extractPDFText extracts all text from PDF bytes using ledongthuc/pdf.
func extractPDFText(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		pg := r.Page(i)
		if pg.V.IsNull() {
			continue
		}
		text, err := pg.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

var (
	// Matches the statement period line: "from DD-MM-YYYY to DD-MM-YYYY"
	periodRe = regexp.MustCompile(`from\s+(\d{2}-\d{2}-\d{4})\s+to\s+(\d{2}-\d{2}-\d{4})`)

	// Matches a transaction row starting with a date.
	// Groups: 1=date, 2=tran_date, 3=rest_of_line
	txnRe = regexp.MustCompile(`(\d{2}-\d{2}-\d{4})\s+\d{2}-\d{2}-\d{4}\s+(.+)`)

	// Extracts all decimal numbers (Indian format with commas).
	amountRe = regexp.MustCompile(`[\d,]+\.\d{2}`)

	// Section terminators: stop parsing transactions when these headers appear.
	sectionEndRe = regexp.MustCompile(`(?i)Credit Cards?|Deposits?\s+\(`)
)

const axisDateLayout = "02-01-2006"

// parseText is exported for unit testing without a real PDF.
func (p *AxisParser) parseText(text string) (ParseResult, error) {
	var result ParseResult

	// Extract period.
	if m := periodRe.FindStringSubmatch(text); len(m) == 3 {
		end, err := time.Parse(axisDateLayout, m[2])
		if err == nil {
			result.Month = end.Month()
			result.Year = end.Year()
		}
	}

	// Find the savings account section and stop at next major section.
	savingsIdx := strings.Index(strings.ToLower(text), "savings account")
	if savingsIdx < 0 {
		savingsIdx = 0 // fall back to whole text
	}
	section := text[savingsIdx:]
	if loc := sectionEndRe.FindStringIndex(section); loc != nil {
		section = section[:loc[0]]
	}

	var closingBalance float64
	lines := strings.Split(section, "\n")
	for _, line := range lines {
		m := txnRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		dateStr := m[1]
		rest := m[2]

		txDate, err := time.Parse(axisDateLayout, dateStr)
		if err != nil {
			continue
		}

		amounts := amountRe.FindAllString(rest, -1)
		if len(amounts) < 3 {
			continue // need at least withdrawal, credit, balance
		}

		// Last 3 numbers: withdrawal, credit, balance.
		n := len(amounts)
		withdrawal := parseIndianFloat(amounts[n-3])
		credit := parseIndianFloat(amounts[n-2])
		balance := parseIndianFloat(amounts[n-1])
		closingBalance = balance

		// Description is everything before the first numeric in rest.
		firstNum := amountRe.FindStringIndex(rest)
		desc := strings.TrimSpace(rest)
		if firstNum != nil {
			desc = strings.TrimSpace(rest[:firstNum[0]])
		}

		result.Transactions = append(result.Transactions, Transaction{
			Date:        txDate,
			Description: desc,
			Debit:       withdrawal,
			Credit:      credit,
		})
	}

	result.ClosingBalance = closingBalance
	return result, nil
}

// parseIndianFloat converts "9,24,764.20" → 924764.20.
func parseIndianFloat(s string) float64 {
	s = strings.ReplaceAll(s, ",", "")
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/agent/statement/ -v
```
Expected: `TestAxisParser_Parse` and `TestAxisParser_Detect` PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/statement/axis.go internal/agent/statement/axis_test.go
git commit -m "feat(statement): add AxisParser for Axis savings account PDFs"
```

---

### Task 8: Gmail OAuth client

**Files:**
- Create: `internal/agent/gmail/client.go`

This is the Gmail API + OAuth2 client. Testing the real API requires live credentials; instead we test the HTTP helpers with `httptest`.

- [ ] **Step 1: Create `internal/agent/gmail/client.go`**

```go
// internal/agent/gmail/client.go
package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googleGmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Message is a Gmail message with its attachment.
type Message struct {
	ID      string
	Subject string
}

// Client wraps the Gmail API service.
type Client struct {
	svc       *googleGmail.Service
	oauthConf *oauth2.Config
	tokenFile string
}

// New creates a Client using credentials from cfg.
// Returns (nil, nil) if clientID is empty (Gmail not configured).
func New(clientID, clientSecret, tokenFile string) (*Client, error) {
	if clientID == "" {
		return nil, nil
	}
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes: []string{
			googleGmail.GmailReadonlyScope,
			"https://www.googleapis.com/auth/gmail.modify", // for MarkRead
		},
		Endpoint:    google.Endpoint,
		RedirectURL: "http://localhost:8787/oauth/callback",
	}
	tok, err := loadToken(tokenFile)
	if err != nil {
		return &Client{oauthConf: conf, tokenFile: tokenFile}, nil // needs auth
	}
	svc, err := newService(conf, tok)
	if err != nil {
		return nil, fmt.Errorf("gmail: new service: %w", err)
	}
	return &Client{svc: svc, oauthConf: conf, tokenFile: tokenFile}, nil
}

// IsAuthorized returns true when a valid token is loaded.
func (c *Client) IsAuthorized() bool { return c.svc != nil }

// AuthURL returns the URL the user must open to authorise Gmail access.
func (c *Client) AuthURL() string {
	return c.oauthConf.AuthCodeURL("state", oauth2.AccessTypeOffline)
}

// ExchangeCode exchanges an OAuth2 auth code for a token and saves it.
// Call this after the user completes the OAuth flow and the local callback server captures the code.
func (c *Client) ExchangeCode(code string) error {
	tok, err := c.oauthConf.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("gmail: exchange code: %w", err)
	}
	if err := saveToken(c.tokenFile, tok); err != nil {
		return fmt.Errorf("gmail: save token: %w", err)
	}
	svc, err := newService(c.oauthConf, tok)
	if err != nil {
		return fmt.Errorf("gmail: new service after exchange: %w", err)
	}
	c.svc = svc
	return nil
}

// Search returns unread messages whose subject contains subjectMatch.
func (c *Client) Search(subjectMatch string) ([]Message, error) {
	q := fmt.Sprintf("is:unread subject:%s has:attachment", subjectMatch)
	r, err := c.svc.Users.Messages.List("me").Q(q).MaxResults(10).Do()
	if err != nil {
		return nil, fmt.Errorf("gmail: list: %w", err)
	}
	var msgs []Message
	for _, m := range r.Messages {
		full, err := c.svc.Users.Messages.Get("me", m.Id).Format("metadata").
			MetadataHeaders("Subject").Do()
		if err != nil {
			continue
		}
		subject := ""
		for _, h := range full.Payload.Headers {
			if h.Name == "Subject" {
				subject = h.Value
				break
			}
		}
		msgs = append(msgs, Message{ID: m.Id, Subject: subject})
	}
	return msgs, nil
}

// DownloadPDF downloads the first PDF attachment from the message and returns its bytes.
func (c *Client) DownloadPDF(msgID string) ([]byte, error) {
	msg, err := c.svc.Users.Messages.Get("me", msgID).Format("full").Do()
	if err != nil {
		return nil, fmt.Errorf("gmail: get message: %w", err)
	}
	for _, part := range msg.Payload.Parts {
		if part.MimeType != "application/pdf" {
			continue
		}
		att, err := c.svc.Users.Messages.Attachments.Get("me", msgID, part.Body.AttachmentId).Do()
		if err != nil {
			return nil, fmt.Errorf("gmail: get attachment: %w", err)
		}
		// Gmail uses URL-safe base64; the Go library decodes automatically.
		return att.DataBytes, nil
	}
	return nil, fmt.Errorf("gmail: no PDF attachment in message %s", msgID)
}

// MarkRead removes the UNREAD label from a message.
func (c *Client) MarkRead(msgID string) error {
	req := &googleGmail.ModifyMessageRequest{RemoveLabelIds: []string{"UNREAD"}}
	_, err := c.svc.Users.Messages.Modify("me", msgID, req).Do()
	return err
}

// OAuthCallbackServer starts a local HTTP server on port 8787, waits for the
// OAuth callback, and returns the auth code. It times out after 5 minutes.
func OAuthCallbackServer() (string, error) {
	codeCh := make(chan string, 1)
	srv := &http.Server{Addr: ":8787"}
	http.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		fmt.Fprintln(w, "Gmail authorised — you can close this tab.")
		codeCh <- code
	})
	go srv.ListenAndServe() //nolint:errcheck
	defer srv.Shutdown(context.Background()) //nolint:errcheck

	select {
	case code := <-codeCh:
		return code, nil
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("oauth: timed out waiting for callback")
	}
}

func newService(conf *oauth2.Config, tok *oauth2.Token) (*googleGmail.Service, error) {
	ctx := context.Background()
	ts := conf.TokenSource(ctx, tok)
	return googleGmail.NewService(ctx, option.WithTokenSource(ts))
}

func loadToken(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func saveToken(path string, tok *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
```

- [ ] **Step 2: Build to confirm compilation**

```bash
go build ./internal/agent/gmail/
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/agent/gmail/client.go
git commit -m "feat(gmail): add OAuth2 client with Search, DownloadPDF, MarkRead"
```

---

### Task 9: Gmail statement poller

**Files:**
- Create: `internal/agent/gmail/poller.go`

The poller runs on a ticker, searches for matching statement emails, and calls a handler function for each new statement found.

- [ ] **Step 1: Create `internal/agent/gmail/poller.go`**

```go
// internal/agent/gmail/poller.go
package gmail

import (
	"os"
	"encoding/json"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
)

const pollInterval = 5 * time.Minute
const processedFile = "processed-emails.json"

// StatementEmail is emitted when a new unread statement email is found.
type StatementEmail struct {
	MessageID     string
	Subject       string
	PDFBytes      []byte
	LedgerAccount string // from config statement_accounts match
}

// Poller periodically checks Gmail for new statement emails.
type Poller struct {
	client         *Client
	subjectMatches []SubjectMatch
	stateDir       string // where to store processed-emails.json
	handler        func(StatementEmail)
}

// SubjectMatch pairs an email subject pattern with a ledger account.
type SubjectMatch struct {
	Pattern       string
	LedgerAccount string
}

// NewPoller creates a Poller. stateDir is where processed-emails.json is written.
func NewPoller(client *Client, matches []SubjectMatch, stateDir string, handler func(StatementEmail)) *Poller {
	return &Poller{
		client:         client,
		subjectMatches: matches,
		stateDir:       stateDir,
		handler:        handler,
	}
}

// Start runs the poll loop in the current goroutine (call via go p.Start()).
func (p *Poller) Start() {
	p.poll() // poll once immediately on startup
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for range ticker.C {
		p.poll()
	}
}

func (p *Poller) poll() {
	if p.client == nil || !p.client.IsAuthorized() {
		return
	}
	processed := p.loadProcessed()
	for _, sm := range p.subjectMatches {
		msgs, err := p.client.Search(sm.Pattern)
		if err != nil {
			log.Warnf("gmail: search %q: %v", sm.Pattern, err)
			continue
		}
		for _, msg := range msgs {
			if processed[msg.ID] {
				continue
			}
			pdfBytes, err := p.client.DownloadPDF(msg.ID)
			if err != nil {
				log.Warnf("gmail: download PDF %s: %v", msg.ID, err)
				continue
			}
			p.handler(StatementEmail{
				MessageID:     msg.ID,
				Subject:       msg.Subject,
				PDFBytes:      pdfBytes,
				LedgerAccount: sm.LedgerAccount,
			})
			if err := p.client.MarkRead(msg.ID); err != nil {
				log.Warnf("gmail: mark read %s: %v", msg.ID, err)
			}
			processed[msg.ID] = true
			p.saveProcessed(processed)
		}
	}
}

func (p *Poller) processedPath() string {
	return filepath.Join(p.stateDir, processedFile)
}

func (p *Poller) loadProcessed() map[string]bool {
	data, err := os.ReadFile(p.processedPath())
	if err != nil {
		return make(map[string]bool)
	}
	var m map[string]bool
	if err := json.Unmarshal(data, &m); err != nil {
		return make(map[string]bool)
	}
	return m
}

func (p *Poller) saveProcessed(m map[string]bool) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(p.stateDir, 0755)
	_ = os.WriteFile(p.processedPath(), data, 0644)
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/agent/gmail/
```
Expected: no errors.

- [ ] **Step 3: Run all agent tests to confirm nothing broken**

```bash
go test ./internal/agent/... 2>&1 | grep -E "FAIL|ok"
```
Expected: all lines show `ok`.

- [ ] **Step 4: Commit**

```bash
git add internal/agent/gmail/poller.go
git commit -m "feat(gmail): add statement poller with processed-emails deduplication"
```

---

### Task 10: Doctor page reconciliation rule

**Files:**
- Modify: `internal/server/doctor.go`

The new rule reads `reconciliation.json` from the journal directory and emits WARN issues for balance mismatches or missing transactions. It uses anonymous structs for JSON decoding — no import of agent packages.

- [ ] **Step 1: Add the reconciliation rule to `internal/server/doctor.go`**

Find the `init()` function in `internal/server/doctor.go` and add one new `Rule` to the `rules` slice:

```go
// Add this entry to the rules slice in init():
{
    Issue: Issue{
        Level:       WARN,
        Summary:     "Statement reconciliation issue",
        Description: "One or more bank statement reconciliation checks failed."},
    Predicate: ruleStatementReconciliation},
```

Then add the `ruleStatementReconciliation` function and its imports at the bottom of `doctor.go`.

First, add `"encoding/json"`, `"errors"`, `"os"`, `"path/filepath"`, `"fmt"` to the import block (some may already be present — add only the missing ones).

Then add the function:

```go
func ruleStatementReconciliation(db *gorm.DB) []error {
	errs := make([]error, 0)

	journalDir := config.GetConfig().JournalDir
	path := filepath.Join(journalDir, "reconciliation.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errs // no reconciliation run yet — not an error
		}
		return errs // silently ignore read errors
	}

	var records []struct {
		Period string `json:"period"`
		Diff   struct {
			Account        string  `json:"account"`
			Month          int     `json:"month"`
			Year           int     `json:"year"`
			StatementClose float64 `json:"statement_close"`
			Missing        []struct {
				Date        string  `json:"date"`
				Description string  `json:"description"`
				Debit       float64 `json:"debit"`
				Credit      float64 `json:"credit"`
			} `json:"missing"`
			Extra []struct {
				Date        string  `json:"date"`
				Description string  `json:"description"`
				Amount      float64 `json:"amount"`
			} `json:"extra"`
		} `json:"diff"`
	}

	if err := json.Unmarshal(data, &records); err != nil {
		return errs
	}

	for _, r := range records {
		d := r.Diff
		label := fmt.Sprintf("%s — %04d-%02d", d.Account, d.Year, d.Month)

		if len(d.Missing) > 0 {
			var details strings.Builder
			for _, m := range d.Missing {
				if m.Debit > 0 {
					fmt.Fprintf(&details, "• %s %s  -₹%.2f\n", m.Date, m.Description, m.Debit)
				} else {
					fmt.Fprintf(&details, "• %s %s  +₹%.2f\n", m.Date, m.Description, m.Credit)
				}
			}
			errs = append(errs, errors.New(fmt.Sprintf(
				"<b>%d transaction(s) missing from ledger</b> (%s)\n%s",
				len(d.Missing), label, details.String())))
		}

		if len(d.Extra) > 0 {
			var details strings.Builder
			for _, e := range d.Extra {
				fmt.Fprintf(&details, "• %s %s  %.2f\n", e.Date, e.Description, e.Amount)
			}
			errs = append(errs, errors.New(fmt.Sprintf(
				"<b>%d extra ledger entr(ies) not in statement</b> (%s)\n%s",
				len(d.Extra), label, details.String())))
		}
	}

	return errs
}
```

- [ ] **Step 2: Build the paisa server**

```bash
go build ./internal/server/
```
Expected: no errors. (Fix any missing imports flagged by the compiler.)

- [ ] **Step 3: Run tests**

```bash
go test ./internal/server/ -run TestDiagnosis -v 2>&1 | head -20
```
(There may be no existing TestDiagnosis — that's fine, just verify no compilation errors.)

- [ ] **Step 4: Commit**

```bash
git add internal/server/doctor.go
git commit -m "feat(doctor): add reconciliation rule to surface missing/extra transactions"
```

---

### Task 11: Wire Gmail + reconciliation into `main.go`

**Files:**
- Modify: `cmd/paisa-agent/main.go`

This task adds Gmail polling, the statement → reconcile pipeline, OAuth flow handling, and Telegram report sending.

- [ ] **Step 1: Read current `main.go`** to understand the imports and `main()` function before editing.

- [ ] **Step 2: Add the necessary imports**

Add to the import block (alongside existing imports):
```go
"fmt"
"path/filepath"

"github.com/ananthakumaran/paisa/internal/agent/gmail"
"github.com/ananthakumaran/paisa/internal/agent/reconcile"
"github.com/ananthakumaran/paisa/internal/agent/statement"
```

- [ ] **Step 3: Add `handleStatementEmail` helper function**

Add this function at the bottom of `cmd/paisa-agent/main.go`:

```go
func handleStatementEmail(
	ev gmail.StatementEmail,
	parsers []statement.Parser,
	pc *paisaclient.Client,
	journalDir string,
	bot *telegram.Bot,
) {
	var result statement.ParseResult
	var parsed bool
	for _, p := range parsers {
		if !p.Detect(ev.Subject) {
			continue
		}
		r, err := p.Parse(ev.PDFBytes)
		if err != nil {
			log.Errorf("statement: parse %s: %v", p.Name(), err)
			bot.SendText(fmt.Sprintf("❌ Failed to parse statement (%s): %v", p.Name(), err)) //nolint:errcheck
			return
		}
		result = r
		parsed = true
		break
	}
	if !parsed {
		log.Warnf("statement: no parser matched subject=%q", ev.Subject)
		bot.SendText(fmt.Sprintf("❌ No parser matched statement email: %q", ev.Subject)) //nolint:errcheck
		return
	}

	postings, err := pc.Postings()
	if err != nil {
		log.Errorf("reconcile: fetch postings: %v", err)
		bot.SendText(fmt.Sprintf("❌ Failed to fetch ledger for %s: %v", ev.LedgerAccount, err)) //nolint:errcheck
		return
	}

	var entries []reconcile.LedgerEntry
	for _, p := range postings {
		if p.Account != ev.LedgerAccount {
			continue
		}
		if int(p.Date.Month()) != int(result.Month) || p.Date.Year() != result.Year {
			continue
		}
		entries = append(entries, reconcile.LedgerEntry{
			Date:        p.Date,
			Description: p.Payee,
			Amount:      p.Amount,
		})
	}

	diff := reconcile.Compare(result, entries)
	diff.Account = ev.LedgerAccount

	rec := reconcile.Record{
		Period:      fmt.Sprintf("%04d-%02d", result.Year, int(result.Month)),
		GeneratedAt: time.Now(),
		Diff:        diff,
	}
	if err := reconcile.Write(journalDir, rec); err != nil {
		log.Errorf("reconcile: write store: %v", err)
	}

	// Telegram report.
	var sb strings.Builder
	acctShort := ev.LedgerAccount
	if idx := strings.LastIndex(acctShort, ":"); idx >= 0 {
		acctShort = acctShort[idx+1:]
	}
	fmt.Fprintf(&sb, "📊 %s — %s %d\n\n", acctShort, result.Month, result.Year)
	fmt.Fprintf(&sb, "Closing balance: ₹%.2f\n", result.ClosingBalance)
	fmt.Fprintf(&sb, "Statement txns: %d | Matched in ledger: %d\n\n",
		len(result.Transactions), len(result.Transactions)-len(diff.Missing))

	if len(diff.Missing) == 0 && len(diff.Extra) == 0 {
		sb.WriteString("✅ All transactions matched\n")
	}
	if len(diff.Missing) > 0 {
		fmt.Fprintf(&sb, "❌ %d missing from ledger:\n", len(diff.Missing))
		for _, tx := range diff.Missing {
			if tx.Debit > 0 {
				fmt.Fprintf(&sb, "  • %s %s  -₹%.2f\n", tx.Date.Format("02-01"), tx.Description, tx.Debit)
			} else {
				fmt.Fprintf(&sb, "  • %s %s  +₹%.2f\n", tx.Date.Format("02-01"), tx.Description, tx.Credit)
			}
		}
	}
	if len(diff.Extra) > 0 {
		fmt.Fprintf(&sb, "⚠️ %d extra in ledger (not in statement):\n", len(diff.Extra))
		for _, le := range diff.Extra {
			fmt.Fprintf(&sb, "  • %s %s  %.2f\n", le.Date.Format("02-01"), le.Description, le.Amount)
		}
	}

	if err := bot.SendText(sb.String()); err != nil {
		log.Errorf("reconcile: send Telegram report: %v", err)
	}
}
```

- [ ] **Step 4: Wire Gmail poller into `main()`**

In the `main()` function, after the `bot := telegram.NewBot(...)` line, add:

```go
	var gmailClient *gmail.Client
	var gmailPoller *gmail.Poller

	if cfg.Gmail != nil {
		var err error
		gmailClient, err = gmail.New(cfg.Gmail.ClientID, cfg.Gmail.ClientSecret, cfg.Gmail.TokenFile)
		if err != nil {
			log.Fatalf("gmail: init: %v", err)
		}

		parsers := []statement.Parser{&statement.AxisParser{}}
		pc := paisaclient.New(cfg.Paisa.URL)
		stateDir := filepath.Dir(cfg.Gmail.TokenFile)

		var subjectMatches []gmail.SubjectMatch
		for _, sa := range cfg.Gmail.Accounts {
			subjectMatches = append(subjectMatches, gmail.SubjectMatch{
				Pattern:       sa.SubjectMatch,
				LedgerAccount: sa.LedgerAccount,
			})
		}

		gmailPoller = gmail.NewPoller(gmailClient, subjectMatches, stateDir, func(ev gmail.StatementEmail) {
			handleStatementEmail(ev, parsers, pc, cfg.Paisa.JournalDir, bot)
		})

		if !gmailClient.IsAuthorized() {
			authURL := gmailClient.AuthURL()
			msg := fmt.Sprintf("Gmail auth required. Opening browser for OAuth...\nIf browser doesn't open, visit:\n%s", authURL)
			bot.SendText(msg) //nolint:errcheck
			log.Infof("gmail: auth required — starting local callback server on :8787")
			go func() {
				code, err := gmail.OAuthCallbackServer()
				if err != nil {
					log.Errorf("gmail: oauth callback: %v", err)
					bot.SendText("❌ Gmail OAuth timed out. Restart the agent to try again.") //nolint:errcheck
					return
				}
				if err := gmailClient.ExchangeCode(code); err != nil {
					log.Errorf("gmail: exchange code: %v", err)
					bot.SendText(fmt.Sprintf("❌ Gmail auth failed: %v", err)) //nolint:errcheck
					return
				}
				bot.SendText("✅ Gmail connected — will poll for statements every 5 minutes") //nolint:errcheck
				log.Infof("gmail: authorised")
				go gmailPoller.Start()
			}()
		} else {
			go gmailPoller.Start()
		}
	}
```

- [ ] **Step 5: Build**

```bash
go build ./cmd/paisa-agent/
```
Expected: no errors. Fix any import errors flagged by the compiler (e.g., missing `"strings"`, `"path/filepath"`).

- [ ] **Step 6: Run full test suite**

```bash
go test ./internal/agent/... 2>&1 | grep -E "FAIL|ok"
```
Expected: all `ok`.

- [ ] **Step 7: Commit**

```bash
git add cmd/paisa-agent/main.go
git commit -m "feat(agent): wire Gmail poller and statement reconciliation pipeline"
```

---

### Task 12: Build arm64 binary and deploy

**Files:**
- `paisa-agent` binary (repo root)

- [ ] **Step 1: Build arm64 binary**

```bash
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o paisa-agent ./cmd/paisa-agent/
```

- [ ] **Step 2: Verify binary**

```bash
file paisa-agent
```
Expected: `paisa-agent: Mach-O 64-bit executable arm64`

- [ ] **Step 3: Deploy**

Run `/deploy` in Claude Code (or `./local_deploy.sh`).
Expected: both `paisa PID` and `paisa-agent PID` shown.

- [ ] **Step 4: Smoke test**

If Gmail is configured with valid credentials:
1. The paisa-agent log should show: `gmail: auth required — starting local callback server on :8787` (first run) or `gmail: polling for statements` (if already authed).
2. To trigger reconciliation: forward a real Axis Bank statement email to your Gmail, or wait for the next statement.
3. Once processed: check Telegram for the `📊 AXIS6386 — May 2026` report.
4. Open http://localhost:7500/more/doctor — any reconciliation issues appear there.

- [ ] **Step 5: Final commit if any build artifacts were changed**

```bash
git status
# only commit binary changes if the repo tracks the binary:
# git add paisa-agent && git commit -m "build: update paisa-agent arm64 binary"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|-----------------|------|
| Gmail config types | Task 1 |
| Paisaclient Postings() | Task 2 |
| Statement Parser interface + Transaction types | Task 3 |
| Reconcile Compare (missing/extra) | Task 4 |
| Reconcile JSON store with upsert | Task 5 |
| External deps (ledongthuc/pdf, oauth2, gmail) | Task 6 |
| Axis Bank PDF parser | Task 7 |
| Gmail OAuth client (Search, DownloadPDF, MarkRead, local callback) | Task 8 |
| Gmail poller (5-min interval, dedup via processed-emails.json) | Task 9 |
| Doctor page integration | Task 10 |
| Main.go wiring + Telegram report | Task 11 |
| Build + deploy | Task 12 |

**Placeholder scan:** None found.

**Type consistency check:**
- `statement.Transaction` used in `reconcile.Compare`, `reconcile.Diff.Missing`, `reconcile.Record`, and `handleStatementEmail` — all consistent.
- `reconcile.LedgerEntry` created in Task 4, used in Task 11's `handleStatementEmail` — consistent.
- `gmail.StatementEmail` created in Task 9, handled in Task 11 — consistent.
- `statement.Parser` interface created in Task 3, implemented in Task 7 (`AxisParser`), used in Task 11 — consistent.
- `config.GmailConfig.Accounts` is `[]StatementAccount`, iterated in Task 11 to build `[]gmail.SubjectMatch` — consistent.

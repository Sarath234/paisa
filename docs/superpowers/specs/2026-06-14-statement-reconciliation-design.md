# Statement Reconciliation — Design Spec
Date: 2026-06-14

## Problem

Bank statements arrive by email at month-end. There is no automated check that all transactions are correctly recorded in the ledger, or that the closing balance matches. Discrepancies (missed SMS, duplicate entry, wrong amount) go undetected until manual review.

## Scope

- Trigger: Axis Bank statement email arrives in Gmail
- Action: parse PDF, compare against ledger, report via Telegram + doctor page
- v1: Axis Savings Account (`AXIS6386`) only
- Extensible: `Parser` interface allows adding HDFC, ICICI CC statements later
- No auto-ledger-entry creation — reconciliation is a report, not a fixer

## Flow

```
Gmail polling loop (every 5 min)
  └─ Search unread emails matching configured subject patterns
       └─ Download PDF attachment
            └─ statement.Parse(pdf) → ParseResult{[]Transaction, ClosingBalance}
                 └─ paisaclient.GetTransactions(account, month) → []LedgerEntry
                      └─ reconcile.Compare(statement, ledger) → Diff
                           └─ reconcile.Store.Write(result) → reconciliation.json
                           └─ bot.SendText(Telegram report)
                           └─ gmail.MarkRead(emailID)
```

## Architecture

### New packages (paisa-agent)

**`internal/agent/gmail/`**

`client.go` — Gmail API via OAuth2:
```go
type Client struct { service *gmail.Service }

func NewClient(cfg GmailConfig) (*Client, error)
func (c *Client) Search(query string) ([]Message, error)
func (c *Client) DownloadAttachment(msgID, attachID string) ([]byte, error)
func (c *Client) MarkRead(msgID string) error
```

`poller.go` — Periodic poll loop (5-minute interval), emits `StatementEmail` events into the main dispatch loop.

---

**`internal/agent/statement/`**

`parser.go` — Shared types + interface:
```go
type Transaction struct {
    Date        time.Time
    Description string
    Debit       float64
    Credit      float64
    Balance     float64
}

type ParseResult struct {
    Transactions   []Transaction
    ClosingBalance float64
    Account        string     // detected from statement header
    Month          time.Month
    Year           int
}

type Parser interface {
    Name() string
    Detect(emailSubject string) bool // true if this parser handles the email
    Parse(pdfBytes []byte) (ParseResult, error)
}
```

`axis.go` — Axis Bank savings account parser:
- Text extracted from PDF pages using `github.com/ledongthuc/pdf`
- Transaction rows anchored on `DD-MM-YYYY` date pattern
- Columns: Date | Tran Date | Particulars | Chq no. | Withdrawal (Dr) | Deposit (Cr) | Balance
- Multi-line particulars joined until next date-anchored row
- Parsing stops at credit card/deposits summary section header
- Closing balance = last row's Balance field
- Indian number format parsed (commas stripped before `strconv.ParseFloat`)

---

**`internal/agent/reconcile/`**

`reconcile.go` — Comparison algorithm:
```go
type LedgerEntry struct {
    Date        time.Time
    Description string
    Amount      float64 // negative = debit, positive = credit
}

type Diff struct {
    Account        string
    Month          time.Month
    Year           int
    StatementClose float64
    LedgerClose    float64
    BalanceMatch   bool
    Missing        []Transaction  // in statement, not matched in ledger
    Extra          []LedgerEntry  // in ledger, not matched in statement
}

func Compare(result ParseResult, ledger []LedgerEntry) Diff
```

Matching: same calendar date + |amount| within ₹0.01 (statement debit = negative ledger amount, credit = positive).

`store.go` — Persist results to `<journal_dir>/reconciliation.json`:
```go
type ReconciliationRecord struct {
    Account     string
    Period      string    // "2026-05"
    GeneratedAt time.Time
    Diff        Diff
}

func Write(journalDir string, r ReconciliationRecord) error
func ReadAll(journalDir string) ([]ReconciliationRecord, error)
```

One file; each run upserts the record for `Account+Period`. Old months stay in the file until cleared manually.

### Changes to existing packages

**`internal/server/doctor.go`**

New rule `ruleStatementReconciliation`:
- Reads `reconciliation.json` from the configured journal directory
- For each `ReconciliationRecord`:
  - Balance mismatch → `WARN` issue: "Balance mismatch: AXIS6386 May 2026"
  - Missing transactions → `WARN` issue per account: "3 transactions missing from ledger: AXIS6386 May 2026"
  - Both clean → no issue (record ignored)

**`cmd/paisa-agent/main.go`**

- Instantiate `gmail.Poller` alongside Telegram polling
- Handle `StatementEmail` events: parse → reconcile → store → Telegram report
- Handle OAuth auth-code message: `"auth <code>"` → exchange token → store

**`paisa-agent.yaml`** — new `gmail:` section:
```yaml
gmail:
  client_id: "..."
  client_secret: "..."
  token_file: "~/.paisa-agent/gmail-token.json"
  statement_accounts:
    - subject_match: "6386"
      ledger_account: "Assets:Checking:AXIS6386"
```

## OAuth Setup Flow

First run (no token file):
1. Agent sends Telegram message: "Gmail auth required. Open this URL and reply with `auth <code>`: https://accounts.google.com/..."
2. User opens URL, approves, copies the auth code
3. User replies via Telegram: `auth 4/0AX4XfWi...`
4. Agent exchanges code for refresh token, writes to `token_file`
5. Telegram: "✅ Gmail connected"

Subsequent runs: token file loaded silently; refresh handled automatically by the OAuth2 library.

## Telegram Report Format

```
📊 Axis AXIS6386 — May 2026

Balance: ✅ ₹9,25,264.20 matches

Transactions: 47 statement / 44 ledger
❌ 3 missing from ledger:
  • 05-05 IRCTC RAIL            -₹1,200.00
  • 12-05 AMAZON PAY            -₹3,499.00
  • 28-05 UPI-HDFC CC PMT      -₹40,000.00

✅ No extra ledger entries
```

Balance mismatch variant:
```
Balance: ❌ Statement ₹9,25,264.20 vs Ledger ₹9,15,264.20 (Δ ₹10,000.00)
```

## Doctor Page Integration

`/more/doctor` already renders `Issue{Level, Summary, Description, Details}`. The new rule adds reconciliation issues to the same list:

| Condition | Level | Summary | Details |
|-----------|-------|---------|---------|
| Balance mismatch | WARN | "Balance mismatch: AXIS6386 May 2026" | "Statement ₹X vs Ledger ₹Y (Δ ₹Z)" |
| Missing transactions | WARN | "3 transactions missing: AXIS6386 May 2026" | Bulleted list of transactions |
| Extra ledger entries | WARN | "2 extra entries in ledger: AXIS6386 May 2026" | Bulleted list |

Clean months produce no issues and disappear from the doctor page automatically.

## Deduplication

`~/.paisa-agent/processed-emails.json` stores processed Gmail message IDs. On each poll, IDs already in the file are skipped. File is append-only (never shrunk); safe across restarts.

## Error Handling

| Error | Action |
|-------|--------|
| Gmail API error | Log warn; retry next 5-min poll cycle |
| PDF parse failure | Telegram: "❌ Failed to parse Axis statement: `<error>`" |
| No matching account in config | Log warn; Telegram: "❌ No account mapping for statement: `<subject>`" |
| Ledger query failure | Telegram: "❌ Ledger query failed for AXIS6386 May 2026: `<error>`" |
| Token expired / revoked | Telegram: restart OAuth flow message |

## Testing

- `statement.AxisParser` — golden file: sample extracted PDF text → expected `[]Transaction` + closing balance
- `reconcile.Compare` — table tests: all match, 3 missing, 2 extra, balance mismatch, empty statement
- `reconcile.Store` — write + read round-trip; upsert replaces existing period
- `gmail.Client` — mock HTTP server tests for search, download, mark-read

## Files Changed

| File | Change |
|------|--------|
| `internal/agent/gmail/client.go` | New — OAuth2 client, search, download, mark-read |
| `internal/agent/gmail/poller.go` | New — 5-min poll loop emitting StatementEmail events |
| `internal/agent/statement/parser.go` | New — Parser interface, Transaction, ParseResult types |
| `internal/agent/statement/axis.go` | New — Axis savings account PDF parser |
| `internal/agent/reconcile/reconcile.go` | New — Compare algorithm, Diff type |
| `internal/agent/reconcile/store.go` | New — JSON persistence for reconciliation results |
| `cmd/paisa-agent/main.go` | Wire Gmail poller; handle StatementEmail + auth-code messages |
| `internal/server/doctor.go` | New rule: read reconciliation.json, emit WARN issues |
| `docs/` | Paisa-agent.yaml example updated with `gmail:` section |

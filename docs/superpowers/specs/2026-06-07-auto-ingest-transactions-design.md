# Auto-Ingest Transactions Design

**Date:** 2026-06-07  
**Branch:** feat/auto-ingest-trans  
**Status:** Approved for implementation

## Overview

A standalone Go sidecar (`paisa-agent`) that ingests bank SMS messages forwarded via iPhone Shortcuts → Telegram bot, parses them into ledger journal entries, sends a Telegram approval draft, and on approval appends to `auto-import.ledger`.

## Pipeline

```
Bank SMS → iPhone Shortcuts → Telegram message → paisa-agent polls →
regex parse → (LLM fallback) → Telegram draft + inline buttons →
✅ Approve → append to auto-import.ledger
```

## Architecture

### Binary

`cmd/paisa-agent/main.go` — loads config, starts Telegram long-poll loop.

### Package Layout

```
internal/agent/
  config/
    config.go       — load paisa-agent.yaml, validate rules
  parser/
    parser.go       — classify message → AccountRule, dispatch to bank parser
    banks.go        — regex extractor per bank format
    merchant.go     — keyword routing: merchant string → account + description
  llm/
    ollama.go       — Ollama HTTP client, fill missing Entry fields
  telegram/
    bot.go          — long-poll getUpdates, dispatch message + callback_query events
    format.go       — render Entry as Telegram message + inline keyboard
  approval/
    state.go        — in-memory map: messageID → {Entry, status}, FSM
  ledger/
    entry.go        — Entry struct + format as ledger block
    appender.go     — append to auto-import.ledger, create + include if absent
```

### Entry Struct

The common currency between all components:

```go
type Entry struct {
    Date string // "2026/06/03"
    Desc string // "Food Swiggy"
    Src  string // "Assets:Checking:FC2148"  — first posting account
    Amt  string // "-215.00 INR"             — amount on first posting
    Dest string // "Expenses:Food:Hyd"       — second posting (auto-balanced)
}
```

Ledger output:
```
{Date} {Desc}
    {Src}    {Amt}
    {Dest}
```

> Note: `Entry.Src` maps to YAML `destinations` (the account with the explicit amount on line 1). `Entry.Dest` maps to YAML `src` (the auto-balanced account on line 2). The naming in YAML describes money flow direction; the naming in Entry describes ledger line order.

## Config (paisa-agent.yaml)

```yaml
paisa:
  url: http://localhost:7500
  journal_dir: /path/to/journal

ollama:
  url: http://localhost:11434
  model: gemma3:4b

telegram:
  bot_token: "..."
  chat_id: 123456789

parser_rules:
  accounts:
    # Fixed routes first (all identifiers must match — AND logic)
    - bank: fixed
      identifiers: ["CRD-PMNT", "8860"]
      src: "Assets:Checking:AXIS6386"
      destinations: "Liabilities:CreditCard:FK8860"
      description: "CC Payment"

    # Format routes after fixed
    - bank: hdfc_debit
      identifiers: ["HDFC Bank Card 2148"]
      destinations: "Assets:Checking:FC2148"

  merchants:
    - keyword: "swiggy"
      account: "Expenses:Food:Hyd"
      description: "Food Swiggy"
```

**Field semantics:**
- `destinations` — first ledger posting account (explicit amount)
- `src` — second ledger posting account (auto-balanced); fixed routes only
- `description` — journal description line; fixed routes only
- `identifiers` — ALL substrings must appear in the SMS (AND match)
- Fixed routes must appear before format routes in the `accounts` list

## Parser Strategy

### Classification

Scan `accounts` list top-to-bottom. First rule where ALL identifiers are present in the SMS wins. Fixed routes listed first ensure they win over format routes on overlapping messages.

### Fixed Route Extraction

No bank-specific regex. Run a generic amount+date extractor against the SMS:
- Amount: patterns `INR X`, `Rs X`, `INR.X`, `Rs.X` — strip Indian commas
- Date: all 5 normalisation patterns (see below)
- Direction: detect "debited"/"debit"/"spent" → negative on `destinations`; "credited"/"credit"/"received" → positive

All other fields (Desc, Src, Dest) come directly from the YAML rule.

### Format Route Extraction

One regex extractor function per `bank` key:

| Bank key | SMS pattern | Date format | Merchant location |
|---|---|---|---|
| `icici_cc` | `INR {amt} spent ... on {date} on {merchant}` | DD-Mon-YY | after last "on " |
| `hdfc_debit` | `Spent! INR INR {amt} On ... At {merchant} On {date}` | DD Mon,YYYY | between "At" and "On {date}" |
| `hdfc_cc` | `Spent Rs.{amt} On ... At {merchant} On {date}:{time}` | YYYY-MM-DD | between "At" and "On {date}" |
| `axis_checking` | `INR {amt} debited/credited\n...\n{date}, {time}\nUPI/.../{merchant}` | DD-MM-YY | last human segment of UPI line |
| `axis_cc` | `Spent INR {amt}\n...\n{date} {time} IST\n{merchant}\n` | DD-MM-YY | line 4 |
| `idfc_checking` | `Spent Rs.{amt} from ... at {merchant} on {date}` or `INR.{amt} earned ... on {date}` | DD/MM/YY | after "at " |

**Amount edge cases:**
- `INR INR 215` (HDFC double-INR) — regex skips first INR
- Indian comma format `10,468.87` — strip commas before parsing
- No decimal `11864` — normalise to `11864.00`

### Date Normalisation

All formats → `YYYY/MM/DD`:

| Input | Example | Output |
|---|---|---|
| DD-Mon-YY | 03-Jun-26 | 2026/06/03 |
| DD Mon,YYYY | 03 Jun,2026 | 2026/06/03 |
| YYYY-MM-DD | 2026-05-21 | 2026/05/21 |
| DD-MM-YY | 03-06-26 | 2026/06/03 |
| DD/MM/YY | 09/04/26 | 2026/04/09 |

Month abbreviations are case-insensitive (Jun / JUN / jun).

### Merchant Routing

After extraction, lowercase the merchant string and scan `merchants` list top-to-bottom. First rule where `keyword` is a substring of the merchant string wins → sets `Entry.Dest` and `Entry.Desc`.

### LLM Fallback

Called only when `Entry.Dest` or `Entry.Desc` is still empty after merchant routing.

Prompt includes: raw SMS text + partially filled Entry. Asks Ollama to return only the missing fields as JSON:
```json
{"desc": "...", "dest": "..."}
```

Date, amount, and src are never sent to LLM (always regex-extracted). If LLM also fails or times out, the Entry is sent to Telegram with the missing fields blank for manual fill via Edit.

## Telegram Approval Flow

### Draft Format

```
📨 New Transaction

desc: Food Swiggy
date: 2026/06/03
src:  Assets:Checking:FC2148
amt:  -215.00 INR
dest: Expenses:Food:Hyd

[✅ Approve]  [✏️ Edit]  [⏭ Skip]
```

### State Machine

States per `(chatID, messageID)`:

```
pending  →  ✅ Approve  →  ledger.Append(entry) → bot edits: "✅ Posted" → done
pending  →  ✏️ Edit     →  editing
pending  →  ⏭ Skip     →  bot edits: "⏭ Skipped" → done

editing  →  text reply  →  parse corrections → rebuild Entry → new draft sent → pending
```

State is in-memory only. If paisa-agent restarts, pending buttons become unresponsive (acceptable — no DB dependency).

### Edit Reply Format

Bot sends the current 5-field block as a template. User replies with only the lines they want to change:
```
desc: Groceries Zepto
dest: Expenses:Groceries:Hyd
```
Parser does key:value scan, merges changed fields onto the existing Entry, re-sends draft with buttons.

## File Append

**Target:** `{journal_dir}/auto-import.ledger`

**First run:** create file if absent. Check main journal for `include auto-import.ledger`; append include line if missing.

**Per approval:** append blank-line-separated ledger block:
```
2026/06/03 Food Swiggy
    Assets:Checking:FC2148             -215.00 INR
    Expenses:Food:Hyd

```

**Amount formatting:**
- Always 2 decimal places
- No Indian comma separators in output
- Negative prefix for debits, no prefix for credits/payments

**Duplicate check:** before appending, scan last 500 lines for same date + amount + src account. If found, send Telegram warning:
```
⚠️ Possible duplicate — matching entry exists. Post anyway?
[✅ Post anyway]  [⏭ Skip]
```

**Concurrency:** `sync.Mutex` around file append.

## Per-Message Walkthrough

All 16 messages from examples file produce the following Telegram drafts:

| # | SMS | Rule | Telegram Draft (desc / amt / src → dest) |
|---|-----|------|------------------------------------------|
| 1 | ICICI CC Amazon | icici_cc | Utils: Amazon Pay / -453.00 / ICICI6009 → Utils:Hyd |
| 2 | ICICI CC payment | fixed | CC Payment / 10468.87 / ICIC6009 → AXIS6386 |
| 3 | HDFC Debit Swiggy | hdfc_debit | Food Swiggy / -215.00 / FC2148 → Food:Hyd |
| 4 | HDFC Debit Blink | hdfc_debit | Groceries Blink / -426.00 / FC2148 → Groceries:Hyd |
| 5 | HDFC Debit Zomato | hdfc_debit | Food Zomato / -327.25 / FC2148 → Food:Hyd |
| 6 | Axis IRCTC | axis_checking | Travel / -1804.05 / AXIS6386 → Travel:Hyd |
| 7 | Axis KONDAVEET | fixed | Rent from Haritha / 30000.00 / AXIS6386 → AXISHARITHA |
| 8 | IDFC interest | idfc_checking | Bank Interest / 318.00 / IDFC6977 → Income:Interest:IDFC6977 |
| 9 | IDFC ZEPTO | idfc_checking | Groceries ZEPTO / -473.00 / IDFC6977 → Groceries:Hyd |
| 10 | HDFC CC ZEPTO | hdfc_cc | Groceries ZEPTO / -341.00 / HDFC2527 → Groceries:Hyd |
| 11 | Axis CC DISTRICT | axis_cc | Entertainment: DISTRICT / -210.12 / MyZone1610 → Entertainment:Hyd |
| 12 | Axis CC FLIPKART | axis_cc | Utils: Flipkart / -11864.00 / SELECT6792 → Utils:Hyd |
| 13 | Axis CC Ing*Flipkar | axis_cc | Utils: Flipkart / -3468.00 / FK8860 → Utils:Hyd |
| 14 | CRD-PMNT FK8860 | fixed | CC Payment / 1417.00 / FK8860 → AXIS6386 |
| 15 | CRD-PMNT MyZone1610 | fixed | CC Payment / 1417.00 / MyZone1610 → AXIS6386 |
| 16 | CRD-PMNT SELECT6792 | fixed | CC Payment / 1417.00 / SELECT6792 → AXIS6386 |

## YAML Fixes Required Before Implementation

| # | Issue | Fix |
|---|-------|-----|
| 1 | `Income:Interest` too generic | Add per-bank interest merchant rules (e.g. `Income:Interest:IDFC6977`) |
| 2 | Amazon description order | Update YAML merchant rule `description` to preferred wording |

## Out of Scope

- Auto-sync triggering paisa after append (can call `POST /api/sync` later as enhancement)
- Telegram webhook mode (long-poll is sufficient for single-user)
- Persistent approval state across restarts
- Gmail / other SMS ingestion channels
- Auto-approval threshold (can add merchant_rules config later)

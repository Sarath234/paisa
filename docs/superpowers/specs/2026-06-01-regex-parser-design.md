# Regex Parser Design — 2026-06-01

## Goal

Replace the pure-LLM parse path for known bank SMS/alert formats with a fast, deterministic regex parser. The LLM (Ollama) remains as a fallback for messages that match no source rule. All hardcoded account names, card identifiers, merchant keywords, and time thresholds move to YAML config — the Go code contains only generic logic.

---

## Architecture

```
Parse(msg)
  └─ RegexParse(msg, cfg.ParserRules)  → matched? → return tx (Confidence=1.0)
                                        → no match → Ollama → return tx
```

The `Parser` struct gains a `rules ParserRules` field populated from config. `RegexParse` is a standalone function (easy to test without HTTP). The LLM path is unchanged.

---

## Config Changes

`accounts: map[string]string` is **removed**. All source account knowledge moves into `parser_rules.sources`. The LLM fallback's `resolveAccount` walks the sources list instead of a separate map.

### New config shape

```yaml
parser_rules:
  day_parts:
    breakfast_end: 11   # hours [0, breakfast_end)
    lunch_end: 15       # hours [breakfast_end, lunch_end)
    dinner_end: 20      # hours [lunch_end, dinner_end), else Evening Snack

  merchants:            # first keyword match wins (case-insensitive)
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
    # fallback entry with empty keyword = catch-all
    - keyword: ""
      description: "Misc"
      account: "Expenses:Misc:Hyd"

  sources:              # first rule where ALL contains strings match wins
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

`dest_account` + `description` on a source rule are optional overrides — when set, merchant matching is skipped entirely (used for loan EMI, recurring fixed payments).

### Go config structs (additions to config.go)

```go
type ParserRules struct {
    DayParts  DayPartsConfig   `yaml:"day_parts"`
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
    Account     string   `yaml:"account"`   // source (debit) account
    DestAccount string   `yaml:"dest_account"` // optional: override dest account
    Description string   `yaml:"description"`  // optional: override description
}
```

`Config.Accounts map[string]string` is removed. `Config` gains `ParserRules ParserRules`.

Default `DayParts`: `BreakfastEnd=11`, `LunchEnd=15`, `DinnerEnd=20`.

---

## regex_parser.go

### Date parsing — 7 patterns tried in order

| # | Pattern | Example |
|---|---------|---------|
| 1 | `on DD-MM-YYYY` | `on 14-05-2025` |
| 2 | `DD-MM-YY` (2-digit year) | `14-05-25` |
| 3 | `on Mon DD, YYYY` | `on May 14, 2025` |
| 4 | `DDMONYY` | `14MAY25` |
| 5 | `on D MON,YYYY` | `on 14 MAY,2025` |
| 6 | `on DD/MM/YYYY` | `on 14/05/2025` |
| 7 | `Month D, YYYY` | `May 14, 2025` |

All patterns output `YYYY-MM-DD` (consistent with LLM path). `journal.Format` normalises to `YYYY/MM/DD`.

2-digit years are prefixed with the current century (`20`).

### Amount parsing

Two patterns tried in order, commas stripped:
1. `(?i)Rs\.?\s*([0-9,]+(?:\.[0-9]+)?)` 
2. `(?i)INR\s*([0-9,]+(?:\.[0-9]+)?)`

### Day-part resolution

Extract `HH` from first `HH:MM[:SS]` timestamp in message. Apply thresholds from config. If no timestamp found, substitute `"Meal"` for `{day_part}`.

### Merchant matching

Iterate `cfg.Merchants` in order. `strings.Contains(strings.ToLower(msg), strings.ToLower(keyword))`. First match wins. Empty keyword = catch-all fallback.

### Source matching

Iterate `cfg.Sources` in order. For each rule, all strings in `Contains` must appear in the message (case-sensitive, matching JS behaviour). First full match wins.

### RegexParse signature

```go
func RegexParse(msg string, rules config.ParserRules) (ParsedTransaction, bool)
```

Returns `(zero, false)` if no source rule matches or date/amount cannot be extracted.

---

## ParsedTransaction changes

Add one field:

```go
SourceAccount string `json:"source_account,omitempty"`
```

Regex path sets this to `src.Account`. LLM path leaves it empty.

---

## Pipeline changes

### post()

```go
debitAccount := tx.SourceAccount
if debitAccount == "" {
    debitAccount = p.resolveAccount(tx.Bank, tx.AccountLast4)
}
```

### resolveAccount()

Updated to walk `cfg.ParserRules.Sources` looking for a source whose account contains the `last4` suffix, as a best-effort fallback for LLM-parsed transactions from known banks. The `accounts` map is gone.

---

## Special cases NOT handled by regex parser (→ LLM fallback)

| Case | Reason |
|------|--------|
| Salary (Axis + "tige") | 7-posting entry with NPS/EPF/ESOP splits — not expressible in single-posting model |
| CC payment (`cred.club@`) | 3-posting entry with cashback income leg |

Both produce messages that match no source rule → fall through to Ollama automatically.

---

## Testing

`regex_parser_test.go` covers:
- All 7 date patterns
- Both amount patterns (Rs and INR, with/without commas)
- Day-part boundaries (10:59, 11:00, 14:59, 15:00, 19:59, 20:00)
- Merchant keyword matching + `{day_part}` substitution
- Source matching with multi-`contains` rule (canara loan)
- Source with `dest_account` override skips merchant matching
- No source match returns `(zero, false)`
- `Parse()` uses regex result when matched, calls Ollama when not

`parser_test.go` updated: `New()` signature gains `ParserRules` arg (pass empty struct in existing tests).

---

## Files changed

| File | Change |
|------|--------|
| `internal/agent/config/config.go` | Drop `Accounts`; add `ParserRules` + sub-types |
| `internal/agent/parser/parser.go` | Add `SourceAccount` to `ParsedTransaction`; `Parser` gains `rules`; `Parse()` tries regex first |
| `internal/agent/parser/regex_parser.go` | New — all regex logic |
| `internal/agent/parser/regex_parser_test.go` | New — full coverage |
| `internal/agent/parser/parser_test.go` | Update `New()` call |
| `internal/agent/pipeline/pipeline.go` | `post()` uses `SourceAccount`; update `resolveAccount()` |
| `scripts/paisa-agent.yaml.example` | New — full working config with user's real accounts |

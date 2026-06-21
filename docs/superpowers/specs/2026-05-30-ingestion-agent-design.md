# Paisa Ingestion Agent — Design Spec

## Goal

Automate transaction ingestion from bank SMS alerts, Gmail transaction emails, and monthly bank statements into Paisa's ledger journal — with smart approval gating via Telegram — while making zero changes to the Paisa binary.

---

## Architecture Overview

A standalone Go binary `paisa-agent` runs as a macOS launchd service on the same machine as Paisa. It has no shared code with Paisa. The only coupling is:

1. **Write access** to `auto-imported.journal` — a plain hledger text file included by the main journal
2. **HTTP call** to Paisa's existing `POST /api/sync` after each write

If the agent is down, Paisa continues working normally. Manual entry continues to work. Nothing breaks.

```
iPhone SMS
  → iPhone Shortcut (fires on bank sender IDs)
  → Telegram Bot (user's private bot)
  → paisa-agent (long-polls Telegram getUpdates every 2s)

Gmail
  → paisa-agent polls Gmail REST API every 5 min
  → detects: alert email (parse body) vs statement email (parse PDF attachment)

Both paths → Ollama (local Gamma model, JSON schema) → normalized Transaction
           → Dedup check (agent.db: imported_refs)
           → Merchant gate (agent.db: merchant_rules)
               known + below threshold → append to auto-imported.journal → POST /api/sync
               unknown / high-value   → Telegram approval card → on ✓ → same write path
```

---

## Sources

### Gmail (REST API)

The agent polls Gmail using the **Gmail REST API v1** with OAuth2 service credentials (one-time browser auth to generate `~/.config/paisa-agent/gmail-token.json`). It filters by a configurable label or sender list (e.g. labels: `bank`, `finance`, or senders: `alerts@hdfcbank.net`).

> Note: The Gmail MCP integration available in Claude Code sessions is Claude-specific and cannot be used inside a standalone Go binary. The Gmail REST API is the correct approach here — it provides the same read access via `users.messages.list` + `users.messages.get`.

**Email classification:**

| Type | Detection | Action |
|------|-----------|--------|
| Transaction alert | Subject contains: "debited", "credited", "transaction alert", "payment", "spent" — no PDF attachment | Parse email body → single Transaction |
| Statement | Has PDF attachment AND subject contains: "statement", "e-statement", "account summary" | Download PDF → parse all rows → bulk dedup + reconcile |

The agent tracks the Gmail message ID in `imported_refs` to avoid reprocessing.

### Telegram (SMS relay)

**iPhone Shortcut setup** (one-time, manual):
- Create a Shortcuts automation: "When I receive a message from [list of bank sender IDs]"
- Action: Get contents of message → URL session POST to `https://api.telegram.org/bot<TOKEN>/sendMessage` with `chat_id` and `text` = raw SMS body

The agent long-polls `getUpdates` (offset-tracking). No public URL or Tailscale needed for this direction. Telegram is also used for outbound approval cards (see Approval Gate below).

---

## Parsing

All raw text (SMS, email body, PDF text) goes through a single Ollama call.

**Model**: Local Gamma/Gemma model via Ollama HTTP API at `localhost:11434/v1/chat/completions`

**Output schema** (enforced via Ollama JSON mode):

```json
{
  "date": "2025-05-14",
  "amount": -2450.00,
  "currency": "INR",
  "merchant": "Swiggy",
  "account_last4": "1234",
  "bank": "HDFC",
  "ref_id": "47291830",
  "tx_type": "debit|credit|transfer|emi|charge",
  "suggested_ledger_account": "Expenses:Food:Dining",
  "confidence": 0.95
}
```

- `amount` is negative for debits (money leaving), positive for credits
- `suggested_ledger_account` is drawn from the user's existing account list (passed as context in the prompt)
- `confidence` < 0.7 routes to Telegram approval regardless of merchant rule

---

## Deduplication

Stored in `agent.db` table `imported_refs`:

```sql
CREATE TABLE imported_refs (
    id          INTEGER PRIMARY KEY,
    ref_id      TEXT,
    date        TEXT,
    amount      REAL,
    account     TEXT,
    source      TEXT,   -- 'sms' | 'gmail_alert' | 'gmail_statement'
    imported_at TEXT
);
```

**Match priority:**

1. **Exact ref_id match** → duplicate, skip (strongest signal; ref IDs are bank-assigned)
2. **Same (account + amount + date ±1 day)** → probable duplicate, flag for Telegram review
3. **No match** → new transaction, proceed to merchant gate

---

## Merchant Rule Engine (Smart Gate)

Stored in `agent.db` table `merchant_rules`:

```sql
CREATE TABLE merchant_rules (
    merchant    TEXT PRIMARY KEY,
    account     TEXT,
    approve_count INTEGER DEFAULT 0,
    auto_approve  INTEGER DEFAULT 0   -- 1 = auto-post silently
);
```

**Bootstrap on first run**: Agent scans the main journal and `auto-imported.journal`, extracts all `payee → account` pairings, inserts them with `approve_count = 3, auto_approve = 1`. Day-one auto-post rate is high for all existing merchants.

**Gate logic:**

| Condition | Action |
|-----------|--------|
| `auto_approve = 1` AND `amount < auto_approve_threshold` (default ₹10,000) | Auto-post silently |
| `confidence < 0.7` | Telegram approval (regardless of merchant rule) |
| Merchant not in rules | Telegram approval |
| `amount >= auto_approve_threshold` | Telegram approval (regardless of merchant rule) |

**On Telegram approval (✓):**
- Post transaction to journal
- Increment `approve_count`
- If `approve_count >= 3`, set `auto_approve = 1`

**On Telegram edit (✎):**
- Bot asks: "Which account?" with inline keyboard of common accounts
- User selects → post with corrected account → update `merchant_rules`

**On Telegram skip (✗):**
- Record ref_id in `imported_refs` with `source = 'skipped'` so it's not re-shown

---

## Journal Writing

The agent appends hledger-format entries to `auto-imported.journal` in the Paisa journal directory.

**One-time manual setup**: Add `include auto-imported.journal` to the user's main `.journal` file.

**Entry format:**

```
; source: sms | gmail_alert | gmail_statement
; ref: 47291830
2025/05/14 Swiggy
    Expenses:Food:Dining        ₹2,450
    Assets:HDFC:Savings
```

After each write, the agent calls `POST /api/sync` on Paisa's local HTTP server so the new transaction appears immediately in the UI.

---

## Month-end Statement Reconciliation

When a statement email is detected:

1. Download PDF attachment
2. Extract text via `pdftotext` (or pure-Go PDF parser)
3. Parse all rows through Ollama → list of Transactions
4. Run dedup against `imported_refs`:
   - **Gaps**: in statement but not imported → post them (with Telegram notification per gap)
   - **Anomalies**: in `imported_refs` but not in statement → flag in reconciliation summary
5. Send Telegram message: `"May statement reconciled: 2 gaps filled · 0 anomalies · ₹1,23,450 total"`

---

## Configuration

`paisa-agent.yaml` (in `~/.config/paisa-agent/` or same dir as `paisa.yaml`):

```yaml
paisa:
  url: http://localhost:7500
  journal_dir: ~/Documents/paisa     # path to journal directory
  api_token: ""                       # if Paisa token auth is enabled

ollama:
  url: http://localhost:11434
  model: gemma3:12b

telegram:
  bot_token: "123456:ABC..."
  chat_id: 987654321

gmail:
  credentials_file: ~/.config/paisa-agent/gmail-token.json   # OAuth2 token from one-time setup
  poll_interval_seconds: 300
  labels: ["bank", "finance"]         # Gmail labels to scan; empty = all inbox

merchant_rules:
  auto_approve_threshold: 10000       # INR; transactions above always need approval
  promote_after_approvals: 3          # approvals before auto_approve = 1
```

---

## Binary and Service

- Built with `go build ./cmd/paisa-agent`
- Installed as a macOS launchd service: `~/Library/LaunchAgents/com.paisa.agent.plist`
- Logs to `~/.local/share/paisa-agent/agent.log`
- `agent.db` at `~/.local/share/paisa-agent/agent.db`

---

## Paisa Changes

**None.** `POST /api/sync` already exists. The agent is a fully independent binary.

---

## Out of Scope (this spec)

- Payment execution (bill pay, UPI) — Action Engine is a separate sub-project
- Proactive intelligence / anomaly alerts — Intelligence Engine is a separate sub-project
- Web UI for reviewing pending transactions — Telegram is the mobile approval interface; Paisa's existing editor handles any manual corrections
- Android SMS ingestion — iPhone Shortcuts only for now

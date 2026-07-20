# Paisa

[![Matrix](https://img.shields.io/matrix/paisa%3Amatrix.org?logo=matrix)](https://matrix.to/#/#paisa:matrix.org)

**Paisa** is a Personal finance manager. It builds on
top of the [ledger](https://www.ledger-cli.org/) double entry accounting tool. Checkout
[documentation](https://paisa.fyi) to get started.

# Demo

A demo of the Web UI can be found at [https://demo.paisa.fyi](https://demo.paisa.fyi)

## Web UI additions (this fork)

On top of upstream Paisa, this fork adds:

- **Assistant page (`/assistant`) + chat widget:** Ask natural-language questions about your spending, budgets, balances, and net worth from the web UI — same Q&A engine the Telegram bot uses (requires paisa-agent running).
- **Quick-entry modal:** Add a transaction from any page, with an SMS parse mode — paste a bank SMS, the agent parses it into a double-entry posting, and you approve before it's written to the journal.
- **Global search:** Press `/` anywhere to search transactions and accounts.
- **Net worth projection (`/assets/projection`):** Forward projection of net worth from historical trends.
- **Rebalancing calculator:** On the allocation page — set target allocations and see the buy/sell amounts needed to rebalance.
- **Spending insights feed (`/insights`):** Rule-based insights across budget, income, expenses, and savings rate.
- **Theme switcher:** Light/dark toggle in the navbar.
- **Performance:** Concurrent journal parse and price sync, dashboard cache pre-warming — noticeably faster cold starts and saves on large journals.

## paisa-agent

A Go sidecar that connects Paisa to Telegram, Gmail, and a local LLM (via Ollama).

**Features:**

- **SMS → Ledger:** Forwards bank transaction SMSes from Telegram, parses them with regex + LLM fallback, and appends double-entry journal entries after user confirmation.
- **Merchant rule learning:** Proposes and saves merchant routing rules when you correct a categorisation; editable via Telegram inline buttons.
- **Finance Q&A:** Answer natural-language questions about your spending, net worth, budgets, and balances directly from Telegram.
- **Statement reconciliation:** Polls Gmail for bank statement emails, parses the PDF, compares transactions against your ledger, and sends a Telegram report highlighting missing or extra entries. Results also surface on the Paisa doctor page (`/more/doctor`). Statements can also be dropped as PDFs into a local folder (see `statements:` below) instead of, or alongside, Gmail.
- **Credit card monitors:** Scheduled checks that message you on Telegram — due-date reminders (with overdue escalation), statement-generated announcements, credit utilization warnings, and interest/late-fee detection on closed statements. Each insight is sent at most once; low-urgency ones batch into a daily digest.

**Config:** Copy `paisa-agent.yaml` to your preferred location and fill in:
- `paisa.url` and `paisa.journal_dir`
- `telegram.bot_token` and `telegram.chat_id`
- `ollama.url` and `ollama.model` (e.g. `gemma3:4b`)
- `gmail.*` (optional) — OAuth2 credentials for statement reconciliation (see setup below)

All the sections below (Gmail, Monitors, Statement drop folder) are optional top-level blocks added to this **same `paisa-agent.yaml`** file, as siblings of `paisa:`/`telegram:`/`ollama:` above — not separate files.

**Run:** `./paisa-agent --config /path/to/paisa-agent.yaml`

### Telegram setup

1. Open Telegram and message [@BotFather](https://t.me/BotFather) → `/newbot` → follow prompts to get a **bot token**.
2. Start a chat with your new bot (search its username and press Start).
3. Send any message to the bot, then open `https://api.telegram.org/bot<TOKEN>/getUpdates` in a browser. Find `"chat":{"id":...}` — that's your **chat ID**.
4. Add to `paisa-agent.yaml`:
   ```yaml
   telegram:
     bot_token: "1234567890:AAG..."
     chat_id: 5987311199
   ```

### Gmail setup (statement reconciliation)

1. Go to [Google Cloud Console](https://console.cloud.google.com) → create a project → enable the **Gmail API**.
2. Create **OAuth 2.0 credentials** (type: Desktop app). Copy the client ID and secret.
3. Add to `paisa-agent.yaml`:
   ```yaml
   gmail:
     client_id: "YOUR_CLIENT_ID"
     client_secret: "YOUR_CLIENT_SECRET"
     token_file: "/Users/YOU/.paisa-agent/gmail-token.json"
     statement_accounts:
       - subject_match: "6386"          # substring matched against email subject
         ledger_account: "Assets:Checking:AXIS6386"
   ```
4. Start the agent. On first run it sends you an OAuth URL via Telegram and starts a local server on `:8787` to capture the redirect. Open the URL in a browser, approve access, and the agent saves a refresh token automatically.
5. Subsequent restarts load the saved token silently — no re-auth needed.

### Monitors (credit card guardian)

Add a `monitors:` top-level block to **`paisa-agent.yaml`** (the same file as `telegram:`/`paisa:` above) to enable the scheduled checks. All fields are optional — an empty `monitors: {}` enables everything with the defaults shown:

```yaml
monitors:
  digest_hour: 8            # hour (0-23, local time) the daily digest and daily checks fire
  credit_cards:
    due_reminder_days: [3, 1, 0]     # days before the due date to remind (0 = on the day)
    utilization_bands: [50, 75, 90]  # warn when utilization crosses these percentages
    interest_patterns: ["INTEREST", "LATE FEE"]  # payee substrings that count as interest/fees
    truth_gap_days: 3         # nudge if no statement SMS/PDF arrives N days after the computed cycle close
```

Credit cards themselves are configured separately, in **Paisa's own `paisa.yaml`** (not `paisa-agent.yaml`) under a top-level `credit_cards:` block — the monitors just read them via the Paisa API. Monitor state (dedupe keys, digest queue) is stored as `monitor-state.json` in `paisa.journal_dir`.

### Statement drop folder

As an alternative to Gmail polling, add a `statements:` top-level block to **`paisa-agent.yaml`** to drop statement PDFs into a watched folder:

```yaml
statements:
  drop_dir: "~/paisa-statements"     # watched for new *.pdf files (checked every minute)
  accounts:
    - filename_match: "*6386*"       # case-insensitive glob matched against the file name
      ledger_account: "Assets:Checking:AXIS6386"
    - filename_match: "*6009*"
      ledger_account: "Liabilities:CreditCard:ICIC6009"
      kind: credit_card                  # routes to CC reconciliation instead of plain statement matching
      pdf_password: "<statement password>"  # only needed if the bank encrypts the PDF
```

Matched PDFs are parsed and reconciled exactly like Gmail statements. Processed files move to `<drop_dir>/processed/`; unmatched or unparseable files move to `<drop_dir>/failed/` with a Telegram notification.

You can also upload statements from the web UI: the navbar's upload button opens a drop-modal that sends the PDF into the same drop folder (results appear in the modal, on Telegram, and on the doctor page). Requires paisa-agent running with `statements.drop_dir` configured.

**Credit card statement truth:** For `kind: credit_card` accounts, bill facts (statement period, due date, total/min due, paid date/amount) are tracked in a small truth store instead of being re-derived each time. Forwarded statement and payment notice SMSes update these facts at *SMS authority*; a dropped statement PDF overrides them at *PDF authority* (higher confidence — SMS text is short and lossy). On drop, reconciliation compares the statement's transaction lines against the ledger for that cycle and sends inline-button cards: approve-to-add for transactions missing from the ledger, confirm-to-remove for ledger entries that look like duplicates not present in the statement (only offered when the entry can be uniquely located in the journal, and the journal file is backed up before any edit). This state — one record per card — lives in `bill-truth.json` in `paisa.journal_dir`.

## Status

I use it to track my personal finance. Most of my personal use cases
are covered. Feel free to open an issue if you found a bug or start a
discussion if you have a feature request. If you have any question,
you can ask on [Matrix chat](https://matrix.to/#/#paisa:matrix.org).

## License

This software is licensed under [the AGPL 3 or later license](./COPYING).

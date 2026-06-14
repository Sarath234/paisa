# Paisa

[![Matrix](https://img.shields.io/matrix/paisa%3Amatrix.org?logo=matrix)](https://matrix.to/#/#paisa:matrix.org)

**Paisa** is a Personal finance manager. It builds on
top of the [ledger](https://www.ledger-cli.org/) double entry accounting tool. Checkout
[documentation](https://paisa.fyi) to get started.

# Demo

A demo of the Web UI can be found at [https://demo.paisa.fyi](https://demo.paisa.fyi)

## paisa-agent

A Go sidecar that connects Paisa to Telegram, Gmail, and a local LLM (via Ollama).

**Features:**

- **SMS → Ledger:** Forwards bank transaction SMSes from Telegram, parses them with regex + LLM fallback, and appends double-entry journal entries after user confirmation.
- **Merchant rule learning:** Proposes and saves merchant routing rules when you correct a categorisation; editable via Telegram inline buttons.
- **Finance Q&A:** Answer natural-language questions about your spending, net worth, budgets, and balances directly from Telegram.
- **Statement reconciliation:** Polls Gmail for bank statement emails, parses the PDF, compares transactions against your ledger, and sends a Telegram report highlighting missing or extra entries. Results also surface on the Paisa doctor page (`/more/doctor`).

**Config:** Copy `paisa-agent.yaml` to your preferred location and fill in:
- `paisa.url` and `paisa.journal_dir`
- `telegram.bot_token` and `telegram.chat_id`
- `ollama.url` and `ollama.model` (e.g. `gemma3:4b`)
- `gmail.*` (optional) — OAuth2 credentials for statement reconciliation (see setup below)

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

## Status

I use it to track my personal finance. Most of my personal use cases
are covered. Feel free to open an issue if you found a bug or start a
discussion if you have a feature request. If you have any question,
you can ask on [Matrix chat](https://matrix.to/#/#paisa:matrix.org).

## License

This software is licensed under [the AGPL 3 or later license](./COPYING).

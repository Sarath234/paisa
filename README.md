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
- `gmail.*` (optional) — OAuth2 credentials from Google Cloud Console for statement reconciliation

**Run:** `./paisa-agent --config /path/to/paisa-agent.yaml`

## Status

I use it to track my personal finance. Most of my personal use cases
are covered. Feel free to open an issue if you found a bug or start a
discussion if you have a feature request. If you have any question,
you can ask on [Matrix chat](https://matrix.to/#/#paisa:matrix.org).

## License

This software is licensed under [the AGPL 3 or later license](./COPYING).

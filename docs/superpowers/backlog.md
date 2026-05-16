# Paisa Development Backlog

## Open — Performance Issues

### Issue 1 — Wrong Sync Strategy (Delete + Re-insert Everything)
**Status:** Open  
**Files:** `internal/model/posting/posting.go:117`, `internal/model/price/price.go:35,57`, `internal/model/portfolio/portfolio.go:22`, `internal/model/model.go:61-92`

Sync does DELETE all + re-INSERT all on every sync, even though historical data rarely changes.

- **Prices:** Historical prices are immutable. Fetch only prices newer than the latest date in DB, insert the delta. No delete needed.
- **Postings:** Ledger CLI emits the full journal so a full list is unavoidable. Diff by `TransactionID` and only delete/insert changed transactions instead of rebuilding the entire table. Note: `TransactionID` is per-transaction not per-posting — multiple postings share it, so no single-column unique key exists.
- **Portfolios:** Same as prices — fund holdings for a past date don't change.

---

### Issue 6 — Per-Posting Service Calls Inside Balance Loop
**Status:** Open  
**Files:** `internal/server/assets/balance.go:79-125`, `internal/server/networth.go:54-60`

`computeNetworth` and `ComputeBreakdown` call `service.IsInterest`, `service.IsStockSplit`, `service.IsCapitalGains` inside a loop over every posting, and `service.XIRR` per account group. XIRR is a numerical iteration — expensive per group when groups share overlapping posting sets.

**Fix direction:** Pre-classify postings by type once before the loop. Batch or cache XIRR results keyed by posting set hash.

---

### Issue 9 — Journal Parsed 5× Sequentially Per Save
**Status:** Open — root cause of 72s outlier resolved (iCloud); this is the remaining optimization  
**Branch:** `perf/concurrent-ledger-parse` (PR #2 open — conflict fixed, ready to review)  
**Files:** `internal/ledger/ledger.go:88-107`, `internal/model/model.go:37-69`, `internal/server/editor.go:72-130`

Each `POST /api/editor/save` triggers 5 sequential ledger CLI subprocess calls (~12-15s total):
1. `validateFile()` → `ledger balance <temp-file>` (~50ms)
2. `SyncJournal.ValidateFile()` → `ledger balance <full-journal>` (~2.9s — **redundant**)
3. `SyncJournal.Prices()` → `ledger pricesdb <full-journal>` (~2.9s)
4. `SyncJournal.Parse()` regular → `ledger csv <full-journal>` (~2.9s)
5. `SyncJournal.Parse()` budget → `ledger csv <full-journal> --budget` (~2.9s)

**Fix:** Remove step 2 (redundant) + run steps 3–5 concurrently → ~3-4s saves.

---

## Open — Feature Enhancements

### Enhancement 3 — Spending Anomaly Alerts (Webhook/Email)
**Priority:** 7/10 | Complexity: Medium | Impact: High

Background job compares actual vs budgeted spend per category. When any category exceeds a configurable threshold %, emit a webhook POST or send email.

**Files:** `internal/config/config.go` (add `AlertRules`), `internal/server/alerts.go` (new), `src/routes/(app)/more/config/+page.svelte`

---

### Enhancement 4 — Portfolio Rebalancing Calculator
**Priority:** 6/10 | Complexity: Low | Impact: Medium

Given current and target allocation, compute exact buy/sell amounts to reach target within a user-specified deposit/withdrawal. Render as a checklist of trades.

**Files:** `internal/server/allocation.go` (add rebalancing math), `src/routes/(app)/assets/allocation/+page.svelte` (add "Rebalance" panel)

---

### Enhancement 5 — Transaction Document Attachments
**Priority:** 4/10 | Complexity: High | Impact: Medium

Attach receipts/PDFs to transactions via ledger comment directives (`; attachment: receipts/file.pdf`). Editor shows paperclip icon; Paisa serves the file.

**Files:** `internal/ledger/ledger.go` (parse `; attachment:` directives), `internal/server/editor.go` (add `GET /api/attachment/:path`), `src/routes/(app)/ledger/editor/[slug]/+page.svelte`

---

## Closed / Done

| # | Item | Notes |
|---|------|-------|
| Perf 2 | Dashboard 13+ independent DB queries | `ed3c56a` |
| Perf 3 | `computeCashFlow` fetches same data 7× | `ed3c56a` |
| Perf 4 | Sequential blocking HTTP calls for commodity sync | `c50a7bc` + `657edd6` |
| Perf 5 | O(n²) account balance computation | `c50a7bc` |
| Perf 7 | N+1 queries inside price cache loading | `c50a7bc` |
| Perf 8 | Price cache loads ALL prices+postings on startup | `ed3c56a` |
| Perf 10 | Day-by-day loop with per-commodity price lookups | `c50a7bc` |
| Perf 11 | Editor save slowness (iCloud eviction) | User pinned folder offline; PR #4 pruned backups |
| Enh 1 | Smart Import Rules Engine | Built (spec + plan + code), PR #3 closed — not in master |
| Enh 2 | Cash Flow Forecasting | Built (spec + plan + code), PR #5/#6 closed — not in master |

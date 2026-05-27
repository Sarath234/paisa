# Net Worth Projection — Design Spec (Enh 17)

## Goal

Add a dedicated Projection page under Assets that shows a forward-looking net worth and net investment chart, powered by current net worth, 12-month average monthly savings, and a user-adjustable annual return rate.

## Architecture

**Backend change:** Add one field — `monthlySavings` — to the existing `GET /api/networth` response. Computed as the 12-month average of `(income − expenses)` from postings. No new endpoint.

**Frontend:** New page `src/routes/(app)/assets/projection/+page.svelte` + D3 renderer `src/lib/projection.ts`. All projection math runs in the browser from three inputs; slider changes rerender the chart with no network call.

**Navbar:** Add "Projection" entry to the Assets sub-nav, after "Net Worth".

## Data Flow

```
GET /api/networth
  → networthTimeline   (existing — last point = current net worth)
  → monthlySavings     (new field — 12-month avg of income − expenses)

Frontend computes projection points:
  W[0] = current net worth (last timeline point)
  I[0] = current net investment (investmentAmount − withdrawalAmount)
  For each month n = 1..horizon:
    W[n] = W[n-1] × (1 + r/12) + monthlySavings
    I[n] = I[n-1] + monthlySavings
  where r = annualReturnRate / 100
```

## Backend: `monthlySavings` computation

In `GetNetworth` (`internal/server/networth.go`):
- Query Income and Expenses postings for the last 12 complete months (exclude current partial month).
- For each month: `savings = sum(income postings negated) − sum(expense postings)`.
- `monthlySavings = average of the 12 monthly values` (decimal, rounded to 2dp).
- Income postings have negative Amount in Paisa's ledger; negate them to get positive income.

## Page Layout

### Stats row (3 tiles)
| Tile | Value | Note |
|------|-------|------|
| Current Net Worth | last timeline point | formatted currency |
| Monthly Savings | monthlySavings | labelled "12-month avg" |
| Projected at horizon | W[horizon] | labelled "at X% annual return" |

### Controls row
- **Horizon toggle** — 4 buttons: 1Y / 3Y / 5Y / 10Y. Default: 5Y.
- **Return rate slider** — range 6–18%, step 1%, default 12%. Shows selected % next to slider.

### Chart
- D3 line chart. X axis: today → selected horizon (monthly ticks). Y axis: formatted currency.
- **Projected Net Worth** — solid line.
- **Projected Net Investment** — dashed line (stroke-dasharray).
- **Shaded fill** between the two lines (market gain area), low opacity.
- Hover tooltip: month label + both values.
- Zero state: if `monthlySavings ≤ 0` and `currentNetworth = 0`, show "Add some transactions first."

## Out of Scope (deferred to backlog)
- Multiple scenario comparison (Enh 16 — What-If Scenario Planner)
- Inflation adjustment
- Goal / milestone marker lines on chart
- Per-asset-class return rates
- Data science / ML-based savings prediction

## Files Changed

| Action | Path |
|--------|------|
| Modify | `internal/server/networth.go` — add `monthlySavings` to `GetNetworth` |
| Modify | `internal/server/networth_test.go` — test `monthlySavings` computation |
| Modify | `src/lib/utils.ts` — add `monthlySavings` to `ajax("/api/networth")` return type |
| Create | `src/lib/projection.ts` — D3 renderer |
| Create | `src/routes/(app)/assets/projection/+page.svelte` — page |
| Modify | `src/lib/components/Navbar.svelte` — add Projection nav entry |

# Cash Flow Forecasting — Design Spec

**Goal:** Extend the existing Networth and Monthly Expenses charts with a 6-month forward projection, driven by recurring transactions and budget allocations, without adding new pages or navigation.

**Architecture:** Two independent chart extensions. Each backend handler gets a `forecast` field appended to its existing response. Each frontend render function gets a new branch to draw forecast data with a distinct visual treatment.

**Tech Stack:** Go (backend), D3.js (charts), Svelte (pages), existing ledger `forecast=true` postings, existing `ComputeRecurringTransactions`.

---

## Extension 1 — Networth Chart

### Backend: `internal/server/networth.go`

`GetNetworth` currently returns `{ networth: []Networth }`. Extend to:

```json
{
  "networth": [...existing points...],
  "forecast": [...6 monthly forecast points...]
}
```

**Forecast computation (`computeNetworthForecast`):**

1. Take today's networth value as the starting balance.
2. Call `ComputeRecurringTransactions(allPostings)` to get `[]TransactionSequence`.
3. For each of the next 6 calendar months:
   - Sum `futureSchedules` amounts where `scheduled` falls in that month.
   - Classify each schedule as income (positive delta) or expense (negative delta) based on the transaction's account prefix (`Income:*` → positive, `Expenses:*`/`Liabilities:*` → negative).
   - `monthNetworth = prevNetworth + incomeSum - expenseSum`
   - Emit a `Networth` struct with `Date = last day of that month` and all amount fields set to represent the projected value.
4. Return 6 points.

**`Networth` struct** (existing, no change):
```go
type Networth struct {
    Date             time.Time       `json:"date"`
    InvestmentAmount decimal.Decimal `json:"investmentAmount"`
    GainAmount       decimal.Decimal `json:"gainAmount"`
    WithdrawalAmount decimal.Decimal `json:"withdrawalAmount"`
}
```

For forecast points: set `InvestmentAmount = projectedBalance`, `GainAmount = 0`, `WithdrawalAmount = 0`. The `networth()` helper function (`investmentAmount + gainAmount - withdrawalAmount`) will return the correct projected value.

### Frontend: `src/lib/networth.ts`

`renderNetworth(points, element)` signature extended to:

```ts
renderNetworth(points: Networth[], forecastPoints: Networth[], element: Element)
```

**Visual treatment (Option B — muted + shaded band):**

1. Extend the x-axis domain from `now()` to `now() + 6 months`.
2. Draw the existing solid line unchanged up to today.
3. Append a vertical dashed "today" marker line at `x(now())`.
4. Draw an SVG `<path>` area band between upper/lower bounds of the forecast (±10% of the last actual networth value) as a translucent fill (`opacity: 0.08`, same color as networth line).
5. Draw the forecast line as a `<polyline>` with `stroke-dasharray="6,4"` and `opacity: 0.5` using the same `COLORS.primary`.
6. Forecast data points rendered as smaller circles (`r=2`) at reduced opacity.
7. Tooltip on forecast points shows: "Projected · [Month YYYY] · ₹X"

### Frontend: `src/routes/(app)/assets/networth/+page.svelte`

Pass `forecast` from the API response as the second argument to `renderNetworth`.

---

## Extension 2 — Monthly Expenses Timeline

### Backend: `internal/server/expense.go`

`GetExpense` currently returns `{ expenses: []Posting }`. Extend to include synthetic forecast postings for the next 6 months appended to the same array (with `Forecast: true`).

**Forecast computation (`computeExpenseForecast`):**

1. Collect all `Expenses:*` accounts from existing postings.
2. For each account, determine the monthly amount:
   - **Budget source:** query `forecast=true` postings for that account. If entries exist for any future month, use those amounts directly.
   - **Historical fallback:** if no budget entries exist for an account, compute the average actual spend over the last 3 completed calendar months. The source is recorded via the `Note` field (see below).
3. For each of the next 6 months × each account with a non-zero projected amount:
   - Emit a synthetic `Posting` with:
     - `Date = 15th of that month` (mid-month sentinel)
     - `Account = the expense account`
     - `Amount = monthly projected amount`
     - `Forecast = true`
     - `Commodity = "INR"`
     - `Note = "budget"` or `"historical"` (source label for tooltip)
4. Append all synthetic postings to the existing `[]Posting` slice before returning.

**Source label storage:** Use the existing `Note` field on `Posting` (`json:"note"`) to store `"budget"` or `"historical"`. The field is already present and unused for synthetic postings.

### Frontend: `src/lib/expense/monthly.ts`

`renderMonthlyExpensesTimeline` already receives all postings (including future ones once appended). Changes:

1. When grouping postings by month (`_.groupBy(postings, p => p.date.format(timeFormat))`), future months naturally fall into their own buckets — no grouping logic change needed.
2. When rendering bars, check if any posting in that month bucket has `Forecast: true`. If yes:
   - Add an SVG `<defs><pattern>` hatch fill per category color.
   - Render bars with `fill="url(#hatch-{color})"` + `stroke` of the same category color + `stroke-dasharray="3,2"`.
3. Tooltip on forecast bars shows: "Projected · [Account] · ₹X · [budget / est. from 3mo avg]"

**Budget nudge:** Below the chart, if any forecast postings have `note = "historical"`, render a dismissible info banner:

> "N expense categories are estimated from your 3-month average. [Set budgets →] to improve forecast accuracy."

The "Set budgets →" link navigates to `/expense/budget`.

### Frontend: `src/routes/(app)/expense/monthly/+page.svelte`

- Pass combined postings (actuals + forecast) to `renderMonthlyExpensesTimeline` — no change needed since backend appends them.
- Add the nudge banner below the chart SVG, conditionally rendered when any `Posting.Forecast && Posting.Note === "historical"` exists in the response.

---

## Data Flow Summary

```
GET /api/networth
  └── { networth: []Networth, forecast: []Networth (6 months) }
        ↓
  renderNetworth(networth, forecast, element)
    ├── solid line: historical points
    └── dashed muted line + shaded band: forecast points

GET /api/expense
  └── { expenses: []Posting }  ← includes Forecast=true postings for next 6 months
        ↓
  renderMonthlyExpensesTimeline(postings, ...)
    ├── solid bars: actual months (Forecast=false)
    ├── hatched bars: future months (Forecast=true)
    └── nudge banner: if any Posting.Note === "historical"
```

---

## Scope Boundaries

- **In scope:** Networth chart extension, Monthly expenses chart extension, budget nudge banner.
- **Out of scope:** New pages or routes, changes to the recurring page, income forecast, investment projection, alerts/notifications.
- **Not changed:** `/api/networth` and `/api/expense` endpoint paths — response shape is additive only.

# Spending Insights Feed — Design Spec

## Goal

Auto-generate plain-English monthly observations from existing transaction data and surface them in two places: a summary (up to 5 cards) on the dashboard, and a full feed on a dedicated `/insights` page.

## Observation Types

Five types, all computed for the current calendar month:

| Type | Example body |
|---|---|
| `spend_category` | "Food spending ₹12,400 this month — 23% more than last month" |
| `savings_rate` | "Savings rate 34% this month — 6 pp above your 12-month average of 28%" |
| `budget` | "Entertainment — ₹3,200 under budget this month (15% variance)" |
| `top_category` | "Dining is your #1 expense this month at 38% of total spend" |
| `income` | "Income ₹85,000 this month — up 12% vs last month" |

## Data Model

```go
type Insight struct {
    Type     string  `json:"type"`      // see types above
    Title    string  `json:"title"`     // short label, e.g. "Food Spending"
    Body     string  `json:"body"`      // full plain-English sentence
    DeltaPct float64 `json:"delta_pct"` // signed %; pp for savings_rate; 0 for top_category
    Positive bool    `json:"positive"`  // true = good direction (green), false = bad (red)
    Suppress bool    `json:"suppress"`  // true if change is below significance threshold
}
```

`positive` polarity per type:
- `spend_category`: down = positive, up = negative
- `savings_rate`: above average = positive
- `income`: up = positive
- `budget`: under budget = positive
- `top_category`: always `true` (neutral, displayed in primary color)

## Significance Thresholds (suppress if below)

| Type | Threshold |
|---|---|
| `spend_category` | \|Δ%\| < 10% |
| `savings_rate` | \|Δ pp\| < 5 pp vs 12-month average |
| `budget` | \|variance\| < 5% of budget amount |
| `top_category` | never suppressed |
| `income` | \|Δ%\| < 2% |

## API

`GET /api/insights` → `[]Insight`

- All observations sorted by `|delta_pct|` descending (most significant first).
- Suppressed items included in response (frontend decides whether to show them).
- "Current month" = calendar month of server's today. "Previous month" = one month prior.

## Backend — `internal/server/insights.go`

Single function `GetInsights(db *gorm.DB) gin.H` reads all relevant postings once and computes all five types:

**`spend_category`**: Group `Expenses:%` postings (excluding `Expenses:Tax`) by top-level sub-account for current and previous month. Emit one insight per category that existed in either month. Delta% = (current − prev) / prev × 100. If prev = 0, mark as new spend, suppress = false.

**`savings_rate`**: For each of the last 13 calendar months, compute `(income − expense) / income × 100`. Current month = index 0. 12-month average = mean of indices 1–12 (exclude current). Delta = current − average (in percentage points).

**`budget`**: Read `config.GetConfig().Budget` for the configured accounts and amounts. For each configured budget account, sum `Expenses:<account>` postings for the current month as `actual`. `variance% = (budgeted − actual) / budgeted × 100`. Emit one insight per configured account.

**`top_category`**: From the current-month category totals computed during `spend_category`, pick the category with the highest spend. Body includes its % of total month spend. `DeltaPct = 0`.

**`income`**: Sum `Income:%` postings for current and previous month. Delta% = (current − prev) / prev × 100.

## Frontend

### `src/lib/components/InsightCard.svelte`

Reusable card. Props: `insight: Insight`. Shows:
- Icon by type (💰 spend, 📈 savings rate, 🏷 budget, 🏆 top category, 💼 income)
- `title` in bold
- `body` as main text
- Colored delta badge: `+23%` green if `positive`, red if not; no badge for `top_category`

Styled as a Bulma box with `has-text-success` / `has-text-danger` for the badge.

### `src/routes/(app)/insights/+page.svelte`

Calls `GET /api/insights`. Shows all non-suppressed insights, sorted by `|delta_pct|` descending, rendered as `InsightCard` in a responsive grid. Zero state ("No insights yet — add some transactions first") when response is empty.

### Dashboard (`src/routes/(app)/+page.svelte`)

Adds a second `ajax("/api/insights")` call in `onMount`. Filters: `insights.filter(i => !i.suppress)`, then `_.uniqBy(..., i => i.type)` → up to 5 cards. Renders in the existing MasonryGrid.

### Sidebar (`src/routes/(app)/+layout.svelte`)

New "Insights" nav link pointing to `/insights`, using the lightbulb icon (`fa-lightbulb`), placed after the existing dashboard link.

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/server/insights.go` | Create | All insight computation, `GetInsights` handler |
| `internal/server/server.go` | Modify | Register `GET /api/insights` route |
| `src/lib/utils.ts` | Modify | Add `Insight` interface and `/api/insights` ajax overload |
| `src/lib/components/InsightCard.svelte` | Create | Single insight card component |
| `src/routes/(app)/insights/+page.svelte` | Create | Full insights feed page |
| `src/routes/(app)/+page.svelte` | Modify | Add insights section to dashboard |
| `src/routes/(app)/+layout.svelte` | Modify | Add Insights sidebar link |

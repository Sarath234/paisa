# Spending Insights Feed Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-generate five types of plain-English monthly observations from transaction data and surface them on the dashboard (up to 5 cards) and a dedicated `/insights` page.

**Architecture:** A new `internal/server/insights.go` reads postings once and emits up to N+1 `Insight` structs (one per category for `spend_category` and `budget`, one each for the other three types). A single `GET /api/insights` endpoint returns the slice sorted by `|delta_pct|` descending. The frontend renders them via a reusable `InsightCard.svelte` in both a full `/insights` page and as a MasonryGrid section on the dashboard.

**Tech Stack:** Go + GORM + Gin (backend), SvelteKit + TypeScript + Bulma CSS (frontend), `shopspring/decimal`, `samber/lo`, `github.com/stretchr/testify`.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/server/insights.go` | Create | All insight computation + HTTP handler |
| `internal/server/insights_test.go` | Create | Unit tests for compute functions |
| `internal/server/server.go` | Modify | Register `GET /api/insights` route |
| `src/lib/utils.ts` | Modify | Add `Insight` interface + ajax overload |
| `src/lib/components/InsightCard.svelte` | Create | Reusable insight card component |
| `src/routes/(app)/insights/+page.svelte` | Create | Full insights feed page |
| `src/routes/(app)/+page.svelte` | Modify | Insights section in dashboard MasonryGrid |
| `src/lib/components/Navbar.svelte` | Modify | "Insights" nav link after Dashboard |

---

### Task 1: Backend — insight types and GetInsights handler

**Files:**
- Create: `internal/server/insights.go`
- Create: `internal/server/insights_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/server/insights_test.go
package server

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func d(s string) decimal.Decimal {
	v, _ := decimal.NewFromString(s)
	return v
}

func p(account string, date time.Time, amount string) posting.Posting {
	return posting.Posting{Account: account, Date: date, Amount: d(amount), Commodity: "INR"}
}

// ---- spend_category ----

func TestComputeSpendCategory_UpIsNegative(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	prev := now.AddDate(0, -1, 0)
	postings := []posting.Posting{
		p("Expenses:Food", now, "12400"),
		p("Expenses:Food", prev, "10000"),
	}
	insights := computeSpendCategory(postings, now)
	require := assert.New(t)
	require.Len(insights, 1)
	require.Equal("Food", insights[0].Title)
	require.InDelta(24.0, insights[0].DeltaPct, 0.1)
	require.False(insights[0].Positive)    // up = bad
	require.False(insights[0].Suppress)   // |Δ%| >= 10%
}

func TestComputeSpendCategory_DownIsPositive(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	prev := now.AddDate(0, -1, 0)
	postings := []posting.Posting{
		p("Expenses:Food", now, "8000"),
		p("Expenses:Food", prev, "10000"),
	}
	insights := computeSpendCategory(postings, now)
	require := assert.New(t)
	require.Len(insights, 1)
	require.InDelta(-20.0, insights[0].DeltaPct, 0.1)
	require.True(insights[0].Positive)
	require.False(insights[0].Suppress)
}

func TestComputeSpendCategory_BelowThresholdSuppressed(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	prev := now.AddDate(0, -1, 0)
	postings := []posting.Posting{
		p("Expenses:Food", now, "10500"), // +5% — below 10% threshold
		p("Expenses:Food", prev, "10000"),
	}
	insights := computeSpendCategory(postings, now)
	assert.True(t, insights[0].Suppress)
}

func TestComputeSpendCategory_ExcludesTax(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	postings := []posting.Posting{
		p("Expenses:Tax", now, "50000"),
	}
	insights := computeSpendCategory(postings, now)
	assert.Empty(t, insights)
}

func TestComputeSpendCategory_NoPrev_NotSuppressed(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	postings := []posting.Posting{
		p("Expenses:Food", now, "5000"),
	}
	insights := computeSpendCategory(postings, now)
	assert.Len(t, insights, 1)
	assert.False(t, insights[0].Suppress)
}

// ---- savings_rate ----

func TestComputeSavingsRate_AboveAverageIsPositive(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	// Build 13 months of income + expense postings.
	// Months 1-12 (prev): income 100k, expense 72k → rate 28%
	// Month 0 (current): income 100k, expense 66k → rate 34%
	var postings []posting.Posting
	for i := 1; i <= 12; i++ {
		m := now.AddDate(0, -i, 0)
		postings = append(postings, p("Income:Salary", m, "100000"))
		postings = append(postings, p("Expenses:Rent", m, "72000"))
	}
	postings = append(postings, p("Income:Salary", now, "100000"))
	postings = append(postings, p("Expenses:Rent", now, "66000"))

	insight := computeSavingsRate(postings, now)
	require := assert.New(t)
	require.InDelta(34.0, insight.DeltaPct, 0.5) // current 34% vs 28% avg = +6 pp
	require.True(insight.Positive)
	require.False(insight.Suppress) // |Δpp| >= 5
}

func TestComputeSavingsRate_BelowThresholdSuppressed(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	var postings []posting.Posting
	for i := 0; i <= 12; i++ {
		m := now.AddDate(0, -i, 0)
		postings = append(postings, p("Income:Salary", m, "100000"))
		postings = append(postings, p("Expenses:Rent", m, "72000")) // 28% every month → 0 pp delta
	}
	insight := computeSavingsRate(postings, now)
	assert.True(t, insight.Suppress) // |Δpp| < 5
}

// ---- budget ----

func TestComputeBudget_UnderBudgetIsPositive(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	budgeted := []posting.Posting{p("Expenses:Entertainment", now, "10000")}
	actual := []posting.Posting{p("Expenses:Entertainment", now, "6800")} // 32% under
	insights := computeBudgetInsights(budgeted, actual, now)
	require := assert.New(t)
	require.Len(insights, 1)
	require.Equal("Entertainment", insights[0].Title)
	require.True(insights[0].Positive)
	require.False(insights[0].Suppress) // variance > 5%
}

func TestComputeBudget_OverBudgetIsNegative(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	budgeted := []posting.Posting{p("Expenses:Food", now, "10000")}
	actual := []posting.Posting{p("Expenses:Food", now, "12000")}
	insights := computeBudgetInsights(budgeted, actual, now)
	assert.False(t, insights[0].Positive)
}

func TestComputeBudget_WithinThresholdSuppressed(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	budgeted := []posting.Posting{p("Expenses:Food", now, "10000")}
	actual := []posting.Posting{p("Expenses:Food", now, "9600")} // 4% under → suppressed
	insights := computeBudgetInsights(budgeted, actual, now)
	assert.True(t, insights[0].Suppress)
}

// ---- top_category ----

func TestComputeTopCategory_HighestSpend(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	postings := []posting.Posting{
		p("Expenses:Dining", now, "15000"),
		p("Expenses:Food", now, "5000"),
		p("Expenses:Transport", now, "3000"),
	}
	insight := computeTopCategory(postings, now)
	assert.Equal(t, "Dining", insight.Title)
	assert.True(t, insight.Positive)
	assert.False(t, insight.Suppress)
	assert.Equal(t, 0.0, insight.DeltaPct)
}

// ---- income ----

func TestComputeIncome_UpIsPositive(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	prev := now.AddDate(0, -1, 0)
	postings := []posting.Posting{
		p("Income:Salary", now, "85000"),
		p("Income:Salary", prev, "75893"),
	}
	insight := computeIncome(postings, now)
	require := assert.New(t)
	require.InDelta(12.0, insight.DeltaPct, 0.5)
	require.True(insight.Positive)
	require.False(insight.Suppress) // |Δ%| >= 2%
}

func TestComputeIncome_BelowThresholdSuppressed(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	prev := now.AddDate(0, -1, 0)
	postings := []posting.Posting{
		p("Income:Salary", now, "85000"),
		p("Income:Salary", prev, "84500"), // ~0.6% change
	}
	insight := computeIncome(postings, now)
	assert.True(t, insight.Suppress)
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/sarath.m/workspace/work/paisa/.worktrees/rebalancing-calculator
go test ./internal/server/... -run TestCompute -v 2>&1 | head -30
```

Expected: compilation errors or FAIL — `computeSpendCategory`, `computeSavingsRate`, etc. not defined.

- [ ] **Step 3: Implement `internal/server/insights.go`**

```go
package server

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/query"
	"github.com/ananthakumaran/paisa/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Insight struct {
	Type     string  `json:"type"`
	Title    string  `json:"title"`
	Body     string  `json:"body"`
	DeltaPct float64 `json:"delta_pct"`
	Positive bool    `json:"positive"`
	Suppress bool    `json:"suppress"`
}

func GetInsights(db *gorm.DB) gin.H {
	now := utils.Now()

	expensePostings := query.Init(db).Like("Expenses:%").LastNMonths(13).All()
	incomePostings := query.Init(db).Like("Income:%").LastNMonths(13).All()
	forecastPostings := query.Init(db).Like("Expenses:%").Forecast().UntilThisMonthEnd().All()

	var insights []Insight
	insights = append(insights, computeSpendCategory(expensePostings, now)...)
	insights = append(insights, computeSavingsRate(append(incomePostings, expensePostings...), now))
	insights = append(insights, computeBudgetInsights(forecastPostings, expensePostings, now)...)
	if top := computeTopCategory(expensePostings, now); top.Title != "" {
		insights = append(insights, top)
	}
	insights = append(insights, computeIncome(incomePostings, now))

	// sort by |delta_pct| descending; top_category (DeltaPct==0) always last
	lo.SortBy(insights, func(a, b Insight) bool {
		return math.Abs(a.DeltaPct) > math.Abs(b.DeltaPct)
	})

	return gin.H{"insights": insights}
}

// topLevelCategory returns the sub-account one level below "Expenses:",
// e.g. "Expenses:Food:Groceries" → "Food".
func topLevelCategory(account string) string {
	parts := strings.SplitN(account, ":", 3)
	if len(parts) < 2 {
		return account
	}
	return parts[1]
}

func filterMonth(postings []posting.Posting, year int, month time.Month) []posting.Posting {
	return lo.Filter(postings, func(p posting.Posting, _ int) bool {
		return p.Date.Year() == year && p.Date.Month() == month
	})
}

func sumByAccount(postings []posting.Posting) map[string]decimal.Decimal {
	result := make(map[string]decimal.Decimal)
	for _, p := range postings {
		result[p.Account] = result[p.Account].Add(p.Amount)
	}
	return result
}

// computeSpendCategory emits one Insight per Expenses:* top-level sub-account
// (excluding Expenses:Tax) that existed in current or previous month.
func computeSpendCategory(postings []posting.Posting, now time.Time) []Insight {
	cur := filterMonth(postings, now.Year(), now.Month())
	prev := filterMonth(postings, now.AddDate(0, -1, 0).Year(), now.AddDate(0, -1, 0).Month())

	// exclude Tax
	cur = lo.Filter(cur, func(p posting.Posting, _ int) bool {
		return !utils.IsSameOrParent(p.Account, "Expenses:Tax")
	})
	prev = lo.Filter(prev, func(p posting.Posting, _ int) bool {
		return !utils.IsSameOrParent(p.Account, "Expenses:Tax")
	})

	// aggregate by top-level category
	curByCategory := make(map[string]decimal.Decimal)
	for _, p := range cur {
		cat := topLevelCategory(p.Account)
		curByCategory[cat] = curByCategory[cat].Add(p.Amount)
	}
	prevByCategory := make(map[string]decimal.Decimal)
	for _, p := range prev {
		cat := topLevelCategory(p.Account)
		prevByCategory[cat] = prevByCategory[cat].Add(p.Amount)
	}

	categories := lo.Uniq(append(lo.Keys(curByCategory), lo.Keys(prevByCategory)...))

	var insights []Insight
	for _, cat := range categories {
		curAmt := curByCategory[cat]
		prevAmt := prevByCategory[cat]

		var deltaPct float64
		var suppress bool
		if prevAmt.IsZero() {
			// new spend this month
			deltaPct = 100
			suppress = false
		} else {
			f, _ := curAmt.Sub(prevAmt).Div(prevAmt).Mul(decimal.NewFromInt(100)).Float64()
			deltaPct = f
			suppress = math.Abs(deltaPct) < 10
		}

		curF, _ := curAmt.Float64()
		prevF, _ := prevAmt.Float64()
		var body string
		if prevAmt.IsZero() {
			body = fmt.Sprintf("%s spending ₹%.0f this month — new spend", cat, curF)
		} else {
			dir := "more"
			if deltaPct < 0 {
				dir = "less"
			}
			body = fmt.Sprintf("%s spending ₹%.0f this month — %.0f%% %s than last month", cat, curF, math.Abs(deltaPct), dir)
		}

		insights = append(insights, Insight{
			Type:     "spend_category",
			Title:    cat + " Spending",
			Body:     body,
			DeltaPct: deltaPct,
			Positive: deltaPct <= 0, // down = good
			Suppress: suppress,
		})
		_ = prevF
	}
	return insights
}

// computeSavingsRate computes the current-month savings rate vs the 12-month average.
// DeltaPct holds the delta in percentage points (not a ratio).
func computeSavingsRate(postings []posting.Posting, now time.Time) Insight {
	rateForMonth := func(year int, month time.Month) float64 {
		inc := filterMonth(postings, year, month)
		exp := filterMonth(postings, year, month)

		incTotal := decimal.Zero
		for _, p := range inc {
			if utils.IsParent(p.Account, "Income") || utils.IsSameOrParent(p.Account, "Income") {
				incTotal = incTotal.Add(p.Amount.Neg()) // income postings are negative
			}
		}
		expTotal := decimal.Zero
		for _, p := range exp {
			if utils.IsParent(p.Account, "Expenses") || utils.IsSameOrParent(p.Account, "Expenses") {
				expTotal = expTotal.Add(p.Amount)
			}
		}
		if incTotal.IsZero() {
			return 0
		}
		r, _ := incTotal.Sub(expTotal).Div(incTotal).Mul(decimal.NewFromInt(100)).Float64()
		return r
	}

	// index 0 = current month; 1..12 = previous months
	rates := make([]float64, 13)
	for i := 0; i < 13; i++ {
		m := now.AddDate(0, -i, 0)
		rates[i] = rateForMonth(m.Year(), m.Month())
	}

	current := rates[0]
	sum := 0.0
	for _, r := range rates[1:] {
		sum += r
	}
	avg := sum / 12.0
	deltaPP := current - avg

	var body string
	if deltaPP >= 0 {
		body = fmt.Sprintf("Savings rate %.0f%% this month — %.0f pp above your 12-month average of %.0f%%", current, deltaPP, avg)
	} else {
		body = fmt.Sprintf("Savings rate %.0f%% this month — %.0f pp below your 12-month average of %.0f%%", current, math.Abs(deltaPP), avg)
	}

	return Insight{
		Type:     "savings_rate",
		Title:    "Savings Rate",
		Body:     body,
		DeltaPct: deltaPP,
		Positive: deltaPP >= 0,
		Suppress: math.Abs(deltaPP) < 5,
	}
}

// computeBudgetInsights computes one insight per budgeted top-level Expenses account.
// budgetedPostings = forecast postings; actualPostings = real expense postings.
func computeBudgetInsights(budgetedPostings, actualPostings []posting.Posting, now time.Time) []Insight {
	curBudgeted := filterMonth(budgetedPostings, now.Year(), now.Month())
	curActual := filterMonth(actualPostings, now.Year(), now.Month())

	// group budgeted by top-level category
	budgetByCategory := make(map[string]decimal.Decimal)
	for _, p := range curBudgeted {
		cat := topLevelCategory(p.Account)
		budgetByCategory[cat] = budgetByCategory[cat].Add(p.Amount)
	}

	if len(budgetByCategory) == 0 {
		return nil
	}

	// group actual by top-level category
	actualByCategory := make(map[string]decimal.Decimal)
	for _, p := range curActual {
		cat := topLevelCategory(p.Account)
		actualByCategory[cat] = actualByCategory[cat].Add(p.Amount)
	}

	var insights []Insight
	for cat, budgeted := range budgetByCategory {
		if budgeted.IsZero() {
			continue
		}
		actual := actualByCategory[cat]
		variance := budgeted.Sub(actual)
		variancePct, _ := variance.Div(budgeted).Mul(decimal.NewFromInt(100)).Float64()

		budgetedF, _ := budgeted.Float64()
		actualF, _ := actual.Float64()
		diffF, _ := variance.Abs().Float64()

		var body string
		if variancePct >= 0 {
			body = fmt.Sprintf("%s — ₹%.0f under budget this month (%.0f%% variance)", cat, diffF, math.Abs(variancePct))
		} else {
			body = fmt.Sprintf("%s — ₹%.0f over budget this month (%.0f%% variance)", cat, diffF, math.Abs(variancePct))
		}

		insights = append(insights, Insight{
			Type:     "budget",
			Title:    cat + " Budget",
			Body:     body,
			DeltaPct: variancePct,
			Positive: variancePct >= 0,
			Suppress: math.Abs(variancePct) < 5,
		})
		_, _ = budgetedF, actualF
	}
	return insights
}

// computeTopCategory picks the Expenses:* category with highest spend this month.
func computeTopCategory(postings []posting.Posting, now time.Time) Insight {
	cur := filterMonth(postings, now.Year(), now.Month())
	cur = lo.Filter(cur, func(p posting.Posting, _ int) bool {
		return !utils.IsSameOrParent(p.Account, "Expenses:Tax")
	})

	if len(cur) == 0 {
		return Insight{}
	}

	byCategory := make(map[string]decimal.Decimal)
	total := decimal.Zero
	for _, p := range cur {
		cat := topLevelCategory(p.Account)
		byCategory[cat] = byCategory[cat].Add(p.Amount)
		total = total.Add(p.Amount)
	}

	topCat := ""
	topAmt := decimal.Zero
	for cat, amt := range byCategory {
		if amt.GreaterThan(topAmt) {
			topAmt = amt
			topCat = cat
		}
	}

	pct, _ := topAmt.Div(total).Mul(decimal.NewFromInt(100)).Float64()
	body := fmt.Sprintf("%s is your #1 expense this month at %.0f%% of total spend", topCat, pct)

	return Insight{
		Type:     "top_category",
		Title:    "Top Category",
		Body:     body,
		DeltaPct: 0,
		Positive: true,
		Suppress: false,
	}
}

// computeIncome computes current vs previous month income delta.
func computeIncome(postings []posting.Posting, now time.Time) Insight {
	sum := func(year int, month time.Month) decimal.Decimal {
		total := decimal.Zero
		for _, p := range filterMonth(postings, year, month) {
			total = total.Add(p.Amount.Neg()) // income postings are negative in ledger
		}
		return total
	}

	cur := sum(now.Year(), now.Month())
	prev := sum(now.AddDate(0, -1, 0).Year(), now.AddDate(0, -1, 0).Month())

	var deltaPct float64
	if !prev.IsZero() {
		f, _ := cur.Sub(prev).Div(prev).Mul(decimal.NewFromInt(100)).Float64()
		deltaPct = f
	}

	curF, _ := cur.Float64()
	var body string
	if prev.IsZero() {
		body = fmt.Sprintf("Income ₹%.0f this month", curF)
	} else {
		dir := "up"
		if deltaPct < 0 {
			dir = "down"
		}
		body = fmt.Sprintf("Income ₹%.0f this month — %s %.0f%% vs last month", curF, dir, math.Abs(deltaPct))
	}

	return Insight{
		Type:     "income",
		Title:    "Income",
		Body:     body,
		DeltaPct: deltaPct,
		Positive: deltaPct >= 0,
		Suppress: math.Abs(deltaPct) < 2,
	}
}
```

- [ ] **Step 4: Run the tests and verify they pass**

```bash
cd /Users/sarath.m/workspace/work/paisa/.worktrees/rebalancing-calculator
go test ./internal/server/... -run TestCompute -v 2>&1
```

Expected: all TestCompute* tests PASS.

**Note on income sign:** In Paisa's ledger model, `Income:*` postings have *negative* amounts (credit postings). The code calls `.Neg()` to get the positive income value. If the tests for savings_rate or income fail due to sign issues, check `p()` helper — it passes the string directly, so income test postings should use negative amounts like `p("Income:Salary", m, "-100000")` and remove the `.Neg()` in the compute functions, OR keep `.Neg()` and use positive strings. Verify against `internal/server/investment.go` `GetInvestment` which reads `Income:%` to see the sign convention actually used.

- [ ] **Step 5: Commit**

```bash
git add internal/server/insights.go internal/server/insights_test.go
git commit -m "feat: add spending insights computation (5 types)"
```

---

### Task 2: Register the API route

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Add the route after the `/api/allocation` registration**

In `internal/server/server.go`, find the line:

```go
	router.GET("/api/allocation", func(c *gin.Context) {
		c.JSON(200, GetAllocation(db))
	})
```

Add immediately after it:

```go
	router.GET("/api/insights", func(c *gin.Context) {
		c.JSON(200, GetInsights(db))
	})
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/sarath.m/workspace/work/paisa/.worktrees/rebalancing-calculator
go build ./internal/server/... 2>&1
```

Expected: no errors.

- [ ] **Step 3: Smoke-test via curl (requires running server)**

If a dev server is running on port 7500:
```bash
curl -s http://localhost:7500/api/insights | head -c 200
```

Expected: JSON with `{"insights": [...]}`.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go
git commit -m "feat: register GET /api/insights route"
```

---

### Task 3: TypeScript `Insight` interface and ajax overload

**Files:**
- Modify: `src/lib/utils.ts`

- [ ] **Step 1: Add the `Insight` interface**

In `src/lib/utils.ts`, find the last `export interface` before the `ajax` function declarations (around line 339, after `Forecast`). Add:

```typescript
export interface Insight {
  type: string;
  title: string;
  body: string;
  delta_pct: number;
  positive: boolean;
  suppress: boolean;
}
```

- [ ] **Step 2: Add the ajax overload**

Find the line `export function ajax(route: "/api/portfolio_allocation"): Promise<PortfolioAllocation>;` and add below it:

```typescript
export function ajax(route: "/api/insights"): Promise<{ insights: Insight[] }>;
```

- [ ] **Step 3: TypeScript check**

```bash
cd /Users/sarath.m/workspace/work/paisa/.worktrees/rebalancing-calculator
npx tsc --noEmit 2>&1
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add src/lib/utils.ts
git commit -m "feat: add Insight type and /api/insights ajax overload"
```

---

### Task 4: `InsightCard.svelte` component

**Files:**
- Create: `src/lib/components/InsightCard.svelte`

- [ ] **Step 1: Create the component**

```svelte
<!-- src/lib/components/InsightCard.svelte -->
<script lang="ts">
  import type { Insight } from "$lib/utils";

  export let insight: Insight;

  const icons: Record<string, string> = {
    spend_category: "💰",
    savings_rate: "📈",
    budget: "🏷",
    top_category: "🏆",
    income: "💼"
  };

  $: icon = icons[insight.type] ?? "💡";
  $: showBadge = insight.type !== "top_category";
  $: badgeClass = insight.positive ? "has-text-success" : "has-text-danger";
  $: badgeText =
    insight.delta_pct > 0
      ? `+${Math.abs(insight.delta_pct).toFixed(0)}%`
      : `${insight.delta_pct.toFixed(0)}%`;
</script>

<div class="box p-4">
  <div class="is-flex is-align-items-center mb-2" style="gap: 0.5rem">
    <span style="font-size: 1.25rem">{icon}</span>
    <strong>{insight.title}</strong>
    {#if showBadge}
      <span class="tag {badgeClass} ml-auto">{badgeText}</span>
    {/if}
  </div>
  <p class="is-size-7 has-text-grey-dark">{insight.body}</p>
</div>
```

- [ ] **Step 2: TypeScript check**

```bash
cd /Users/sarath.m/workspace/work/paisa/.worktrees/rebalancing-calculator
npx tsc --noEmit 2>&1
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add src/lib/components/InsightCard.svelte
git commit -m "feat: add InsightCard component"
```

---

### Task 5: `/insights` page

**Files:**
- Create: `src/routes/(app)/insights/+page.svelte`

- [ ] **Step 1: Create the page**

```svelte
<!-- src/routes/(app)/insights/+page.svelte -->
<script lang="ts">
  import InsightCard from "$lib/components/InsightCard.svelte";
  import { ajax, type Insight } from "$lib/utils";
  import _ from "lodash";
  import { onMount } from "svelte";

  let insights: Insight[] = [];
  let loaded = false;

  onMount(async () => {
    ({ insights } = await ajax("/api/insights"));
    insights = _.orderBy(
      insights.filter((i) => !i.suppress),
      (i) => Math.abs(i.delta_pct),
      "desc"
    );
    loaded = true;
  });
</script>

<section class="section">
  <div class="container is-fluid">
    <h1 class="title">Insights</h1>
    {#if loaded && insights.length === 0}
      <p class="has-text-grey">No insights yet — add some transactions first.</p>
    {:else}
      <div class="columns is-multiline">
        {#each insights as insight}
          <div class="column is-4">
            <InsightCard {insight} />
          </div>
        {/each}
      </div>
    {/if}
  </div>
</section>
```

- [ ] **Step 2: TypeScript check**

```bash
cd /Users/sarath.m/workspace/work/paisa/.worktrees/rebalancing-calculator
npx tsc --noEmit 2>&1
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add src/routes/\(app\)/insights/+page.svelte
git commit -m "feat: add /insights feed page"
```

---

### Task 6: Dashboard integration

**Files:**
- Modify: `src/routes/(app)/+page.svelte`

- [ ] **Step 1: Add Insight to the imports from utils**

In `src/routes/(app)/+page.svelte`, find the `import { ajax, ... } from "$lib/utils"` block (lines 8–22). Add `type Insight` to the named imports:

```typescript
  import {
    ajax,
    formatCurrency,
    formatFloat,
    type Budget,
    type CashFlow,
    type Insight,
    type Networth,
    type Posting,
    type Transaction,
    type TransactionSequence,
    type Legend,
    now,
    type GoalSummary,
    type AssetBreakdown
  } from "$lib/utils";
```

- [ ] **Step 2: Add InsightCard import**

After the existing component imports, add:

```typescript
  import InsightCard from "$lib/components/InsightCard.svelte";
```

- [ ] **Step 3: Add the `insights` state variable**

After `let checkingBalances: Record<string, AssetBreakdown> = {};` (around line 53), add:

```typescript
  let insights: Insight[] = [];
```

- [ ] **Step 4: Fetch insights in onMount**

In `onMount`, after the `await ajax("/api/dashboard")` destructuring (after line 143), add:

```typescript
    ({ insights } = await ajax("/api/insights"));
    insights = _.uniqBy(
      _.orderBy(
        insights.filter((i) => !i.suppress),
        (i) => Math.abs(i.delta_pct),
        "desc"
      ),
      (i) => i.type
    ).slice(0, 5);
```

- [ ] **Step 5: Add the insights section to the template**

Find the MasonryGrid block that renders `checkingBalances` (around line 284):

```svelte
                <div class="content">
                  <UntypedMasonryGrid gap={10} maxStretchColumnSize={400} align="stretch">
                    {#each _.values(checkingBalances) as assetBreakdown}
                      <div class="is-flex-grow-1">
                        <BalanceCard {assetBreakdown} />
                      </div>
                    {/each}
                  </UntypedMasonryGrid>
                </div>
```

Add a new section after it (still within the outer tile structure, before the `{/if}` that closes the `{#if !_.isEmpty(checkingBalances)}`):

Actually, add a new `<div class="tile is-parent">` block after the checking balances tile `</div>` end. Find the block ending with:

```svelte
        {/if}
```

And add before it (as a sibling tile block):

```svelte
        {#if insights.length > 0}
          <div class="tile is-parent">
            <article class="tile is-child min-w-0">
              <p class="subtitle">
                <a class="secondary-link has-text-grey" href="/insights">Insights</a>
              </p>
              <div class="content">
                <UntypedMasonryGrid gap={10} maxStretchColumnSize={400} align="stretch">
                  {#each insights as insight}
                    <div class="is-flex-grow-1">
                      <InsightCard {insight} />
                    </div>
                  {/each}
                </UntypedMasonryGrid>
              </div>
            </article>
          </div>
        {/if}
```

- [ ] **Step 6: TypeScript check**

```bash
cd /Users/sarath.m/workspace/work/paisa/.worktrees/rebalancing-calculator
npx tsc --noEmit 2>&1
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add src/routes/\(app\)/+page.svelte
git commit -m "feat: add insights section to dashboard"
```

---

### Task 7: Navbar link

**Files:**
- Modify: `src/lib/components/Navbar.svelte`

- [ ] **Step 1: Add the Insights link**

In `src/lib/components/Navbar.svelte`, find the links array (line 54):

```typescript
  const links: Link[] = [
    { label: "Dashboard", href: "/", hide: true },
    {
      label: "Cash Flow",
```

Add the Insights link after Dashboard:

```typescript
  const links: Link[] = [
    { label: "Dashboard", href: "/", hide: true },
    { label: "Insights", href: "/insights" },
    {
      label: "Cash Flow",
```

- [ ] **Step 2: TypeScript check**

```bash
cd /Users/sarath.m/workspace/work/paisa/.worktrees/rebalancing-calculator
npx tsc --noEmit 2>&1
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add src/lib/components/Navbar.svelte
git commit -m "feat: add Insights nav link"
```

---

### Task 8: Format and full build check

**Files:** All modified frontend files.

- [ ] **Step 1: Run Prettier**

```bash
cd /Users/sarath.m/workspace/work/paisa/.worktrees/rebalancing-calculator
npx prettier --write src/lib/utils.ts src/lib/components/InsightCard.svelte "src/routes/(app)/insights/+page.svelte" "src/routes/(app)/+page.svelte" src/lib/components/Navbar.svelte 2>&1
```

- [ ] **Step 2: Run TypeScript check**

```bash
npx tsc --noEmit 2>&1
```

Expected: no errors.

- [ ] **Step 3: Run Go tests**

```bash
go test ./internal/... 2>&1
```

Expected: all tests PASS.

- [ ] **Step 4: Commit any formatting changes**

```bash
git add -p  # stage only formatting changes, nothing structural
git commit -m "style: prettier format insights files"
```

(Skip if Prettier made no changes.)

---

## Post-implementation notes

**Income sign convention:** Paisa stores `Income:*` postings with negative `Amount` values (they credit the equity). The `computeSavingsRate` and `computeIncome` functions call `.Neg()` to get the positive income value. Verify this matches actual data by running `go test ./internal/server/... -run TestCompute -v` after implementation. If `rateForMonth` returns 0 or negative savings rates for healthy data, the sign convention may differ — check `internal/server/investment.go`'s `GetInvestment` function which computes similar income totals.

**Budget with no forecast postings:** If the user has no `.Forecast()` postings in their ledger (most users won't), `computeBudgetInsights` returns `nil` and no budget insights are emitted — this is correct behaviour.

**Significance threshold for `top_category`:** Per spec, `top_category` is never suppressed. The `computeTopCategory` function always returns `Suppress: false`.

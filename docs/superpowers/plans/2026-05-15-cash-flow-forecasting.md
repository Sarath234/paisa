# Cash Flow Forecasting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the Networth and Monthly Expenses charts with 6-month forward projections — a dashed muted line continuation on Networth and hatched forecast bars on Monthly Expenses, driven by recurring transactions and budget/historical data.

**Architecture:** Two independent backend extensions (new helper functions, additive response fields) and two frontend render function extensions. No new routes, pages, or navigation. Backend is Go + Gin; frontend is D3 + Svelte + TypeScript with bun tests.

**Tech Stack:** Go, D3.js, Svelte, TypeScript, `shopspring/decimal`, `samber/lo`, bun (frontend tests), `stretchr/testify` (Go tests).

---

## File Structure

**Created:**
- `internal/server/networth_test.go` — Go tests for `computeNetworthForecast`
- `internal/server/expense_forecast_test.go` — Go tests for `computeExpenseForecast`

**Modified:**
- `internal/server/networth.go` — add `computeNetworthForecast`, extend `GetNetworth` response
- `internal/server/expense.go` — add `computeExpenseForecast`, add `forecast_expenses` to `GetExpense`
- `src/lib/utils.ts` — add `forecast?: boolean` to `Posting` interface
- `src/lib/networth.ts` — extend `renderNetworth` to accept + render `forecastPoints`
- `src/lib/expense/monthly.ts` — detect forecast months, render hatched bars with SVG patterns
- `src/routes/(app)/assets/networth/+page.svelte` — pass `result.forecast` to `renderNetworth`
- `src/routes/(app)/expense/monthly/+page.svelte` — add `forecast_expenses` destructuring, concatenate for timeline, add nudge banner

---

## Task 1: Add `forecast` field to TypeScript Posting interface

**Files:**
- Modify: `src/lib/utils.ts`

The Go `Posting` model has `Forecast bool \`json:"forecast"\`` (line 41 of `internal/model/posting/posting.go`). The TypeScript interface is missing this field. It must exist before the frontend can read it from API responses.

- [ ] **Step 1: Open the file and find the Posting interface**

The `Posting` interface starts around line 1180 of `src/lib/utils.ts`. It currently ends with `balance: number;` and has no `forecast` field.

- [ ] **Step 2: Add `forecast` to the interface**

Find this block in `src/lib/utils.ts`:

```ts
export interface Posting {
  id: string;
  date: dayjs.Dayjs;
  payee: string;
  account: string;
  commodity: string;
  quantity: number;
  amount: number;
  status: string;
  tag_recurring: string;
  transaction_begin_line: number;
  transaction_end_line: number;
  file_name: string;
  note: string;
  transaction_note: string;

  market_amount: number;
  balance: number;
}
```

Add `forecast: boolean;` after `balance: number;`:

```ts
export interface Posting {
  id: string;
  date: dayjs.Dayjs;
  payee: string;
  account: string;
  commodity: string;
  quantity: number;
  amount: number;
  status: string;
  tag_recurring: string;
  transaction_begin_line: number;
  transaction_end_line: number;
  file_name: string;
  note: string;
  transaction_note: string;

  market_amount: number;
  balance: number;
  forecast: boolean;
}
```

- [ ] **Step 3: Run the frontend type check**

```bash
cd /Users/sarath.m/workspace/work/paisa && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors about `forecast` on `Posting`.

- [ ] **Step 4: Commit**

```bash
git add src/lib/utils.ts
git commit -m "feat: add forecast field to Posting TypeScript interface"
```

---

## Task 2: Backend — `computeNetworthForecast`

**Files:**
- Modify: `internal/server/networth.go`
- Create: `internal/server/networth_test.go`

`computeNetworthForecast` projects the current networth forward 6 months by summing the net monthly cash flow from recurring transactions. It lives in `networth.go` alongside `computeNetworthTimeline`. It calls `ComputeRecurringTransactions` (same `server` package, defined in `recurring.go`) — no import needed.

**Sign conventions in ledger double-entry:**
- Salary posting: `Income:Salary -100000`, `Assets:Checking +100000`
- Expense posting: `Expenses:Food +5000`, `Assets:Checking -5000`
- Net impact on checking: sum of positive-amount postings in `Assets:*` = salary inflow; sum of positive-amount postings in `Expenses:*` = outflow

**Algorithm:**
- For each recurring `TransactionSequence`, look at its most recent transaction's postings.
- Sum positive posting amounts for `Expenses:*` → monthly outflow (subtract from delta).
- Sum positive posting amounts for `Income:*` accounts: income postings are negative (credit side), so the positive posting is `Assets:Checking`. To identify income sequences, check if any posting has `Income:` prefix, then sum positive posting amounts as the income amount.
- Multiply by `30.0 / seq.Interval` to get per-month occurrences.
- Starting from today's networth value, accumulate `balance += monthlyDelta` for each of 6 future months.
- Emit a `Networth` struct per month: `InvestmentAmount = balance`, `GainAmount = 0`, `WithdrawalAmount = 0` (so the `networth()` helper `investmentAmount + gainAmount - withdrawalAmount` returns the projected balance).

- [ ] **Step 1: Write the failing test**

Create `internal/server/networth_test.go`:

```go
package server

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestComputeNetworthForecast_ReturnsSixPoints(t *testing.T) {
	result := computeNetworthForecast([]posting.Posting{}, decimal.Zero)
	assert.Len(t, result, 6)
}

func TestComputeNetworthForecast_FlatBalanceWithNoRecurring(t *testing.T) {
	// With no postings that form valid recurring sequences, monthly delta = 0.
	// ComputeRecurringTransactions requires len(transactions) > 1 per sequence,
	// so lone test postings produce no sequences and balance stays flat.
	start := decimal.NewFromFloat(500000)
	result := computeNetworthForecast([]posting.Posting{}, start)
	for _, r := range result {
		assert.True(t, r.InvestmentAmount.Equal(start),
			"no recurring sequences → balance stays flat at %v", start)
	}
}

func TestComputeNetworthForecast_DatesAreFutureEndOfMonth(t *testing.T) {
	result := computeNetworthForecast([]posting.Posting{}, decimal.Zero)
	now := time.Now()
	for i, r := range result {
		assert.True(t, r.Date.After(now),
			"point %d: date %v should be after now %v", i, r.Date, now)
		// End-of-month: the next day should be in a different month
		nextDay := r.Date.AddDate(0, 0, 1)
		assert.NotEqual(t, r.Date.Month(), nextDay.Month(),
			"point %d: date %v should be end of month", i, r.Date)
	}
}

func TestComputeNetworthForecast_MonotonicDates(t *testing.T) {
	result := computeNetworthForecast([]posting.Posting{}, decimal.Zero)
	for i := 1; i < len(result); i++ {
		assert.True(t, result[i].Date.After(result[i-1].Date),
			"dates should be monotonically increasing")
	}
}

func TestComputeNetworthForecast_OnlyInvestmentAmountSet(t *testing.T) {
	// Forecast points use InvestmentAmount as the projected balance;
	// GainAmount and WithdrawalAmount must be zero so networth() = InvestmentAmount.
	start := decimal.NewFromFloat(100000)
	result := computeNetworthForecast([]posting.Posting{}, start)
	for _, r := range result {
		assert.True(t, r.GainAmount.IsZero())
		assert.True(t, r.WithdrawalAmount.IsZero())
	}
}

// Ensure the posting import is used (posting.Posting is referenced in computeNetworthForecast's signature)
var _ posting.Posting
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/server/... -run TestComputeNetworthForecast 2>&1
```

Expected: FAIL with "undefined: computeNetworthForecast"

- [ ] **Step 3: Implement `computeNetworthForecast` in `internal/server/networth.go`**

Add these imports to `networth.go` (it already imports `strings`, `time`, `posting`, `utils`, `decimal`):

```go
import (
    "sort"
    "strings"
    "time"

    "github.com/ananthakumaran/paisa/internal/config"
    "github.com/ananthakumaran/paisa/internal/model/posting"
    "github.com/ananthakumaran/paisa/internal/model/price"
    "github.com/ananthakumaran/paisa/internal/query"
    "github.com/ananthakumaran/paisa/internal/service"
    "github.com/ananthakumaran/paisa/internal/utils"
    "github.com/gin-gonic/gin"
    "github.com/shopspring/decimal"
    "gorm.io/gorm"
)
```

Add the function at the bottom of `internal/server/networth.go`:

```go
func computeNetworthForecast(allPostings []posting.Posting, lastNetworth decimal.Decimal) []Networth {
	sequences := ComputeRecurringTransactions(allPostings)

	var monthlyDelta decimal.Decimal
	for _, seq := range sequences {
		if len(seq.Transactions) == 0 || seq.Interval == 0 {
			continue
		}
		lastTx := seq.Transactions[0]

		var hasIncome, hasExpense bool
		var txAmount decimal.Decimal
		for _, p := range lastTx.Postings {
			if strings.HasPrefix(p.Account, "Income:") {
				hasIncome = true
			}
			if strings.HasPrefix(p.Account, "Expenses:") {
				hasExpense = true
			}
			if p.Amount.GreaterThan(decimal.Zero) {
				txAmount = txAmount.Add(p.Amount)
			}
		}

		occurrences := decimal.NewFromFloat(30.0 / float64(seq.Interval))
		if hasIncome {
			monthlyDelta = monthlyDelta.Add(txAmount.Mul(occurrences))
		} else if hasExpense {
			monthlyDelta = monthlyDelta.Sub(txAmount.Mul(occurrences))
		}
	}

	now := utils.Now()
	forecast := make([]Networth, 6)
	balance := lastNetworth
	for i := 0; i < 6; i++ {
		balance = balance.Add(monthlyDelta)
		monthEnd := utils.EndOfMonth(now.AddDate(0, i+1, 0))
		forecast[i] = Networth{
			Date:             monthEnd,
			InvestmentAmount: balance,
		}
	}
	return forecast
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/server/... -run TestComputeNetworthForecast -v 2>&1
```

Expected: PASS — 3 tests passing.

- [ ] **Step 5: Commit**

```bash
git add internal/server/networth.go internal/server/networth_test.go
git commit -m "feat: add computeNetworthForecast to project networth 6 months forward"
```

---

## Task 3: Backend — extend `GetNetworth` response

**Files:**
- Modify: `internal/server/networth.go`
- Modify: `internal/server/server.go` (no change needed — same endpoint path)

`GetNetworth` currently returns `{ networthTimeline: []Networth, xirr: float }`. Add `forecast: []Networth` computed from the last networth point and all postings.

- [ ] **Step 1: Update `GetNetworth` in `internal/server/networth.go`**

Find the existing function:

```go
func GetNetworth(db *gorm.DB) gin.H {
	postings := query.Init(db).Like("Assets:%", "Income:CapitalGains:%", "Liabilities:%").UntilToday().All()

	postings = service.PopulateMarketPrice(db, postings)
	networthTimeline := computeNetworthTimeline(db, postings, false)
	xirr := service.XIRR(db, postings)
	return gin.H{"networthTimeline": networthTimeline, "xirr": xirr}
}
```

Replace with:

```go
func GetNetworth(db *gorm.DB) gin.H {
	postings := query.Init(db).Like("Assets:%", "Income:CapitalGains:%", "Liabilities:%").UntilToday().All()

	postings = service.PopulateMarketPrice(db, postings)
	networthTimeline := computeNetworthTimeline(db, postings, false)
	xirr := service.XIRR(db, postings)

	var lastNetworth decimal.Decimal
	if len(networthTimeline) > 0 {
		last := networthTimeline[len(networthTimeline)-1]
		lastNetworth = last.InvestmentAmount.Add(last.GainAmount).Sub(last.WithdrawalAmount)
	}
	allPostings := query.Init(db).All()
	forecast := computeNetworthForecast(allPostings, lastNetworth)

	return gin.H{"networthTimeline": networthTimeline, "xirr": xirr, "forecast": forecast}
}
```

- [ ] **Step 2: Build to confirm no compile errors**

```bash
cd /Users/sarath.m/workspace/work/paisa && go build ./... 2>&1
```

Expected: exits 0, no output.

- [ ] **Step 3: Run all Go tests**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./... 2>&1
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add internal/server/networth.go
git commit -m "feat: add forecast field to GetNetworth response"
```

---

## Task 4: Backend — `computeExpenseForecast`

**Files:**
- Modify: `internal/server/expense.go`
- Create: `internal/server/expense_forecast_test.go`

`computeExpenseForecast` generates synthetic `posting.Posting` objects for the next 6 months, one per `Expenses:*` account per month. It uses budget postings (`forecast=true`) when available, else a 3-month historical average. The source is stored in `posting.Note` as `"budget"` or `"historical"`.

- [ ] **Step 1: Write the failing test**

Create `internal/server/expense_forecast_test.go`:

```go
package server

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// expPosting builds a posting at the 15th of the given month offset from now.
// monthOffset: -3 = 3 months ago, 1 = next month.
func expPosting(monthOffset int, account string, amount float64) posting.Posting {
	now := time.Now()
	t := now.AddDate(0, monthOffset, 0)
	d := time.Date(t.Year(), t.Month(), 15, 0, 0, 0, 0, time.Local)
	return posting.Posting{
		Date:      d,
		Account:   account,
		Amount:    decimal.NewFromFloat(amount),
		Commodity: "INR",
	}
}

func TestComputeExpenseForecast_HistoricalFallback(t *testing.T) {
	// 3 months of actual expenses (6000, 9000, 9000) → average = 8000
	actuals := []posting.Posting{
		expPosting(-3, "Expenses:Food", 6000),
		expPosting(-2, "Expenses:Food", 9000),
		expPosting(-1, "Expenses:Food", 9000),
	}

	result := computeExpenseForecast(actuals, []posting.Posting{})

	assert.Len(t, result, 6, "one account × 6 months = 6 forecast postings")
	for _, p := range result {
		assert.Equal(t, "Expenses:Food", p.Account)
		assert.Equal(t, "historical", p.Note)
		assert.True(t, p.Forecast)
		assert.True(t, p.Amount.Equal(decimal.NewFromFloat(8000)),
			"expected avg 8000, got %v", p.Amount)
	}
}

func TestComputeExpenseForecast_BudgetTakesPriority(t *testing.T) {
	actual := expPosting(-1, "Expenses:Rent", 20000)

	// Budget posting for next month (Forecast=true, amount=25000)
	budget := expPosting(1, "Expenses:Rent", 25000)
	budget.Forecast = true
	budget.Note = ""

	result := computeExpenseForecast([]posting.Posting{actual}, []posting.Posting{budget})

	// Month 1 of the forecast should match the budget amount
	assert.NotEmpty(t, result)
	first := result[0]
	assert.Equal(t, "budget", first.Note)
	assert.True(t, first.Amount.Equal(decimal.NewFromFloat(25000)))
}

func TestComputeExpenseForecast_SixMonthsOneFuturePerAccount(t *testing.T) {
	actuals := []posting.Posting{
		expPosting(-1, "Expenses:Utilities", 3000),
	}

	result := computeExpenseForecast(actuals, []posting.Posting{})

	assert.Len(t, result, 6)
	now := time.Now()
	for _, p := range result {
		assert.True(t, p.Date.After(now), "date %v should be after now", p.Date)
		assert.Equal(t, 15, p.Date.Day(), "sentinel date should be 15th")
		assert.True(t, p.Forecast)
	}
}

func TestComputeExpenseForecast_EmptyActualsProducesNoForecast(t *testing.T) {
	result := computeExpenseForecast([]posting.Posting{}, []posting.Posting{})
	assert.Len(t, result, 0)
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/server/... -run TestComputeExpenseForecast 2>&1
```

Expected: FAIL with "undefined: computeExpenseForecast"

- [ ] **Step 3: Implement `computeExpenseForecast` in `internal/server/expense.go`**

Add `"time"` and `"github.com/ananthakumaran/paisa/internal/config"` to the import block in `expense.go`. The current imports are `fmt`, `sort`, `strings` + internal packages. The new function uses `time.Date(...)` and `config.TimeZone()`. The updated import block:

```go
import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/config"
	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/model/transaction"
	"github.com/ananthakumaran/paisa/internal/query"
	"github.com/ananthakumaran/paisa/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)
```

Add the function at the bottom of `internal/server/expense.go`:

```go
// computeExpenseForecast generates synthetic forecast postings for the next 6 months.
// budgetPostings should be forecast=true postings (from query.Forecast()).
// actuals should be the historical Expenses:* postings.
func computeExpenseForecast(actuals []posting.Posting, budgetPostings []posting.Posting) []posting.Posting {
	now := utils.Now()

	// Budget: account -> month-string("2006-01") -> amount
	type accountMonth struct{ account, month string }
	budgetByKey := make(map[accountMonth]decimal.Decimal)
	for _, p := range budgetPostings {
		k := accountMonth{p.Account, p.Date.Format("2006-01")}
		budgetByKey[k] = budgetByKey[k].Add(p.Amount)
	}

	// Historical average: last 3 completed months per account
	threeMonthsAgo := now.AddDate(0, -3, 0)
	recent := lo.Filter(actuals, func(p posting.Posting, _ int) bool {
		return p.Date.After(threeMonthsAgo) && !p.Forecast
	})
	grouped := lo.GroupBy(recent, func(p posting.Posting) string { return p.Account })
	avgByAccount := make(map[string]decimal.Decimal)
	for acc, ps := range grouped {
		total := lo.Reduce(ps, func(sum decimal.Decimal, p posting.Posting, _ int) decimal.Decimal {
			return sum.Add(p.Amount)
		}, decimal.Zero)
		avgByAccount[acc] = total.Div(decimal.NewFromInt(3))
	}

	// All unique expense accounts seen in actuals
	accounts := lo.Uniq(lo.Map(actuals, func(p posting.Posting, _ int) string { return p.Account }))
	sort.Strings(accounts)

	var result []posting.Posting
	for i := 1; i <= 6; i++ {
		futureMonth := now.AddDate(0, i, 0)
		monthStr := futureMonth.Format("2006-01")
		midMonth := time.Date(futureMonth.Year(), futureMonth.Month(), 15, 0, 0, 0, 0, config.TimeZone())

		for _, acc := range accounts {
			var amount decimal.Decimal
			var note string

			if budgetAmt, ok := budgetByKey[accountMonth{acc, monthStr}]; ok && budgetAmt.GreaterThan(decimal.Zero) {
				amount = budgetAmt
				note = "budget"
			} else if avg, ok := avgByAccount[acc]; ok && avg.GreaterThan(decimal.Zero) {
				amount = avg
				note = "historical"
			}

			if amount.GreaterThan(decimal.Zero) {
				result = append(result, posting.Posting{
					Date:      midMonth,
					Account:   acc,
					Amount:    amount,
					Commodity: "INR",
					Note:      note,
					Forecast:  true,
				})
			}
		}
	}
	return result
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/server/... -run TestComputeExpenseForecast -v 2>&1
```

Expected: PASS — 4 tests passing.

- [ ] **Step 5: Commit**

```bash
git add internal/server/expense.go internal/server/expense_forecast_test.go
git commit -m "feat: add computeExpenseForecast for 6-month expense projection"
```

---

## Task 5: Backend — extend `GetExpense` response

**Files:**
- Modify: `internal/server/expense.go`

Add `forecast_expenses` field to the `GetExpense` response. It contains the synthetic forecast postings. The existing `expenses`, `month_wise`, etc. fields are unchanged — the Svelte page controls which fields it passes to each chart.

- [ ] **Step 1: Update `GetExpense` in `internal/server/expense.go`**

Find the existing `GetExpense` function (it returns a `gin.H` with `expenses`, `month_wise`, `year_wise`, `graph`, etc.). 

Add these two lines before the `return` statement:

```go
forecastPostings := query.Init(db).Like("Expenses:%").Forecast().All()
forecastExpenses := computeExpenseForecast(expenses, forecastPostings)
```

And add `"forecast_expenses": forecastExpenses` to the returned `gin.H`:

```go
return gin.H{
    "expenses":                expenses,
    "forecast_expenses":       forecastExpenses,
    "month_wise": gin.H{
        "expenses":    utils.GroupByMonth(expenses),
        "incomes":     utils.GroupByMonth(incomes),
        "investments": utils.GroupByMonth(investments),
        "taxes":       utils.GroupByMonth(taxes)},
    "year_wise": gin.H{
        "expenses":    utils.GroupByFY(expenses),
        "incomes":     utils.GroupByFY(incomes),
        "investments": utils.GroupByFY(investments),
        "taxes":       utils.GroupByFY(taxes)},
    "graph":                   graph,
    "segmented_graph":         segmentedGraph,
    "segmented_graph_monthly": segmentedGraphMonthly,
    "segmented_graph_weekly":  segmentedGraphWeekly}
```

- [ ] **Step 2: Build to confirm no compile errors**

```bash
cd /Users/sarath.m/workspace/work/paisa && go build ./... 2>&1
```

Expected: exits 0.

- [ ] **Step 3: Run all Go tests**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./... 2>&1
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add internal/server/expense.go
git commit -m "feat: add forecast_expenses to GetExpense response"
```

---

## Task 6: Frontend — extend `renderNetworth` with forecast rendering

**Files:**
- Modify: `src/lib/networth.ts`

`renderNetworth` currently takes `(points: Networth[], element: Element)`. Extend it to `(points: Networth[], forecastPoints: Networth[], element: Element)`.

After drawing the existing chart (solid lines, gain/loss areas), append:
1. A vertical dashed "today" marker line.
2. A shaded uncertainty band (±10% of last actual networth) over the forecast period.
3. A dashed, muted-opacity continuation line through the forecast points.

The x-axis domain is extended to cover the last forecast point's date when forecast points are provided.

- [ ] **Step 1: Update the function signature and x-domain in `src/lib/networth.ts`**

Find the `renderNetworth` function opening:

```ts
export function renderNetworth(
  points: Networth[],
  element: Element
): { destroy: () => void; legends: Legend[] } {
  const start = _.min(_.map(points, (p) => p.date)),
    end = now();
```

Replace with:

```ts
export function renderNetworth(
  points: Networth[],
  forecastPoints: Networth[],
  element: Element
): { destroy: () => void; legends: Legend[] } {
  const start = _.min(_.map(points, (p) => p.date));
  const lastForecast = _.last(forecastPoints);
  const end = lastForecast ? lastForecast.date : now();
```

- [ ] **Step 2: Add forecast rendering after the existing voronoi/hover code**

The existing function ends around line 248 with `return { destroy, legends };`. Insert the forecast rendering block just before the `return`:

Find:
```ts
  const legends: Legend[] = [
```

Add this block immediately before those `legends` lines (after the voronoi mouseover code):

```ts
  // Forecast rendering
  if (forecastPoints.length > 0) {
    const todayX = x(now());

    // "Today" vertical marker
    g.append("line")
      .attr("x1", todayX)
      .attr("x2", todayX)
      .attr("y1", 0)
      .attr("y2", height)
      .style("stroke", "#6b7280")
      .style("stroke-width", "1")
      .style("stroke-dasharray", "4,3");

    // Uncertainty band: ±10% of last actual networth value
    const lastActual = networth(_.last(points));
    const bandWidth = Math.abs(lastActual) * 0.1;
    const bandData = [_.last(points), ...forecastPoints];

    g.append("path")
      .datum(bandData)
      .style("fill", lineScale("networth"))
      .style("opacity", "0.08")
      .attr(
        "d",
        d3
          .area<Networth>()
          .curve(d3.curveMonotoneX)
          .x((d) => x(d.date))
          .y0((d) => y(networth(d) - bandWidth))
          .y1((d) => y(networth(d) + bandWidth))
      );

    // Dashed forecast line
    g.append("path")
      .datum([_.last(points), ...forecastPoints])
      .style("fill", "none")
      .style("stroke", lineScale("networth"))
      .style("stroke-width", "1.5")
      .style("stroke-dasharray", "6,4")
      .style("opacity", "0.5")
      .attr(
        "d",
        d3
          .line<Networth>()
          .curve(d3.curveMonotoneX)
          .x((d) => x(d.date))
          .y((d) => y(networth(d)))
      );

    // Forecast data point circles with tooltips
    g.selectAll(".forecast-dot")
      .data(forecastPoints)
      .enter()
      .append("circle")
      .attr("class", "forecast-dot")
      .attr("r", 3)
      .attr("cx", (d) => x(d.date))
      .attr("cy", (d) => y(networth(d)))
      .style("fill", lineScale("networth"))
      .style("opacity", "0.5")
      .attr(
        "data-tippy-content",
        (d) =>
          tooltip([
            ["Projected", d.date.format("MMM YYYY")],
            ["Net Worth", [formatCurrency(networth(d)), "has-text-weight-bold has-text-right"]]
          ])
      );
  }
```

- [ ] **Step 3: Run the frontend type check**

```bash
cd /Users/sarath.m/workspace/work/paisa && npx tsc --noEmit 2>&1 | head -20
```

Expected: no new type errors.

- [ ] **Step 4: Run frontend tests**

```bash
cd /Users/sarath.m/workspace/work/paisa && bun test --preload ./src/happydom.ts src 2>&1 | tail -10
```

Expected: all existing tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/lib/networth.ts
git commit -m "feat: extend renderNetworth with 6-month forecast line and shaded band"
```

---

## Task 7: Frontend — update networth Svelte page

**Files:**
- Modify: `src/routes/(app)/assets/networth/+page.svelte`

The page fetches `result.networthTimeline` and passes it to `renderNetworth`. Now it must also destructure `result.forecast` and pass it as the second argument.

- [ ] **Step 1: Update the `onMount` fetch in `+page.svelte`**

Find in `src/routes/(app)/assets/networth/+page.svelte`:

```ts
  onMount(async () => {
    const result = await ajax("/api/networth");
    points = result.networthTimeline;
```

Replace with:

```ts
  onMount(async () => {
    const result = await ajax("/api/networth");
    points = result.networthTimeline;
    forecastPoints = result.forecast ?? [];
```

- [ ] **Step 2: Add `forecastPoints` variable declaration**

Find in the `<script>` block where `points` is declared:

```ts
  let points: Networth[] = [];
```

Add below it:

```ts
  let forecastPoints: Networth[] = [];
```

- [ ] **Step 3: Update the `$:` reactive block that calls `renderNetworth`**

Find:

```ts
  $: if (!_.isEmpty(points)) {
    if (destroy) {
      destroy();
    }

    ({ destroy, legends } = renderNetworth(
      _.filter(
        points,
        (p) => p.date.isSameOrBefore($dateRange.to) && p.date.isSameOrAfter($dateRange.from)
      ),
      svg
    ));
  }
```

Replace with:

```ts
  $: if (!_.isEmpty(points)) {
    if (destroy) {
      destroy();
    }

    ({ destroy, legends } = renderNetworth(
      _.filter(
        points,
        (p) => p.date.isSameOrBefore($dateRange.to) && p.date.isSameOrAfter($dateRange.from)
      ),
      forecastPoints,
      svg
    ));
  }
```

- [ ] **Step 4: Type check**

```bash
cd /Users/sarath.m/workspace/work/paisa && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add "src/routes/(app)/assets/networth/+page.svelte"
git commit -m "feat: pass forecast points to renderNetworth in networth page"
```

---

## Task 8: Frontend — hatched forecast bars in monthly expense timeline

**Files:**
- Modify: `src/lib/expense/monthly.ts`

`renderMonthlyExpensesTimeline` groups postings by month. Forecast postings (with `forecast: true`) will naturally land in future month buckets. This task makes those bars render with hatched SVG patterns instead of solid fills.

**Approach:**
1. Add `isForecast: boolean` to the local `Point` interface.
2. When building points via `forEachMonth`, set `isForecast: true` if any posting in that month bucket has `forecast: true`.
3. Add SVG `<defs>` with a hatch `<pattern>` per category color (keyed by sanitized group name).
4. In the `enter` and `update` handlers for bars, override `fill` per-rect when `isForecast` is true.

- [ ] **Step 1: Extend the `Point` interface and set `isForecast` when building points**

Find in `src/lib/expense/monthly.ts` the local `Point` interface:

```ts
  interface Point {
    month: string;
    timestamp: Dayjs;
    [key: string]: number | string | Dayjs;
  }
```

Replace with:

```ts
  interface Point {
    month: string;
    timestamp: Dayjs;
    isForecast: boolean;
    [key: string]: number | string | Dayjs | boolean;
  }
```

Find where `points.push(...)` is called inside `forEachMonth`:

```ts
    points.push(
      _.merge(
        {
          timestamp: month,
          month: month.format(timeFormat),
          postings: postings,
          trend: {}
        },
        defaultValues,
        values
      )
    );
```

Replace with:

```ts
    const isForecast = postings.length > 0 && postings.every((p) => p.forecast);
    points.push(
      _.merge(
        {
          timestamp: month,
          month: month.format(timeFormat),
          postings: postings,
          isForecast: isForecast,
          trend: {}
        },
        defaultValues,
        values
      )
    );
```

- [ ] **Step 2: Add SVG `<defs>` with hatch patterns**

Find in `renderMonthlyExpensesTimeline` where the `<g>` element is created:

```ts
  const g = svg.append("g").attr("transform", "translate(" + margin.left + "," + margin.top + ")");
```

Add hatch pattern defs immediately after that line:

```ts
  const defs = svg.append("defs");
  groups.forEach((group) => {
    const color = z(group);
    const patternId = `hatch-${group.replace(/[^a-zA-Z0-9]/g, "-")}`;
    const pattern = defs
      .append("pattern")
      .attr("id", patternId)
      .attr("patternUnits", "userSpaceOnUse")
      .attr("width", 6)
      .attr("height", 6)
      .attr("patternTransform", "rotate(45)");
    pattern
      .append("line")
      .attr("x1", 0)
      .attr("y1", 0)
      .attr("x2", 0)
      .attr("y2", 6)
      .style("stroke", color)
      .style("stroke-width", 2)
      .style("opacity", 0.6);
  });
```

- [ ] **Step 3: Override bar fill for forecast months**

Find the `enter` handler for bars (inside the `bars.selectAll("g")...` chain). It currently ends with:

```ts
          enter
            .append("rect")
            .attr("class", "zoomable")
            .on("click", (_event, data) => {
              const timestamp: Dayjs = data.data.timestamp as any;
              monthStore.set(timestamp.format("YYYY-MM"));
            })
            .attr("data-tippy-content", tooltipContent(allowedGroups))
            .attr("x", function (d) { ... })
            .attr("width", Math.min(x.bandwidth(), MAX_BAR_WIDTH))
            .attr("y", y.range()[0])
            .transition(t)
            .attr("y", function (d) { return y(d[1]); })
            .attr("height", function (d) { return y(d[0]) - y(d[1]); }),
```

After `.attr("class", "zoomable")` add these two attributes (before the `.on("click"...)` call):

```ts
            .attr("fill", function (d) {
              if ((d.data as any).isForecast) {
                const key = (d3.select(this.parentElement).datum() as any).key as string;
                return `url(#hatch-${key.replace(/[^a-zA-Z0-9]/g, "-")})`;
              }
              return null;
            })
            .attr("stroke", function (d) {
              if ((d.data as any).isForecast) {
                const key = (d3.select(this.parentElement).datum() as any).key as string;
                return z(key);
              }
              return null;
            })
            .attr("stroke-dasharray", function (d) {
              return (d.data as any).isForecast ? "3,2" : null;
            })
```

Do the same for the `update` handler — add the same three `attr` calls at the start of the `update` chain.

- [ ] **Step 4: Run frontend type check**

```bash
cd /Users/sarath.m/workspace/work/paisa && npx tsc --noEmit 2>&1 | head -20
```

Expected: no new errors.

- [ ] **Step 5: Run frontend tests**

```bash
cd /Users/sarath.m/workspace/work/paisa && bun test --preload ./src/happydom.ts src 2>&1 | tail -10
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add src/lib/expense/monthly.ts
git commit -m "feat: render forecast expense bars as hatched SVG patterns in monthly timeline"
```

---

## Task 9: Frontend — update expense monthly page + nudge banner

**Files:**
- Modify: `src/routes/(app)/expense/monthly/+page.svelte`

The page currently fetches `expenses` and `month_wise` from `/api/expense`. Now it also gets `forecast_expenses`. These are concatenated with `expenses` before passing to `renderMonthlyExpensesTimeline` (so the render function sees future months). The `month_wise` stats sidebar is NOT affected (still uses only actual postings).

A nudge banner is added below the monthly expense chart SVG. It reads: "N expense categories are estimated from 3-month average. Set budgets to improve forecast accuracy." — visible only when at least one forecast posting has `note === "historical"`.

- [ ] **Step 1: Add `forecastExpenses` variable and update destructuring in `onMount`**

Find the `let` declarations in the `<script>` block:

```ts
  let groups = writable([]);
  let z: d3.ScaleOrdinal<string, string, never>,
    renderer: (ps: Posting[]) => void,
    expenses: Posting[],
    ...
```

Add `forecastExpenses: Posting[]` to the declarations:

```ts
  let groups = writable([]);
  let forecastExpenses: Posting[] = [];
  let z: d3.ScaleOrdinal<string, string, never>,
    renderer: (ps: Posting[]) => void,
    expenses: Posting[],
    ...
```

Find the `onMount` destructuring:

```ts
    ({
      expenses: expenses,
      month_wise: {
        expenses: grouped_expenses,
        incomes: grouped_incomes,
        investments: grouped_investments,
        taxes: grouped_taxes
      }
    } = await ajax("/api/expense"));
```

Replace with:

```ts
    ({
      expenses: expenses,
      forecast_expenses: forecastExpenses,
      month_wise: {
        expenses: grouped_expenses,
        incomes: grouped_incomes,
        investments: grouped_investments,
        taxes: grouped_taxes
      }
    } = await ajax("/api/expense"));
    forecastExpenses = forecastExpenses ?? [];
```

- [ ] **Step 2: Pass combined postings to `renderMonthlyExpensesTimeline`**

Find:

```ts
    ({ z, destroy, legends } = renderMonthlyExpensesTimeline(expenses, groups, month, dateRange));
```

Replace with:

```ts
    ({ z, destroy, legends } = renderMonthlyExpensesTimeline([...expenses, ...forecastExpenses], groups, month, dateRange));
```

- [ ] **Step 3: Add `historicalCount` computed variable**

Add this reactive statement after the existing reactive blocks:

```ts
  $: historicalCount = forecastExpenses.filter((p) => p.note === "historical")
    .map((p) => p.account)
    .filter((acc, i, arr) => arr.indexOf(acc) === i).length;
```

- [ ] **Step 4: Add the nudge banner in the template**

Find the expense timeline box in the template:

```html
          <div class="column is-full">
            <div class="box">
              <ZeroState item={expenses}>
                <strong>Oops!</strong> You have no expenses.
              </ZeroState>
              <LegendCard {legends} clazz="ml-4 overflow-x-auto" />
              <svg id="d3-monthly-expense-timeline" width="100%" height="400" />
            </div>
          </div>
```

Add the nudge banner after the `<svg>` tag, before the closing `</div>` of the box:

```html
              {#if historicalCount > 0}
                <div class="notification is-warning is-light mt-3 p-3" style="font-size: 0.85rem">
                  {historicalCount} expense {historicalCount === 1 ? "category is" : "categories are"} estimated from your 3-month average.
                  <a href="/expense/budget">Set budgets →</a> to improve forecast accuracy.
                </div>
              {/if}
```

- [ ] **Step 5: Type check**

```bash
cd /Users/sarath.m/workspace/work/paisa && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors.

- [ ] **Step 6: Run frontend tests**

```bash
cd /Users/sarath.m/workspace/work/paisa && bun test --preload ./src/happydom.ts src 2>&1 | tail -10
```

Expected: all pass.

- [ ] **Step 7: Run all Go tests**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./... 2>&1
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add "src/routes/(app)/expense/monthly/+page.svelte"
git commit -m "feat: add expense forecast to monthly timeline with budget nudge banner"
```

---

## Verification

After all tasks complete, manually verify:

1. Start the app (`go run .` or `make run`) and open the Networth page.
2. The Networth chart should show a dashed continuation of the networth line extending 6 months past today, with a faint shaded band and a vertical "today" marker.
3. Open Expense → Monthly. The chart should show 6 future months with hatched bars.
4. If any category has no budget, the yellow nudge banner appears below the chart with a link to `/expense/budget`.
5. Setting budgets in ledger for an account should change its forecast bar tooltip from "est. from 3mo avg" to "budget" after re-sync.

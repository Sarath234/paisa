# Net Worth Projection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Projection page under Assets that charts net worth and net investment N years forward, driven by current net worth, 12-month average monthly savings, and a user-adjustable return rate slider.

**Architecture:** Backend adds `monthlySavings` (12-month avg of income−expenses) to the existing `/api/networth` response. Frontend computes all projection points from that value plus slider state — no extra API calls. A new D3 renderer in `src/lib/projection.ts` draws two lines (net worth, net investment) with a shaded gain area.

**Tech Stack:** Go + GORM (backend), SvelteKit + D3 v7 + tippy.js (frontend), Bulma CSS, shopspring/decimal.

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/server/networth.go` | Add `computeMonthlySavings`, update `GetNetworth` |
| Create | `internal/server/networth_test.go` | Test `computeMonthlySavings` |
| Modify | `src/lib/utils.ts` | Add `monthlySavings` to ajax overload |
| Create | `src/lib/projection.ts` | D3 projection renderer |
| Create | `src/routes/(app)/assets/projection/+page.svelte` | Projection page |
| Modify | `src/lib/components/Navbar.svelte` | Add Projection nav entry |

---

## Task 1: Backend — `computeMonthlySavings` + updated `GetNetworth`

**Files:**
- Modify: `internal/server/networth.go`
- Create: `internal/server/networth_test.go`

### Context

`internal/server/networth.go` is in `package server`. The package already has a `filterMonth` helper in `insights.go` (same package) — use it directly. Income postings have **negative** Amount in Paisa's ledger; negate them to get positive income. `utils.IsSameOrParent(account, "Income")` and `utils.IsSameOrParent(account, "Expenses")` detect account types. The `query` package: `query.Init(db).Like("Income:%", "Expenses:%").LastNMonths(13).All()` fetches 13 months including the current partial month.

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

func TestComputeMonthlySavings_Average(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	var postings []posting.Posting
	// 12 complete months: income -100000, expenses 70000 → savings 30000/month
	for i := 1; i <= 12; i++ {
		m := now.AddDate(0, -i, 0)
		postings = append(postings, p("Income:Salary", m, "-100000"))
		postings = append(postings, p("Expenses:Rent", m, "70000"))
	}
	// Current partial month (should be excluded)
	postings = append(postings, p("Income:Salary", now, "-100000"))
	postings = append(postings, p("Expenses:Rent", now, "10000"))

	result := computeMonthlySavings(postings, now)
	expected, _ := decimal.NewFromString("30000")
	assert.True(t, result.Equal(expected), "expected 30000, got %s", result)
}

func TestComputeMonthlySavings_ZeroIfNoPostings(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	result := computeMonthlySavings([]posting.Posting{}, now)
	assert.True(t, result.IsZero())
}

func TestComputeMonthlySavings_NegativeWhenSpendingExceedsIncome(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	var postings []posting.Posting
	for i := 1; i <= 12; i++ {
		m := now.AddDate(0, -i, 0)
		postings = append(postings, p("Income:Salary", m, "-50000"))
		postings = append(postings, p("Expenses:Rent", m, "80000"))
	}
	result := computeMonthlySavings(postings, now)
	assert.True(t, result.IsNegative())
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /path/to/paisa
go test ./internal/server/... -run TestComputeMonthlySavings -v
```

Expected: FAIL — `computeMonthlySavings: undefined`

- [ ] **Step 3: Implement `computeMonthlySavings` in `networth.go`**

Add this function at the bottom of `internal/server/networth.go` (after `computeNetworthTimeline`):

```go
func computeMonthlySavings(postings []posting.Posting, now time.Time) decimal.Decimal {
	total := decimal.Zero
	for i := 1; i <= 12; i++ {
		m := now.AddDate(0, -i, 0)
		monthly := filterMonth(postings, m.Year(), m.Month())
		income := decimal.Zero
		expenses := decimal.Zero
		for _, p := range monthly {
			if utils.IsSameOrParent(p.Account, "Income") {
				income = income.Add(p.Amount.Neg())
			} else if utils.IsSameOrParent(p.Account, "Expenses") {
				expenses = expenses.Add(p.Amount)
			}
		}
		total = total.Add(income.Sub(expenses))
	}
	return total.Div(decimal.NewFromInt(12)).Round(2)
}
```

- [ ] **Step 4: Update `GetNetworth` to include `monthlySavings`**

Replace the existing `GetNetworth` function in `internal/server/networth.go`:

```go
func GetNetworth(db *gorm.DB) gin.H {
	postings := query.Init(db).Like("Assets:%", "Income:CapitalGains:%", "Liabilities:%").UntilToday().All()
	postings = service.PopulateMarketPrice(db, postings)
	networthTimeline := computeNetworthTimeline(db, postings, false)
	xirr := service.XIRR(db, postings)

	incExpPostings := query.Init(db).Like("Income:%", "Expenses:%").LastNMonths(13).All()
	monthlySavings := computeMonthlySavings(incExpPostings, utils.Now())

	return gin.H{"networthTimeline": networthTimeline, "xirr": xirr, "monthlySavings": monthlySavings}
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/server/... -run TestComputeMonthlySavings -v
```

Expected: PASS — 3 tests passing

- [ ] **Step 6: Run full test suite**

```bash
go test ./internal/...
```

Expected: all existing tests still pass

- [ ] **Step 7: Commit**

```bash
git add internal/server/networth.go internal/server/networth_test.go
git commit -m "feat: add monthlySavings (12mo avg) to /api/networth response"
```

---

## Task 2: TypeScript — update `ajax("/api/networth")` type

**Files:**
- Modify: `src/lib/utils.ts`

### Context

The ajax overload for `/api/networth` is at line ~574 of `src/lib/utils.ts`:
```typescript
export function ajax(route: "/api/networth"): Promise<{
  networthTimeline: Networth[];
  xirr: number;
}>;
```
Add `monthlySavings: number` to this return type.

- [ ] **Step 1: Update the ajax overload**

In `src/lib/utils.ts`, find the `/api/networth` overload and replace it:

```typescript
export function ajax(route: "/api/networth"): Promise<{
  networthTimeline: Networth[];
  xirr: number;
  monthlySavings: number;
}>;
```

- [ ] **Step 2: TypeScript check**

```bash
npx tsc --noEmit
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add src/lib/utils.ts
git commit -m "feat: add monthlySavings to /api/networth TypeScript type"
```

---

## Task 3: D3 renderer — `src/lib/projection.ts`

**Files:**
- Create: `src/lib/projection.ts`

### Context

Pattern to follow: `src/lib/networth.ts`. Key imports available from `$lib/utils`: `formatCurrency`, `formatCurrencyCrude`, `isMobile`, `tooltip`, `svgUrl`. Colors from `$lib/colors`: `COLORS.primary` (net worth line — deep purple a100 `#b388ff`), `COLORS.secondary` (investment line — light blue a400 `#40c4ff`), `COLORS.gain` (`#b2df8a` for the shaded fill). tippy.js for hover tooltips.

Projection formula per month:
- `W[n] = W[n-1] * (1 + r/12) + monthlySavings`
- `I[n] = I[n-1] + monthlySavings`

where `r` is the decimal annual return rate (e.g., `0.12` for 12%).

X axis: time scale from today → today + horizonMonths. Y axis: linear scale over all W and I values.

- [ ] **Step 1: Create `src/lib/projection.ts`**

```typescript
import * as d3 from "d3";
import _ from "lodash";
import tippy from "tippy.js";
import COLORS from "./colors";
import {
  formatCurrency,
  formatCurrencyCrude,
  isMobile,
  svgUrl,
  tooltip,
  type Legend
} from "./utils";

interface ProjectionPoint {
  date: Date;
  networth: number;
  investment: number;
}

function computePoints(
  currentNetworth: number,
  currentInvestment: number,
  monthlySavings: number,
  horizonMonths: number,
  annualReturnRate: number
): ProjectionPoint[] {
  const monthlyRate = annualReturnRate / 12;
  const points: ProjectionPoint[] = [];
  const origin = new Date();
  origin.setDate(1); // snap to month start for clean x-axis

  let w = currentNetworth;
  let inv = currentInvestment;

  for (let i = 0; i <= horizonMonths; i++) {
    const date = new Date(origin);
    date.setMonth(date.getMonth() + i);
    points.push({ date, networth: w, investment: inv });
    w = w * (1 + monthlyRate) + monthlySavings;
    inv = inv + monthlySavings;
  }
  return points;
}

export function renderProjection(
  currentNetworth: number,
  currentInvestment: number,
  monthlySavings: number,
  horizonYears: number,
  annualReturnPct: number,
  element: Element
): { destroy: () => void; legends: Legend[] } {
  const horizonMonths = horizonYears * 12;
  const annualRate = annualReturnPct / 100;
  const points = computePoints(
    currentNetworth,
    currentInvestment,
    monthlySavings,
    horizonMonths,
    annualRate
  );

  const svg = d3.select(element);
  svg.selectAll("*").remove();

  const right = isMobile() ? 10 : 80;
  const margin = { top: 15, right, bottom: 20, left: 40 };
  const width =
    Math.max(element.parentElement.clientWidth, 800) - margin.left - margin.right;
  const height = +svg.attr("height") - margin.top - margin.bottom;
  const g = svg
    .append("g")
    .attr("transform", `translate(${margin.left},${margin.top})`);

  svg.attr("width", width + margin.left + margin.right);

  const allValues = _.flatMap(points, (p) => [p.networth, p.investment]);
  allValues.push(0);

  const x = d3
    .scaleTime()
    .range([0, width])
    .domain([points[0].date, points[points.length - 1].date]);

  const y = d3.scaleLinear().range([height, 0]).domain(d3.extent(allValues));

  // Axes
  g.append("g")
    .attr("class", "axis x")
    .attr("transform", `translate(0,${height})`)
    .call(d3.axisBottom(x));

  g.append("g")
    .attr("class", "axis y")
    .call(d3.axisLeft(y).tickSize(-width).tickFormat(formatCurrencyCrude));

  if (!isMobile()) {
    g.append("g")
      .attr("class", "axis y")
      .attr("transform", `translate(${width},0)`)
      .call(d3.axisRight(y).tickPadding(5).tickFormat(formatCurrencyCrude));
  }

  // Shaded gain area between investment and networth
  const gainAreaID = _.uniqueId("proj-gain");
  const gainArea = d3
    .area<ProjectionPoint>()
    .curve(d3.curveMonotoneX)
    .x((d) => x(d.date))
    .y0((d) => y(d.investment))
    .y1((d) => y(d.networth));

  g.append("path")
    .datum(points)
    .style("fill", COLORS.gain)
    .style("opacity", "0.2")
    .attr("d", gainArea);

  // Net Investment line (dashed)
  g.append("path")
    .datum(points)
    .style("stroke", COLORS.secondary)
    .style("stroke-width", "1.5")
    .style("stroke-dasharray", "6,4")
    .style("fill", "none")
    .attr(
      "d",
      d3
        .line<ProjectionPoint>()
        .curve(d3.curveMonotoneX)
        .x((d) => x(d.date))
        .y((d) => y(d.investment))
    );

  // Net Worth line (solid)
  g.append("path")
    .datum(points)
    .style("stroke", COLORS.primary)
    .style("stroke-width", "2")
    .style("fill", "none")
    .attr(
      "d",
      d3
        .line<ProjectionPoint>()
        .curve(d3.curveMonotoneX)
        .x((d) => x(d.date))
        .y((d) => y(d.networth))
    );

  // Hover
  const hoverCircle = g.append("circle").attr("r", "3").attr("fill", "none");
  const t = tippy(hoverCircle.node(), { theme: "light", delay: 0, allowHTML: true });

  const voronoiNW: [number, number][] = points.map((d) => [x(d.date), y(d.networth)]);
  const voronoiInv: [number, number][] = points.map((d) => [x(d.date), y(d.investment)]);
  const voronoi = d3.Delaunay.from(voronoiNW.concat(voronoiInv)).voronoi([0, 0, width, height]);

  const labelFmt = d3.timeFormat("%b %Y");

  g.append("g")
    .selectAll("path")
    .data(
      (points.map((p) => ["networth", p]) as ["networth" | "investment", ProjectionPoint][]).concat(
        points.map((p) => ["investment", p]) as ["networth" | "investment", ProjectionPoint][]
      )
    )
    .enter()
    .append("path")
    .style("pointer-events", "all")
    .style("fill", "none")
    .attr("d", (_, i) => voronoi.renderCell(i))
    .on("mouseover", (_, [type, d]) => {
      const cy = type === "networth" ? y(d.networth) : y(d.investment);
      const color = type === "networth" ? COLORS.primary : COLORS.secondary;
      hoverCircle.attr("cx", x(d.date)).attr("cy", cy).attr("fill", color);
      t.setProps({
        placement: type === "networth" ? "top" : "bottom",
        content: tooltip([
          ["Month", labelFmt(d.date)],
          ["Projected Net Worth", [formatCurrency(d.networth), "has-text-weight-bold has-text-right"]],
          ["Projected Net Investment", [formatCurrency(d.investment), "has-text-weight-bold has-text-right"]]
        ])
      });
      t.show();
    })
    .on("mouseout", () => {
      t.hide();
      hoverCircle.attr("fill", "none");
    });

  const legends: Legend[] = [
    { label: "Projected Net Worth", color: COLORS.primary, shape: "line" },
    { label: "Projected Net Investment", color: COLORS.secondary, shape: "line" },
    { label: "Market Gain", color: COLORS.gain, shape: "square" }
  ];

  return { destroy: () => t.destroy(), legends };
}
```

- [ ] **Step 2: TypeScript check**

```bash
npx tsc --noEmit
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add src/lib/projection.ts
git commit -m "feat: add projection D3 renderer"
```

---

## Task 4: Page — `src/routes/(app)/assets/projection/+page.svelte`

**Files:**
- Create: `src/routes/(app)/assets/projection/+page.svelte`

### Context

Follow the pattern of `src/routes/(app)/assets/networth/+page.svelte`. Key components: `LevelItem` for the stats tiles, `ZeroState` for empty state, `LegendCard` for the legend. `formatCurrency` for currency display. The page fetches `/api/networth`, extracts the last `networthTimeline` point for current net worth and net investment, and uses `monthlySavings`. Reactive statements rerender the chart when `horizonYears` or `returnPct` changes (via `$: if` block).

Current net worth = `investmentAmount + gainAmount - withdrawalAmount`.  
Current net investment = `investmentAmount - withdrawalAmount`.  

Both values come from the last point in `networthTimeline`.

- [ ] **Step 1: Create the page**

```svelte
<!-- src/routes/(app)/assets/projection/+page.svelte -->
<script lang="ts">
  import { ajax, formatCurrency, isMobile, type Legend } from "$lib/utils";
  import COLORS from "$lib/colors";
  import { renderProjection } from "$lib/projection";
  import _ from "lodash";
  import { onDestroy, onMount } from "svelte";
  import LevelItem from "$lib/components/LevelItem.svelte";
  import ZeroState from "$lib/components/ZeroState.svelte";
  import LegendCard from "$lib/components/LegendCard.svelte";

  const HORIZONS = [1, 3, 5, 10];

  let currentNetworth = 0;
  let currentInvestment = 0;
  let monthlySavings = 0;
  let horizonYears = 5;
  let returnPct = 12;
  let loaded = false;

  let svg: Element;
  let destroy: (() => void) | null = null;
  let legends: Legend[] = [];

  function projectedValue(): number {
    const r = returnPct / 100 / 12;
    let w = currentNetworth;
    for (let i = 0; i < horizonYears * 12; i++) {
      w = w * (1 + r) + monthlySavings;
    }
    return w;
  }

  $: if (loaded && svg) {
    if (destroy) destroy();
    ({ destroy, legends } = renderProjection(
      currentNetworth,
      currentInvestment,
      monthlySavings,
      horizonYears,
      returnPct,
      svg
    ));
  }

  onDestroy(() => {
    if (destroy) destroy();
  });

  onMount(async () => {
    const result = await ajax("/api/networth");
    const last = _.last(result.networthTimeline);
    if (last) {
      currentNetworth = last.investmentAmount + last.gainAmount - last.withdrawalAmount;
      currentInvestment = last.investmentAmount - last.withdrawalAmount;
    }
    monthlySavings = result.monthlySavings;
    loaded = true;
  });
</script>

<section class="section">
  <div class="container is-fluid">
    <nav class="level {isMobile() && 'grid-2'}">
      <LevelItem title="Current Net Worth" color={COLORS.primary} value={formatCurrency(currentNetworth)} />
      <LevelItem
        title="Monthly Savings"
        color={COLORS.secondary}
        value={formatCurrency(monthlySavings)}
        subtitle="12-month avg"
      />
      <LevelItem
        title="Projected in {horizonYears}Y"
        color={COLORS.gainText}
        value={loaded ? formatCurrency(projectedValue()) : "—"}
        subtitle="at {returnPct}% return"
      />
    </nav>
  </div>
</section>

<section class="section">
  <div class="container is-fluid">
    <!-- Controls -->
    <div class="box p-4 mb-4">
      <div class="columns is-vcentered is-mobile">
        <div class="column is-narrow">
          <span class="label is-small mb-0">Horizon</span>
          <div class="buttons has-addons mt-1">
            {#each HORIZONS as h}
              <button
                class="button is-small {horizonYears === h ? 'is-primary' : ''}"
                on:click={() => (horizonYears = h)}
              >{h}Y</button>
            {/each}
          </div>
        </div>
        <div class="column">
          <span class="label is-small mb-0">Annual Return: {returnPct}%</span>
          <input
            class="mt-1"
            type="range"
            min="6"
            max="18"
            step="1"
            bind:value={returnPct}
            style="width:100%;accent-color:{COLORS.primary}"
          />
        </div>
      </div>
    </div>

    <!-- Chart -->
    <div class="columns">
      <div class="column is-12">
        <div class="box overflow-x-auto">
          <ZeroState item={loaded ? [currentNetworth] : []}>
            <strong>Oops!</strong> You have no transactions.
          </ZeroState>
          <LegendCard {legends} clazz="ml-4" />
          <svg id="d3-projection" height="500" bind:this={svg} />
        </div>
      </div>
    </div>
  </div>
</section>
```

- [ ] **Step 2: TypeScript check**

```bash
npx tsc --noEmit
```

Expected: no errors. If `subtitle` prop doesn't exist on `LevelItem`, remove those lines — they're optional labels.

- [ ] **Step 3: Check `LevelItem` props**

```bash
grep -n "export let\|subtitle" src/lib/components/LevelItem.svelte
```

If `subtitle` prop is not defined in `LevelItem`, remove the two `subtitle=` attributes from the page. The tile will still display correctly without them.

- [ ] **Step 4: Commit**

```bash
git add src/routes/\(app\)/assets/projection/+page.svelte
git commit -m "feat: add net worth projection page"
```

---

## Task 5: Navbar — add Projection entry

**Files:**
- Modify: `src/lib/components/Navbar.svelte`

### Context

The Assets section in `Navbar.svelte` (around line 94) looks like:

```typescript
{
  label: "Assets",
  href: "/assets",
  children: [
    { label: "Balance", href: "/balance" },
    { label: "Networth", href: "/networth", dateRangeSelector: true },
    { label: "Investment", href: "/investment" },
    { label: "Gain", href: "/gain" },
    { label: "Allocation", href: "/allocation", help: "allocation-targets" },
    { label: "Analysis", href: "/analysis", tag: "alpha", help: "analysis" }
  ]
}
```

Add `Projection` after `Networth`.

- [ ] **Step 1: Add the nav entry**

In `src/lib/components/Navbar.svelte`, find the Assets children array and add the Projection entry after `Networth`:

```typescript
{ label: "Networth", href: "/networth", dateRangeSelector: true },
{ label: "Projection", href: "/projection" },
```

- [ ] **Step 2: TypeScript check**

```bash
npx tsc --noEmit
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add src/lib/components/Navbar.svelte
git commit -m "feat: add Projection to Assets nav"
```

---

## Task 6: Integration check — dev server smoke test

- [ ] **Step 1: Build the backend**

```bash
go build ./...
```

Expected: no compile errors

- [ ] **Step 2: Start dev server**

```bash
make dev
```

Open `http://localhost:5173/assets/projection`. Verify:
- Stats row shows Current Net Worth, Monthly Savings, Projected value
- Chart renders two lines with shaded area between them
- Horizon toggle buttons (1Y/3Y/5Y/10Y) rerender the chart
- Return rate slider updates the chart and the "Projected in NY" tile value

- [ ] **Step 3: Run full Go test suite**

```bash
go test ./internal/...
```

Expected: all tests pass

- [ ] **Step 4: Final commit if any fixes needed**

```bash
git add -p
git commit -m "fix: projection page integration fixes"
```

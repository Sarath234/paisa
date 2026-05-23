# Savings Rate Dashboard Widget Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two LevelItem tiles to the dashboard Assets block showing the current month's savings rate (with ↑↓ trend indicator) and the pooled 12-month average savings rate.

**Architecture:** Pure frontend change — the `cashFlows` array already returned by `/api/dashboard` contains per-month `income` and `expenses`. We add reactive derivations in the dashboard page to compute the rates, then render a third `<nav class="level grid-2">` row in the existing Assets block. No backend changes, no new components.

**Tech Stack:** Svelte reactivity (`$:` declarations), lodash (`_.takeRight`, `_.sumBy`), existing `LevelItem` component, existing `COLORS` constants.

---

## File Map

| File | Change |
|---|---|
| `src/routes/(app)/+page.svelte` | Add 6 reactive derivations in `<script>`; add third `<nav class="level grid-2">` row in template |

---

## Task 1: Add reactive savings rate computations

**Files:**
- Modify: `src/routes/(app)/+page.svelte`

The `CashFlow` interface (from `src/lib/utils.ts`) has:
```typescript
interface CashFlow {
  date: dayjs.Dayjs;  // use .format("YYYY-MM") to match month string
  income: number;
  expenses: number;
  // (other fields not needed)
}
```

The page already has `cashFlows: CashFlow[]` populated from the API response, and `now()` imported from `$lib/utils`.

- [ ] **Step 1: Add the six reactive derivations to the `<script>` block**

Open `src/routes/(app)/+page.svelte`. After the existing reactive declarations (around line 55–59), add:

```typescript
  // Savings rate: current month
  $: currentSavingsRate = (() => {
    const current = cashFlows.find(
      (cf) => cf.date.format("YYYY-MM") === now().format("YYYY-MM")
    );
    if (!current || current.income === 0) return null;
    return ((current.income - current.expenses) / current.income) * 100;
  })();

  // Savings rate: pooled 12-month average
  $: avg12mSavingsRate = (() => {
    const window = _.takeRight(cashFlows, 12);
    const totalIncome = _.sumBy(window, (cf) => cf.income);
    if (totalIncome === 0) return null;
    const totalExpenses = _.sumBy(window, (cf) => cf.expenses);
    return ((totalIncome - totalExpenses) / totalIncome) * 100;
  })();

  // Trend: up/down/neutral vs 12m avg
  $: savingsTrend = (() => {
    if (currentSavingsRate === null || avg12mSavingsRate === null || cashFlows.length < 2)
      return "neutral" as const;
    if (currentSavingsRate > avg12mSavingsRate) return "up" as const;
    if (currentSavingsRate < avg12mSavingsRate) return "down" as const;
    return "neutral" as const;
  })();

  // Display label for current month: "38.0% ↑" or "—"
  $: savingsRateLabel = (() => {
    if (currentSavingsRate === null) return "—";
    const pct = `${currentSavingsRate.toFixed(1)}%`;
    if (savingsTrend === "up") return `${pct} ↑`;
    if (savingsTrend === "down") return `${pct} ↓`;
    return pct;
  })();

  // Color for current month tile
  $: savingsRateColor = (() => {
    if (currentSavingsRate === null) return COLORS.primary;
    if (currentSavingsRate < 0) return COLORS.lossText;
    if (savingsTrend === "up") return COLORS.gainText;
    if (savingsTrend === "down") return COLORS.lossText;
    return COLORS.primary;
  })();

  // Display label for 12m avg: "32.5%" or "—"
  $: avg12mLabel = avg12mSavingsRate !== null ? `${avg12mSavingsRate.toFixed(1)}%` : "—";
```

- [ ] **Step 2: Run TypeScript check**

```bash
npx tsc --noEmit
```

Expected: no errors. If `now()` shows an error, confirm it is already imported from `$lib/utils` at line ~20. If `_.takeRight` or `_.sumBy` show errors, confirm lodash is imported at line ~11.

---

## Task 2: Add the UI row

**Files:**
- Modify: `src/routes/(app)/+page.svelte`

The existing Assets metrics block (inside `{#if networth}`) has two `<nav class="level grid-2">` rows:
1. Net Worth + Net Investment
2. Gain / Loss + XIRR

We add a third row immediately after row 2.

- [ ] **Step 1: Add the third nav row after the Gain/Loss + XIRR nav**

Locate the closing `</nav>` of the Gain/Loss + XIRR row (around line 191). Add immediately after it:

```svelte
                    <nav class="level grid-2">
                      <LevelItem
                        narrow
                        title="Savings Rate"
                        color={savingsRateColor}
                        value={savingsRateLabel}
                      />
                      <LevelItem
                        narrow
                        title="12m Avg"
                        value={avg12mLabel}
                      />
                    </nav>
```

- [ ] **Step 2: Run TypeScript check**

```bash
npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Verify visually in the browser**

Start the dev server if not running:
```bash
npm run dev
```

Open the dashboard. In the Assets block you should see:

```
Net Worth          Net Investment
Gain / Loss        XIRR
Savings Rate XX%↑  12m Avg YY%
```

Check each edge case manually:
- The current month tile shows a % with ↑ or ↓ (or neither if equal to avg)
- 12m Avg shows a plain %
- Negative savings rate → both tiles red
- Above-average month → Savings Rate tile green with ↑
- Below-average month → Savings Rate tile red with ↓

- [ ] **Step 4: Commit**

```bash
git add src/routes/(app)/+page.svelte
git commit -m "feat: add savings rate widget to dashboard"
```

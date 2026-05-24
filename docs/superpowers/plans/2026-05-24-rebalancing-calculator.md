# Portfolio Rebalancing Calculator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a rebalancing calculator panel to the allocation page that tells the user how much to buy or sell in each target group given a deposit or withdrawal amount.

**Architecture:** Pure frontend addition to `src/routes/(app)/assets/allocation/+page.svelte`. No backend changes. The `allocationTargets` variable is lifted from `onMount` scope to module level, `deposit` input drives reactive `rebalanced` computation, results shown in a Bulma table.

**Tech Stack:** SvelteKit (reactive declarations), lodash, existing `formatCurrency` / `formatPercentage` helpers and `COLORS` constants, Bulma CSS table.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `src/routes/(app)/assets/allocation/+page.svelte` | Modify | Lift `allocationTargets` to module scope; add `deposit` input and `rebalanced` reactive; add calculator section |

---

## Task 1: Add rebalancing calculator to the allocation page

**Files:**
- Modify: `src/routes/(app)/assets/allocation/+page.svelte`

Read the current file before editing: it lives at `src/routes/(app)/assets/allocation/+page.svelte`.

Key facts about the existing page:
- `total: number` — already a module-level variable, sum of all asset market values
- `allocationTargets` — currently declared as a `const` inside `onMount`; must be lifted to `let allocationTargets: AllocationTarget[] = []` at module level and assigned inside `onMount`
- `AllocationTarget` type (from `$lib/utils`): `{ name: string, target: number, current: number, aggregates: { [key: string]: Aggregate } }`
- `Aggregate` type: `{ market_amount: number, ... }`
- `formatCurrency` and `formatPercentage` are already imported from `$lib/utils`
- `COLORS` is already imported from `$lib/colors`
- `_` (lodash) is already imported

- [ ] **Step 1: Lift `allocationTargets` to module scope and add `deposit`**

First, add `AllocationTarget` to the existing import from `$lib/utils`. Find the line:
```typescript
import { ajax, formatPercentage, rem, type Aggregate, type Legend } from "$lib/utils";
```
Change it to:
```typescript
import { ajax, formatPercentage, rem, type Aggregate, type AllocationTarget, type Legend } from "$lib/utils";
```

Then add two new variables after `let total = 0;`:
```typescript
let allocationTargets: AllocationTarget[] = [];
let deposit = 0;
```

In `onMount`, replace the destructuring:
```typescript
const {
  aggregates: aggregates,
  aggregates_timeline: aggregatesTimeline,
  allocation_targets: allocationTargets
} = await ajax("/api/allocation");
```
with:
```typescript
const {
  aggregates: aggregates,
  aggregates_timeline: aggregatesTimeline,
  allocation_targets: targets
} = await ajax("/api/allocation");
allocationTargets = targets;
```

- [ ] **Step 2: Add the `rebalanced` reactive declaration**

After the existing `let depth = 2;` line, add:

```typescript
$: rebalanced = allocationTargets.map((at) => {
  const currentAmount = _.sumBy(_.values(at.aggregates), (a) => a.market_amount);
  const newTotal = total + deposit;
  const targetAmount = newTotal * (at.target / 100);
  const rawDelta = targetAmount - currentAmount;
  const delta = Math.max(rawDelta, -currentAmount);
  const currentPercent = total > 0 ? (currentAmount / total) * 100 : 0;
  const tolerance = total * 0.005;
  return { name: at.name, currentAmount, currentPercent, targetPercent: at.target, delta, tolerance };
});
```

- [ ] **Step 3: Add the calculator section to the template**

After the closing `</section>` of the last existing section (the "Allocation Table" section, which ends with `<BoxLabel text="Allocation Table" />`), add:

```svelte
<section class="section tab-allocation" class:is-hidden={!showAllocation}>
  <div class="container is-fluid">
    <div class="columns">
      <div class="column is-12">
        <div class="box">
          <div class="field mb-4" style="max-width: 320px">
            <label class="label">Amount to invest (negative to withdraw)</label>
            <div class="control">
              <input class="input" type="number" bind:value={deposit} />
            </div>
          </div>
          <table class="table is-fullwidth is-hoverable">
            <thead>
              <tr>
                <th>Group</th>
                <th class="has-text-right">Current</th>
                <th class="has-text-right">Target</th>
                <th class="has-text-right">Action</th>
              </tr>
            </thead>
            <tbody>
              {#each rebalanced as row}
                <tr>
                  <td>{row.name}</td>
                  <td class="has-text-right">
                    {formatCurrency(row.currentAmount)}
                    <span class="has-text-grey is-size-7"
                      >({formatPercentage(row.currentPercent / 100, 1)})</span
                    >
                  </td>
                  <td class="has-text-right">{formatPercentage(row.targetPercent / 100, 1)}</td>
                  <td class="has-text-right">
                    {#if Math.abs(row.delta) > row.tolerance}
                      <span style="color: {row.delta > 0 ? COLORS.gainText : COLORS.lossText}">
                        {row.delta > 0 ? "Buy" : "Sell"}
                        {formatCurrency(Math.abs(row.delta))}
                      </span>
                    {:else}
                      <span class="has-text-grey">—</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    </div>
    <BoxLabel text="Rebalancing Calculator" />
  </div>
</section>
```

Note: The existing allocation sections use `class="section tab-allocation"` without a hidden guard because they're inside the `style={showAllocation ? "" : "display: none"}` on the first section. The new section needs its own `class:is-hidden={!showAllocation}` since it's outside that first section.

- [ ] **Step 4: Run TypeScript check**

```bash
npx svelte-kit sync && npx tsc --noEmit 2>&1 | grep "error TS" | grep -v node_modules
```

Expected: no output. If you see `allocationTargets` type errors, ensure `AllocationTarget` is imported from `$lib/utils` (it should already be — check the existing imports at the top of the script block).

- [ ] **Step 5: Run Prettier**

```bash
npx prettier --write "src/routes/(app)/assets/allocation/+page.svelte"
```

- [ ] **Step 6: Verify in browser**

Start dev server: `npm run dev`

Open `/assets/allocation`. If you have `allocation_targets` configured in your paisa config, you'll see the "Rebalancing Calculator" section at the bottom.

Verify:
- With `deposit = 0`: groups that have drifted from target show Buy/Sell, groups within 0.5% show `—`
- Typing a positive deposit amount: under-weight groups' Buy amounts increase
- Typing a negative deposit amount: over-weight groups show Sell amounts
- The table is hidden when no allocation targets are configured

- [ ] **Step 7: Commit**

```bash
git add "src/routes/(app)/assets/allocation/+page.svelte"
git commit -m "feat: add rebalancing calculator to allocation page"
```

# Portfolio Rebalancing Calculator — Design Spec

**Date:** 2026-05-24
**Enhancement:** Enh 4
**Status:** Approved

---

## Overview

Add a rebalancing calculator panel to the existing allocation page (`/assets/allocation`). Given a deposit or withdrawal amount, compute how much to buy or sell in each allocation target group to get back to the configured target percentages. No backend changes — all data is already returned by `/api/allocation`.

---

## Location

A new `<section class="section tab-allocation">` added at the bottom of `src/routes/(app)/assets/allocation/+page.svelte`, after the "Allocation Table" section. Hidden (via `showAllocation`) when no allocation targets are configured.

---

## Input

A single `<input type="number">` labelled "Amount to invest (negative to withdraw)", defaulting to `0`. Reactive — results update as the user types with no submit button.

---

## Computation

All client-side. Inputs already available on the page after `onMount`:
- `total`: sum of all asset market values (already computed)
- `allocationTargets: AllocationTarget[]`: fetched from `/api/allocation`, must be lifted to module-level variable (currently scoped to `onMount`)
- `deposit`: user input (number, default 0)

For each `AllocationTarget` group:

```
currentAmount = sum(group.aggregates[*].market_amount)
newTotal      = total + deposit
targetAmount  = newTotal × (group.target / 100)
delta         = targetAmount − currentAmount
```

Capping: `delta = max(delta, -currentAmount)` — cannot sell more than you own.

Tolerance: if `Math.abs(delta) ≤ total * 0.005` (0.5% of portfolio), treat as on-target. When `total === 0`, skip tolerance check.

---

## Output Table

Columns: **Group | Current | Target % | Action**

- **Current**: `formatCurrency(currentAmount)` + `(currentPercent%)` in grey
- **Target %**: `formatPercentage(group.target / 100, 1)`
- **Action**:
  - `delta > tolerance` → `Buy ₹X` in `COLORS.gainText` green
  - `delta < -tolerance` → `Sell ₹X` in `COLORS.lossText` red
  - otherwise → `—` in grey

---

## Edge Cases

| Scenario | Behaviour |
|---|---|
| `deposit = 0` | Shows pure drift correction — no new money, just rebalance |
| New portfolio (`total = 0`) | All groups show Buy; tolerance skipped |
| Withdrawal exceeds group value | Delta capped at `-currentAmount` (sell all) |
| No allocation targets configured | Section hidden via existing `showAllocation` flag |

# Savings Rate Dashboard Widget — Design Spec

**Date:** 2026-05-23  
**Enhancement:** Enh 7  
**Status:** Approved

---

## Overview

Add two `LevelItem` tiles to the existing Assets metrics block on the dashboard showing:
1. **Savings Rate** — current month's savings rate with a ↑↓ trend indicator vs the 12-month average
2. **12m Avg** — pooled trailing 12-month savings rate

No backend changes required. All data is already available in the `cashFlows` array returned by `/api/dashboard`.

---

## Formula

**Savings Rate** = `(Income − Expenses) / Income × 100`

**Current month**: computed from the `CashFlow` entry whose `.date` matches the current month.

**12m avg (pooled)**: `(Σincome − Σexpenses) / Σincome × 100` across the last 12 `CashFlow` entries (or fewer if less history exists). Pooled rather than averaged-of-rates to avoid distortion from low-income months.

**Trend indicator**: compare current month rate to 12m avg:
- Current > 12m avg → ↑ (green)
- Current < 12m avg → ↓ (red)
- Equal or insufficient data for comparison → no arrow

---

## UI Placement

Inside the existing `{#if networth}` block in `src/routes/(app)/+page.svelte`, add a third `<nav class="level grid-2">` row after the Gain/Loss + XIRR row:

```
[ Net Worth ]        [ Net Investment ]
[ Gain / Loss ]      [ XIRR           ]
[ Savings Rate ↑ ]   [ 12m Avg        ]   ← new row
```

Both tiles use the existing `LevelItem` component with `narrow` prop. The trend arrow is rendered inline as part of the `value` string passed to `LevelItem`.

---

## Data Flow

1. `/api/dashboard` already returns `cashFlows: CashFlow[]` (each entry: `{ date, income, expenses, ... }`)
2. `+page.svelte` already stores this in `cashFlows`
3. Two new reactive derivations compute `currentSavingsRate` and `avg12mSavingsRate` from `cashFlows`
4. A third derivation computes `savingsTrend: "up" | "down" | "neutral"`
5. The formatted value string passed to `LevelItem` is e.g. `"38% ↑"` or `"—"` if income is zero

---

## Edge Cases

| Scenario | Behaviour |
|---|---|
| Current month income = 0 | Show `—` for Savings Rate; omit trend arrow |
| Fewer than 2 months of data | Show rate without trend arrow |
| All `cashFlows` empty | Entire row hidden inside existing `{#if networth}` guard |
| Negative savings rate (expenses > income) | Show negative %, colored red (same as Gain/Loss loss color) |

---

## Files Changed

| File | Change |
|---|---|
| `src/routes/(app)/+page.svelte` | Add computed savings rate values; add third `<nav class="level grid-2">` row with two `LevelItem`s |

No backend files changed. No new components.

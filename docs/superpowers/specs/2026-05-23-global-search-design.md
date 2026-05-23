# Global Transaction Search — Design Spec

**Date:** 2026-05-23
**Enhancement:** Enh 8
**Status:** Approved

---

## Overview

Add a global search bar to the navbar that lets users find transactions by payee, account, or note. Results appear on a dedicated `/search` page as a flat list of matching transactions. No backend changes required — filtering is done client-side on the existing `/api/transaction` payload.

---

## User Flow

1. User types in the navbar search input
2. After 300ms debounce, URL updates to `/search?q=<query>` via SvelteKit `goto`
3. If already on `/search`, the reactive filter updates in place — no re-fetch
4. If on another page, SvelteKit navigates to `/search?q=<query>`
5. Direct URL load (`/search?q=swiggy`) pre-populates the input and filters immediately

---

## Filter Logic

Case-insensitive substring match. A transaction matches if:
- `transaction.payee` contains `q`, **or**
- any `posting.account` contains `q`, **or**
- any `posting.note` contains `q`

All comparisons use `.toLowerCase().includes(q.toLowerCase())`.

---

## Files

| File | Type | Responsibility |
|---|---|---|
| `src/lib/components/NavSearch.svelte` | New | Search input in navbar; debounces input and calls `goto('/search?q=...')` |
| `src/routes/(app)/search/+page.ts` | New | SvelteKit load function; reads `q` from `url.searchParams` |
| `src/routes/(app)/search/+page.svelte` | New | Fetches `/api/transaction` once; filters reactively; renders results |

No backend files changed. No new API endpoints.

---

## Component Details

### `NavSearch.svelte`

- Plain `<input type="search">` styled to match the navbar (Bulma `input is-small`)
- Positioned in `navbar-end`, between `ThemeSwitcher` and `Actions`
- On input: debounce 300ms → `goto('/search?q=' + encodeURIComponent(value), { replaceState: true })`
- On mount: read current URL's `q` param to pre-populate if landing on `/search` directly
- Pressing Escape clears the input and navigates back (or clears `q` param)

### `+page.ts` (load function)

```typescript
export function load({ url }) {
  return { q: url.searchParams.get("q") ?? "" };
}
```

### `+page.svelte`

- On mount: fetch `/api/transaction` → store `transactions: Transaction[]`
- Receive `q` from load data; keep in sync with `$page.url.searchParams` reactively
- `$: filtered = q.trim() ? filter(transactions, q) : []`
- Renders:
  - Empty query → "Type to search" placeholder
  - Results found → `<N> transaction(s) found` count + list of `TransactionCard`
  - No results → "No transactions found for `<q>`"

---

## Edge Cases

| Scenario | Behaviour |
|---|---|
| Empty query | Show "Type to search" — do not dump all transactions |
| No matches | Show "No transactions found for `<q>`" |
| Direct URL `/search?q=swiggy` | Input pre-populated, results shown immediately |
| Very short query (1 char) | Still filters — no minimum length |
| `q` param removed from URL | Input clears, back to empty state |

---

## Navbar Wiring

`Navbar.svelte` imports `NavSearch` and places it in `navbar-end` between `ThemeSwitcher` and `Actions`. The existing layout is unchanged for all other pages.

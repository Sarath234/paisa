# Global Transaction Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a global search bar to the navbar that lets users find transactions by payee, account, or note, with results on a dedicated `/search` page updated live as the user types.

**Architecture:** Pure frontend — `NavSearch.svelte` debounces input and navigates to `/search?q=<query>`. The search page fetches `/api/transaction` once on mount, then filters client-side reactively. No backend changes. Results rendered using the existing `TransactionCard` component.

**Tech Stack:** SvelteKit (`goto`, `$page` store, `PageLoad`), lodash (`_.debounce`), existing `TransactionCard` component, Bulma CSS.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `src/lib/components/NavSearch.svelte` | Create | Search input; debounces and calls `goto('/search?q=...')` |
| `src/lib/components/Navbar.svelte` | Modify | Import NavSearch; add it to navbar-end |
| `src/routes/(app)/search/+page.ts` | Create | SvelteKit load function; reads `q` from URL params |
| `src/routes/(app)/search/+page.svelte` | Create | Fetches transactions once; filters reactively; renders results |

---

## Task 1: Create NavSearch component

**Files:**
- Create: `src/lib/components/NavSearch.svelte`

- [ ] **Step 1: Create the component**

```svelte
<script lang="ts">
  import { goto } from "$app/navigation";
  import { page } from "$app/stores";
  import _ from "lodash";

  let value = "";

  // Keep input in sync when navigating with browser back/forward
  $: {
    const q = $page.url.searchParams.get("q") ?? "";
    if (q !== value) value = q;
  }

  const debouncedNavigate = _.debounce((q: string) => {
    const url = q ? `/search?q=${encodeURIComponent(q)}` : "/search";
    goto(url, { replaceState: true, keepFocus: true });
  }, 300);

  function handleInput(e: Event) {
    value = (e.target as HTMLInputElement).value;
    debouncedNavigate(value);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      value = "";
      debouncedNavigate.cancel();
      goto("/search", { replaceState: true, keepFocus: true });
    }
  }
</script>

<input
  class="input is-small"
  type="search"
  placeholder="Search transactions…"
  style="width: 200px"
  {value}
  on:input={handleInput}
  on:keydown={handleKeydown}
/>
```

- [ ] **Step 2: Run TypeScript check**

```bash
npx svelte-kit sync && npx tsc --noEmit 2>&1 | grep "error TS" | grep -v node_modules
```

Expected: no output (no errors).

- [ ] **Step 3: Commit**

```bash
git add src/lib/components/NavSearch.svelte
git commit -m "feat: add NavSearch component with debounced navigation"
```

---

## Task 2: Wire NavSearch into Navbar

**Files:**
- Modify: `src/lib/components/Navbar.svelte`

The navbar-end block (around line 296) has a `<div class="field is-grouped">` containing the quick-entry button, ThemeSwitcher, and Actions. Add NavSearch as a new `<p class="control">` before ThemeSwitcher.

- [ ] **Step 1: Add the import**

At the top of `<script>` in `Navbar.svelte`, after the existing imports, add:

```typescript
  import NavSearch from "$lib/components/NavSearch.svelte";
```

- [ ] **Step 2: Add NavSearch to navbar-end**

Find the block:
```svelte
          <p class="control">
            <ThemeSwitcher />
          </p>
```

Add a new `<p class="control">` immediately before it:

```svelte
          <p class="control">
            <NavSearch />
          </p>
          <p class="control">
            <ThemeSwitcher />
          </p>
```

- [ ] **Step 3: Run TypeScript check**

```bash
npx tsc --noEmit 2>&1 | grep "error TS" | grep -v node_modules
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add src/lib/components/Navbar.svelte
git commit -m "feat: add NavSearch to navbar"
```

---

## Task 3: Create search route load function

**Files:**
- Create: `src/routes/(app)/search/+page.ts`

- [ ] **Step 1: Create the load function**

```typescript
import type { PageLoad } from "./$types";

export const load = (({ url }) => {
  return { q: url.searchParams.get("q") ?? "" };
}) satisfies PageLoad;
```

- [ ] **Step 2: Run TypeScript check**

```bash
npx svelte-kit sync && npx tsc --noEmit 2>&1 | grep "error TS" | grep -v node_modules
```

Expected: no output. (`svelte-kit sync` generates `$types` for the new route.)

- [ ] **Step 3: Commit**

```bash
git add src/routes/\(app\)/search/+page.ts
git commit -m "feat: add search route load function"
```

---

## Task 4: Create search results page

**Files:**
- Create: `src/routes/(app)/search/+page.svelte`

`ajax("/api/transaction")` returns `{ transactions: Transaction[] }` — typed overload already exists in `src/lib/utils.ts:570`.

`TransactionCard` takes a single prop `t: Transaction` (`src/lib/components/TransactionCard.svelte:15`).

- [ ] **Step 1: Create the page**

```svelte
<script lang="ts">
  import { page } from "$app/stores";
  import { ajax, type Transaction } from "$lib/utils";
  import TransactionCard from "$lib/components/TransactionCard.svelte";
  import { MasonryGrid } from "@egjs/svelte-grid";
  import _ from "lodash";
  import { onMount } from "svelte";

  let UntypedMasonryGrid = MasonryGrid as any;
  let transactions: Transaction[] = [];
  let loaded = false;

  $: q = $page.url.searchParams.get("q") ?? "";

  $: filtered = (() => {
    if (!q.trim()) return [];
    const lower = q.toLowerCase();
    return transactions.filter(
      (t) =>
        t.payee.toLowerCase().includes(lower) ||
        t.postings.some(
          (p) =>
            p.account.toLowerCase().includes(lower) ||
            (p.note ?? "").toLowerCase().includes(lower)
        )
    );
  })();

  onMount(async () => {
    ({ transactions } = await ajax("/api/transaction"));
    loaded = true;
  });
</script>

<section class="section">
  <div class="container is-fluid">
    {#if !q.trim()}
      <p class="has-text-grey">Type to search transactions by payee, account, or note.</p>
    {:else if !loaded}
      <p class="has-text-grey">Loading…</p>
    {:else if filtered.length === 0}
      <p class="has-text-grey">No transactions found for <strong>{q}</strong>.</p>
    {:else}
      <p class="mb-3 has-text-grey is-size-7">
        <strong>{filtered.length}</strong> transaction(s) found
      </p>
      <UntypedMasonryGrid gap={10} maxStretchColumnSize={500} align="stretch">
        {#each filtered as t (t.id)}
          <div class="mr-3 is-flex-grow-1">
            <TransactionCard {t} />
          </div>
        {/each}
      </UntypedMasonryGrid>
    {/if}
  </div>
</section>
```

- [ ] **Step 2: Run TypeScript check**

```bash
npx svelte-kit sync && npx tsc --noEmit 2>&1 | grep "error TS" | grep -v node_modules
```

Expected: no output.

- [ ] **Step 3: Run Prettier**

```bash
npx prettier --write "src/routes/(app)/search/+page.svelte" "src/lib/components/NavSearch.svelte"
```

Expected: files formatted, no errors.

- [ ] **Step 4: Verify manually in the browser**

```bash
npm run dev
```

Open the app. Verify:
- Search input is visible in the navbar
- Typing "swiggy" navigates to `/search?q=swiggy` and shows matching transactions
- Typing in a new term updates results live (no page reload)
- Empty input shows "Type to search" message
- Non-matching term shows "No transactions found for X"
- Pressing Escape clears the input
- Loading `/search?q=swiggy` directly pre-populates the input

- [ ] **Step 5: Commit**

```bash
git add "src/routes/(app)/search/+page.svelte"
git commit -m "feat: add global transaction search results page"
```

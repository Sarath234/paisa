# Quick Transaction Entry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a global `+` button and `N` shortcut that opens a modal form for adding ledger transactions from anywhere in the app.

**Architecture:** New `QuickEntryModal.svelte` component handles all form state and save logic. `Navbar.svelte` dispatches a `quickentry` event when the `+` button is clicked. `+layout.svelte` mounts the modal outside the `{#key $willRefresh}` block and registers the `N` keyboard shortcut. A new `appendTransaction` helper in `editor.ts` handles the read-then-append-then-save flow using existing endpoints.

**Tech Stack:** Svelte, TypeScript, svelte-select (already a dependency), bulma-toast, existing `/api/editor/files`, `/api/editor/file`, `/api/editor/save` endpoints.

---

## File Structure

| File | Change |
|------|--------|
| `src/lib/editor.ts` | Add `appendTransaction(filename, text)` |
| `src/lib/components/QuickEntryModal.svelte` | New — full modal component |
| `src/lib/components/Navbar.svelte` | Add `+` button, dispatch `quickentry` event |
| `src/routes/(app)/+layout.svelte` | Mount modal, wire event, register `N` shortcut |

---

## Task 1: Add `appendTransaction` to `src/lib/editor.ts`

**Files:**
- Modify: `src/lib/editor.ts`

- [ ] **Step 1: Add the function at the bottom of `src/lib/editor.ts`**

```typescript
export async function appendTransaction(filename: string, transactionText: string): Promise<void> {
  const { file } = await ajax("/api/editor/file", {
    method: "POST",
    body: JSON.stringify({ name: filename })
  });
  const newContent = file.content.trimEnd() + "\n\n" + transactionText + "\n";
  const result = await ajax("/api/editor/save", {
    method: "POST",
    body: JSON.stringify({ name: filename, content: newContent, operation: "overwrite" })
  });
  if (!result.saved) {
    throw new Error(result.message || "Failed to save");
  }
}
```

Also add `ajax` to the imports at the top of `editor.ts` if not already present:

```typescript
import { ajax } from "$lib/utils";
```

- [ ] **Step 2: Type-check**

```bash
npx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -E "Error|error" | head -20
```

Expected: no new errors.

- [ ] **Step 3: Commit**

```bash
git add src/lib/editor.ts
git commit -m "feat: add appendTransaction helper to editor.ts"
```

---

## Task 2: Create `QuickEntryModal.svelte`

**Files:**
- Create: `src/lib/components/QuickEntryModal.svelte`

- [ ] **Step 1: Create the file with the full component**

```svelte
<script lang="ts">
  import { onMount } from "svelte";
  import Select from "svelte-select";
  import Modal from "$lib/components/Modal.svelte";
  import { ajax, type LedgerFile } from "$lib/utils";
  import { appendTransaction } from "$lib/editor";
  import { format } from "$lib/journal";
  import * as toast from "bulma-toast";
  import _ from "lodash";
  import dayjs from "dayjs";

  export let active = false;

  const STORAGE_KEY = "paisa:quickentry:lastFile";

  interface PostingRow {
    account: string;
    amount: string;
  }

  let files: LedgerFile[] = [];
  let accounts: string[] = [];
  let payees: string[] = [];
  let saving = false;
  let errorMsg = "";

  let selectedFileItem: { value: string; label: string } | null = null;
  let date = dayjs().format("YYYY/MM/DD");
  let payee = "";
  let postings: PostingRow[] = [
    { account: "", amount: "" },
    { account: "", amount: "" }
  ];
  // Parallel array of svelte-select selections for account fields
  let postingSelections: ({ value: string; label: string } | null)[] = [null, null];

  $: accountOptions = accounts.map((a) => ({ value: a, label: a }));
  $: payeeOptions = payees.map((p) => ({ value: p, label: p }));
  $: fileOptions = files.map((f) => ({ value: f.name, label: f.name }));

  // Persist last-selected file
  $: if (selectedFileItem) {
    localStorage.setItem(STORAGE_KEY, selectedFileItem.value);
  }

  $: valid =
    payee.trim().length > 0 &&
    selectedFileItem !== null &&
    postings.some((p) => p.amount.trim().length > 0);

  async function loadData() {
    const result = await ajax("/api/editor/files?metadata_only=true");

    // Sort by most recent backup descending; no-backup files go last
    files = _.orderBy(
      result.files,
      (f) => (f.versions.length > 0 ? f.versions[0] : ""),
      "desc"
    );
    accounts = result.accounts;
    payees = result.payees;

    applyDefaultFile();
  }

  function applyDefaultFile() {
    // 1. Restore last selection
    const last = localStorage.getItem(STORAGE_KEY);
    if (last && files.find((f) => f.name === last)) {
      selectedFileItem = { value: last, label: last };
      return;
    }

    // 2. Current month file (YYYYMM in path), excluding the root journal file
    const journalBasename = USER_CONFIG.journal_path.split("/").pop() ?? "";
    const yyyymm = dayjs().format("YYYYMM");
    const monthFile = files.find((f) => f.name.includes(yyyymm) && f.name !== journalBasename);
    if (monthFile) {
      selectedFileItem = { value: monthFile.name, label: monthFile.name };
      return;
    }

    // 3. Most recently edited non-root file
    const first = files.find((f) => f.name !== journalBasename);
    if (first) {
      selectedFileItem = { value: first.name, label: first.name };
    }
  }

  // Reload data and reset form fields (but keep file + date) when modal opens
  $: if (active) {
    errorMsg = "";
    loadData();
  }

  function addPosting() {
    postings = [...postings, { account: "", amount: "" }];
    postingSelections = [...postingSelections, null];
  }

  function removePosting(i: number) {
    postings = postings.filter((_, idx) => idx !== i);
    postingSelections = postingSelections.filter((_, idx) => idx !== i);
  }

  function updateAccount(i: number, value: string) {
    postings[i] = { ...postings[i], account: value };
    postingSelections[i] = value ? { value, label: value } : null;
    postings = postings;
    postingSelections = postingSelections;
  }

  function buildTransactionText(): string {
    const lines = [`${date} ${payee.trim()}`];
    for (const p of postings) {
      if (p.account.trim()) {
        const line = p.amount.trim()
          ? `    ${p.account.trim()}    ${p.amount.trim()}`
          : `    ${p.account.trim()}`;
        lines.push(line);
      }
    }
    return format(lines.join("\n"));
  }

  function resetPostings() {
    postings = [
      { account: "", amount: "" },
      { account: "", amount: "" }
    ];
    postingSelections = [null, null];
  }

  async function save(addAnother = false) {
    if (!valid || !selectedFileItem) return;
    saving = true;
    errorMsg = "";
    try {
      await appendTransaction(selectedFileItem.value, buildTransactionText());
      toast.toast({ message: `Added to ${selectedFileItem.value}`, type: "is-success" });
      if (addAnother) {
        payee = "";
        resetPostings();
      } else {
        active = false;
      }
    } catch (e: any) {
      errorMsg = e?.message || "Failed to save";
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:active width="min(680px, 100vw)">
  <svelte:fragment slot="head">
    <p class="modal-card-title">Add Transaction</p>
  </svelte:fragment>

  <svelte:fragment slot="body">
    {#if errorMsg}
      <div class="notification is-danger is-light mb-3 py-2">{errorMsg}</div>
    {/if}

    <!-- File picker -->
    <div class="field">
      <label class="label is-small">File</label>
      <div class="control">
        <Select
          items={fileOptions}
          bind:value={selectedFileItem}
          showChevron={true}
          searchable={true}
          clearable={false}
          floatingConfig={{ strategy: "fixed" }}
        />
      </div>
    </div>

    <!-- Date -->
    <div class="field">
      <label class="label is-small">Date</label>
      <div class="control">
        <input class="input is-small" type="text" bind:value={date} />
      </div>
    </div>

    <!-- Payee: plain text input + datalist (free-form entry is primary use case) -->
    <div class="field">
      <label class="label is-small">Payee</label>
      <div class="control">
        <input
          class="input is-small"
          type="text"
          placeholder="e.g. Swiggy, Amazon"
          bind:value={payee}
          list="quick-entry-payees"
        />
        <datalist id="quick-entry-payees">
          {#each payees as p}
            <option value={p} />
          {/each}
        </datalist>
      </div>
    </div>

    <!-- Posting rows -->
    {#each postings as posting, i}
      <div class="field is-grouped" style="align-items: flex-start">
        <!-- Account: svelte-select searchable dropdown -->
        <div class="control is-expanded">
          <Select
            items={accountOptions}
            value={postingSelections[i]}
            showChevron={false}
            searchable={true}
            clearable={true}
            placeholder="Account"
            floatingConfig={{ strategy: "fixed" }}
            on:change={(e) => updateAccount(i, e.detail?.value ?? "")}
            on:clear={() => updateAccount(i, "")}
          />
        </div>
        <!-- Amount -->
        <div class="control" style="width: 180px; flex-shrink: 0">
          <input
            class="input is-small"
            type="text"
            placeholder="e.g. 5000 INR"
            bind:value={posting.amount}
          />
        </div>
        <!-- Remove button (only when > 2 rows) -->
        {#if postings.length > 2}
          <div class="control">
            <button
              class="button is-small is-light is-danger"
              on:click={() => removePosting(i)}
              title="Remove posting"
            >
              <span class="icon is-small"><i class="fas fa-times" /></span>
            </button>
          </div>
        {/if}
      </div>
    {/each}

    <a class="is-size-7 has-text-link" role="button" tabindex="0" on:click={addPosting}>
      + Add posting
    </a>
  </svelte:fragment>

  <svelte:fragment slot="foot">
    <button class="button is-success" disabled={!valid || saving} on:click={() => save(false)}>
      {saving ? "Saving…" : "Save"}
    </button>
    <button class="button" disabled={!valid || saving} on:click={() => save(true)}>
      Save &amp; Add Another
    </button>
    <button class="button" on:click={() => (active = false)}>Cancel</button>
  </svelte:fragment>
</Modal>
```

- [ ] **Step 2: Type-check**

```bash
npx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -E "Error|error" | head -20
```

Expected: no new errors.

- [ ] **Step 3: Format**

```bash
npx prettier --write src/lib/components/QuickEntryModal.svelte
```

- [ ] **Step 4: Commit**

```bash
git add src/lib/components/QuickEntryModal.svelte
git commit -m "feat: add QuickEntryModal component for global transaction entry"
```

---

## Task 3: Wire up Navbar button, layout mounting, and `N` keyboard shortcut

**Files:**
- Modify: `src/lib/components/Navbar.svelte`
- Modify: `src/routes/(app)/+layout.svelte`

- [ ] **Step 1: Add `createEventDispatcher` and `+` button to `Navbar.svelte`**

At the top of the `<script>` block in `src/lib/components/Navbar.svelte`, add the dispatcher:

```typescript
import { createEventDispatcher } from "svelte";
const dispatch = createEventDispatcher();
```

In the `navbar-end` section, add the `+` button before the `<ThemeSwitcher />` control (and only when not readonly):

```svelte
{#if !readonly}
  <p class="control">
    <button
      class="button"
      on:click={() => dispatch("quickentry")}
      title="Add transaction (N)"
      aria-label="Add transaction"
    >
      <span class="icon">
        <i class="fas fa-plus" />
      </span>
    </button>
  </p>
{/if}
```

The full `navbar-end` section after the change:

```svelte
<div class="navbar-end" style="margin-right: 0.3em">
  <div class="navbar-item">
    <div class="field is-grouped">
      {#if readonly}
        <p class="control">
          <span
            class="mt-1 tag is-rounded is-danger is-light invertable"
            data-tippy-content="<p>Paisa is in readonly mode</p>">readonly</span
          >
        </p>
      {/if}

      {#if !readonly}
        <p class="control">
          <button
            class="button"
            on:click={() => dispatch("quickentry")}
            title="Add transaction (N)"
            aria-label="Add transaction"
          >
            <span class="icon">
              <i class="fas fa-plus" />
            </span>
          </button>
        </p>
      {/if}

      <p class="control">
        <ThemeSwitcher />
      </p>
      <p class="control">
        <Actions />
      </p>
    </div>
  </div>
</div>
```

- [ ] **Step 2: Mount modal and wire shortcut in `src/routes/(app)/+layout.svelte`**

Replace the full content of `src/routes/(app)/+layout.svelte` with:

```svelte
<script lang="ts">
  import { afterNavigate, beforeNavigate } from "$app/navigation";
  import { followCursor, delegate, hideAll } from "tippy.js";
  import _ from "lodash";
  import Spinner from "$lib/components/Spinner.svelte";
  import Navbar from "$lib/components/Navbar.svelte";
  import QuickEntryModal from "$lib/components/QuickEntryModal.svelte";
  import { willClearTippy, willRefresh } from "../../store";
  import { onMount } from "svelte";

  let isBurger: boolean = null;
  let quickEntryActive = false;

  function clearTippy() {
    hideAll();
  }

  function setupTippy() {
    delegate("body", {
      target: "[data-tippy-content]",
      theme: "light",
      onShow: (instance) => {
        const content = instance.reference.getAttribute("data-tippy-content");
        if (!_.isEmpty(content)) {
          instance.setContent(content);
        } else {
          return false;
        }
      },
      maxWidth: "none",
      delay: 0,
      allowHTML: true,
      followCursor: true,
      popperOptions: {
        modifiers: [
          {
            name: "flip",
            options: {
              fallbackPlacements: ["auto"]
            }
          }
        ]
      },
      plugins: [followCursor]
    });
  }

  willClearTippy.subscribe(clearTippy);
  beforeNavigate(clearTippy);
  willRefresh.subscribe(() => {
    clearTippy();
    setupTippy();
  });

  afterNavigate(() => {
    isBurger = null;
    setupTippy();
  });

  onMount(() => {
    function handleKeydown(e: KeyboardEvent) {
      if (e.key !== "n" && e.key !== "N") return;
      const target = e.target as HTMLElement;
      const tag = target?.tagName?.toLowerCase();
      if (
        tag === "input" ||
        tag === "textarea" ||
        tag === "select" ||
        target?.isContentEditable
      )
        return;
      if (USER_CONFIG.readonly) return;
      quickEntryActive = true;
    }
    window.addEventListener("keydown", handleKeydown);
    return () => window.removeEventListener("keydown", handleKeydown);
  });
</script>

{#key $willRefresh}
  <Navbar bind:isBurger on:quickentry={() => (quickEntryActive = true)} />

  <Spinner>
    <slot />
  </Spinner>
{/key}

<QuickEntryModal bind:active={quickEntryActive} />
```

- [ ] **Step 3: Type-check**

```bash
npx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -E "Error|error" | head -20
```

Expected: no new errors.

- [ ] **Step 4: Format**

```bash
npx prettier --write src/lib/components/Navbar.svelte src/routes/\(app\)/+layout.svelte
```

- [ ] **Step 5: Manual smoke test**

Start the dev server (`make serve` or `npm run dev` in `src/`) and verify:
- `+` button appears in top-right navbar (not present in readonly mode)
- Clicking `+` opens the modal
- Pressing `N` from the dashboard opens the modal
- Pressing `N` while a text input is focused does NOT open the modal
- File dropdown shows full relative paths, sorted with most recently edited first
- Current month file (`202605/transactions.ledger`) is pre-selected
- After saving, a success toast appears with the filename
- "Save & Add Another" keeps modal open, clears payee + postings
- Transaction is actually appended to the file (verify in editor)

- [ ] **Step 6: Commit**

```bash
git add src/lib/components/Navbar.svelte src/routes/\(app\)/+layout.svelte
git commit -m "feat: add global quick-entry button and N shortcut to navbar"
```

---

## Verification

After all tasks:

```bash
# Type check
npx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -c "Error"
# Expected: 0

# Go tests unaffected
go test ./internal/... 2>&1 | tail -5
# Expected: all ok
```

End-to-end: open app → press `N` → fill in date/payee/accounts/amounts → Save → open editor for that file → confirm transaction appears at the bottom, correctly formatted.

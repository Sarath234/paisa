# Quick Transaction Entry — Design Spec

## Goal

A global quick-entry modal that lets the user add a ledger transaction from anywhere in the app without navigating to the editor.

## Architecture

### New file
- **`src/lib/components/QuickEntryModal.svelte`** — self-contained modal: file picker, date, payee, posting rows, save logic.

### Modified files
- **`src/routes/(app)/+layout.svelte`** — add `+` button to global navbar; mount `QuickEntryModal`; register `N` keyboard shortcut.
- **`src/lib/editor.ts`** — add `appendTransaction(filename, text): Promise<void>` helper that GETs current file content, appends the formatted transaction, and POSTs via existing `/api/editor/save`.

No backend changes required. All existing endpoints are sufficient.

---

## Form Fields

| Field | Behaviour |
|-------|-----------|
| **File picker** | Dropdown of all journal files shown as full relative paths (e.g. `202605/transactions.ledger`). Sorted by most recent backup timestamp descending. |
| **Date** | Text input, pre-filled with today in `YYYY/MM/DD`. Editable. |
| **Payee** | `svelte-select` searchable dropdown from payees array. Supports typing a new payee not in the list. |
| **Posting rows** | Start with 2 rows. Each row: account field (`svelte-select` searchable dropdown from accounts, supports new account), amount field (free text, e.g. `5000 INR`), remove button (visible only when > 2 rows). |
| **Add posting** | Link below rows; appends a new empty posting row. |

The last posting's amount may be left blank — ledger infers the balance automatically.

---

## File Picker — Default Selection

Priority order on open:

1. Last-selected file from `localStorage` key `paisa:quickentry:lastFile`.
2. File whose relative path contains the current `YYYYMM` string (e.g. `202605`).
3. Most recently edited file (top of recency-sorted list).

`main.ledger` (or any file that matches the journal root path from config) is never auto-selected as default — it is an include-only file. It remains available in the dropdown for edge cases.

File recency is derived from the latest entry in each file's `versions[]` array. Backup filenames embed `YYYY-MM-DD-HH-MM-SS.mmm`, so lexicographic descending sort gives the most recent edit. Files with no backups sort to the bottom.

---

## Keyboard Shortcut

`N` key opens the modal from any page when focus is not inside an `input`, `textarea`, `select`, or `[contenteditable]` element. `Escape` closes.

---

## Generated Ledger Text

Built from form values, then passed through the existing `format()` function from `src/lib/journal.ts`:

```
2026/05/17 Groceries
    Expenses:Food                    5000 INR
    Assets:Checking
```

A blank line is prepended before the transaction block when appending to the file.

---

## Data Flow

### On modal open
1. `GET /api/editor/files?metadata_only=true` — returns file list with `versions[]`. (Lightweight; no file content.)
2. Sort files by recency, apply default selection logic.
3. Populate payees and accounts arrays into `svelte-select` components for payee and account fields.

### On Save
1. Build transaction string from form, run through `format()`.
2. `GET /api/editor/file` with `{ name: selectedFile }` — fetch current file content.
3. Append `\n\n` + formatted transaction.
4. `POST /api/editor/save` with `{ name, content, operation: "overwrite" }`.
5. Success: toast "Added to 202605/transactions.ledger", close modal (or reset form if "Save & Add Another").
6. Error: show inline error message, keep modal open.

### Save & Add Another
- Saves the transaction.
- Clears payee and all posting rows (resets to 2 empty rows).
- Keeps selected file and date unchanged.
- Keeps modal open.

---

## Edge Cases

- **File open in editor simultaneously**: editor shows stale content until the user reloads. No data loss — the save creates a backup automatically. No live sync attempted.
- **New month, no YYYYMM file yet**: falls back to most recently edited file. User can manually select the correct file once created.
- **Empty payee or all-empty amounts**: form validates that payee is non-empty and at least one amount is filled before enabling Save.

---

## Footer Buttons

| Button | Action |
|--------|--------|
| `Save` | Append, close modal, show success toast |
| `Save & Add Another` | Append, reset payee + postings, keep modal open |
| `Cancel` | Close modal without saving |

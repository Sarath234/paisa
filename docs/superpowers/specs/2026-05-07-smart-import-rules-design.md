# Smart Import Rules Engine — Design Spec

**Date:** 2026-05-07
**Status:** Approved

---

## Goal

Eliminate repeated manual account categorization on CSV imports. Users define named regex rules in `paisa.yaml`; the import page auto-fills a new `ACCOUNT` column in the CSV preview table, which templates reference as `{{ROW.ACCOUNT}}`.

---

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Rule scope | Global (all templates) | Account mappings are meaningful regardless of bank/template |
| Match target | All columns (full row text) | Bank CSVs have one text column in practice; keeps rules portable |
| Match surfacing | ACCOUNT column in UI preview | Visible feedback; template opt-in via `{{ROW.ACCOUNT}}` |
| Matching location | Client-side (browser) | Consistent with existing client-side template rendering; no round-trip |
| Storage | `paisa.yaml` via config CRUD | Same pattern as `import_templates`; no database needed |

---

## Data Model

### Config struct (`internal/config/config.go`)

```go
type ImportRule struct {
    Name    string `json:"name" yaml:"name"`
    Match   string `json:"match" yaml:"match"`
    Account string `json:"account" yaml:"account"`
}
```

Added to `Config`:

```go
ImportRules []ImportRule `json:"import_rules" yaml:"import_rules"`
```

### Example `paisa.yaml` entry

```yaml
import_rules:
  - name: "Swiggy food orders"
    match: "(?i)swiggy"
    account: "Expenses:Food:Delivery"
  - name: "Salary credit"
    match: "(?i)salary|payroll"
    account: "Income:Salary"
```

### Matching semantics

- Rules evaluated in declaration order; first match wins.
- Regex tested against all cell values in the row joined by a single space.
- Invalid regexes are caught client-side on blur and block saving; the rule is never persisted with a broken regex.
- Unmatched rows get `ACCOUNT: ""` — the template must handle the empty case (e.g. a fallback account or conditional).

---

## Backend

### New package: `internal/model/importrule/importrule.go`

Mirrors `internal/model/template/template.go` exactly:

```go
func All() []config.ImportRule
func Upsert(rule config.ImportRule)
func Delete(name string)
```

- `All()` — returns `config.GetConfig().ImportRules`.
- `Upsert()` — deletes by name then appends; calls `config.SaveConfigObject`.
- `Delete()` — filters by name; calls `config.SaveConfigObject`.

### New routes (`internal/server/server.go`)

```
GET  /api/import/rules            → { rules: ImportRule[] }
POST /api/import/rules/upsert     → body: ImportRule → { rule, saved: true }
POST /api/import/rules/delete     → body: { name } → { success: true }
```

All routes guard on `config.GetConfig().Readonly` and return `{ saved: false, message: "Readonly mode" }` when set.

---

## Frontend

### Matching logic (`src/lib/spreadsheet.ts`)

New exported function:

```typescript
export function applyRules(
  rows: Array<Record<string, any>>,
  rules: ImportRule[]
): Array<Record<string, any>> {
  return rows.map((row) => {
    const rowText = Object.values(row).join(" ");
    const matched = rules.find((r) => {
      try { return new RegExp(r.match, "i").test(rowText); }
      catch { return false; }
    });
    return { ...row, ACCOUNT: matched?.account ?? "" };
  });
}
```

Called in `+page.svelte` after `asRows()` and re-run whenever rules or uploaded data change.

### ACCOUNT column in preview table (`src/routes/(app)/ledger/import/+page.svelte`)

- The existing CSV preview table gains one column at the far right, header `ACCOUNT`.
- Cells with a matched account show the account name in a `tag is-success` badge.
- Empty cells (no match) are rendered blank.

### Rules management panel (`src/routes/(app)/ledger/import/+page.svelte`)

Collapsible section below the template editor in the left column, matching the existing toolbar box style:

- **Rule table:** Name | Regex | Account | Delete (trash icon). One row per rule. Order matches declaration order (first match wins).
- **Add rule form:** Three inline inputs — Name, Regex, Account — plus a Save button.
  - Account input uses the existing `accountTfIdf` store for autocomplete.
  - Regex is validated with `new RegExp(match)` on blur; invalid regex shows `is-danger help` text and disables Save.
- Any add/delete calls the upsert/delete API then re-fetches rules and re-runs `applyRules` on current rows.

### `src/lib/utils.ts` — new ajax overloads

```typescript
export function ajax(route: "/api/import/rules", options?: RequestOptions): Promise<{ rules: ImportRule[] }>;
export function ajax(route: "/api/import/rules/upsert", options?: RequestOptions): Promise<{ rule: ImportRule; saved: boolean }>;
export function ajax(route: "/api/import/rules/delete", options?: RequestOptions): Promise<{ success: boolean }>;
```

---

## TypeScript type

```typescript
export interface ImportRule {
  name: string;
  match: string;
  account: string;
}
```

Defined in `src/lib/utils.ts` alongside `ImportTemplate`.

---

## Testing

### Backend (Go)

- Unit test `applyRules`-equivalent logic in `importrule_test.go`: verify first-match-wins, case-insensitive match, empty result for no match, invalid regex is skipped.
- Test `Upsert` and `Delete` persist correctly to config.

### Frontend (manual smoke test)

- Upload a CSV → rows with matching descriptions show green ACCOUNT badge.
- Add a rule → immediately re-applies to current rows.
- Delete a rule → ACCOUNT column clears for previously matched rows.
- Invalid regex in Add form → Save button disabled, error shown.
- `{{ROW.ACCOUNT}}` in a template renders the matched account in the preview pane.
- Readonly mode → API returns error, UI shows toast.

---

## Out of scope

- Rule ordering drag-and-drop (declaration order is sufficient for v1).
- Per-template rules (global rules cover the use case).
- Backend regex validation endpoint (client-side catch is sufficient).
- Rule import/export.

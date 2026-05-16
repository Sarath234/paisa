# Smart Import Rules Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users define named regex rules in `paisa.yaml` that auto-fill a new `ACCOUNT` column in the import page's CSV preview, available as `{{ROW.ACCOUNT}}` in Handlebars templates.

**Architecture:** Rules are stored in `paisa.yaml` alongside `import_templates`, managed through a new `internal/model/importrule` package and three new API routes. Matching runs client-side in TypeScript (same location as template rendering). The import page gains an ACCOUNT column in the preview table and a rules CRUD panel below the template editor.

**Tech Stack:** Go (config YAML, gin routes), TypeScript/Svelte (frontend matching + UI), bun:test (frontend tests), go test (backend tests).

---

## Files

- Modify: `internal/config/config.go` — add `ImportRule` struct + `ImportRules []ImportRule` to `Config` + `defaultConfig`
- Modify: `internal/config/schema.json` — add `import_rules` array schema
- Create: `internal/model/importrule/importrule.go` — `All()`, `Upsert()`, `Delete()`
- Create: `internal/model/importrule/importrule_test.go` — unit tests for model functions
- Modify: `internal/server/server.go` — add 3 routes + import for new package
- Modify: `src/lib/utils.ts` — add `ImportRule` interface + 3 ajax overloads
- Modify: `src/lib/spreadsheet.ts` — add `applyRules()` function
- Create: `src/lib/spreadsheet_rules.test.ts` — bun unit tests for `applyRules`
- Modify: `src/routes/(app)/ledger/import/+page.svelte` — rules panel + ACCOUNT column

---

## Task 1: Add `ImportRule` to config and schema

**Files:**
- Modify: `internal/config/config.go:49-52` (after `ImportTemplate` struct)
- Modify: `internal/config/config.go:151` (after `ImportTemplates` field in `Config`)
- Modify: `internal/config/config.go:181` (in `defaultConfig`)
- Modify: `internal/config/schema.json:1034` (after `import_templates` block)

- [ ] **Step 1: Add the `ImportRule` struct to `config.go`**

In `internal/config/config.go`, add this struct immediately after the `ImportTemplate` struct (after line 52):

```go
type ImportRule struct {
	Name    string `json:"name" yaml:"name"`
	Match   string `json:"match" yaml:"match"`
	Account string `json:"account" yaml:"account"`
}
```

- [ ] **Step 2: Add `ImportRules` field to `Config` struct**

In the `Config` struct (around line 151, after `ImportTemplates`), add:

```go
ImportRules []ImportRule `json:"import_rules" yaml:"import_rules"`
```

- [ ] **Step 3: Add `ImportRules` to `defaultConfig`**

In `defaultConfig` (around line 181, after `ImportTemplates: []ImportTemplate{}`), add:

```go
ImportRules: []ImportRule{},
```

- [ ] **Step 4: Add `import_rules` to the JSON schema**

In `internal/config/schema.json`, find the closing `}` of the `import_templates` block (around line 1034) and add this block immediately after it:

```json
    "import_rules": {
      "type": "array",
      "default": [],
      "itemsUniqueProperties": ["name"],
      "items": {
        "type": "object",
        "ui:header": "name",
        "properties": {
          "name": {
            "type": "string",
            "description": "Name of the rule",
            "minLength": 1
          },
          "match": {
            "type": "string",
            "description": "Regex pattern to match against all row cells",
            "minLength": 1
          },
          "account": {
            "type": "string",
            "description": "Ledger account to assign when the pattern matches",
            "minLength": 1
          }
        },
        "required": ["name", "match", "account"],
        "additionalProperties": false
      }
    },
```

- [ ] **Step 5: Build to verify compilation**

```bash
cd /Users/sarath.m/workspace/work/paisa && go build github.com/ananthakumaran/paisa/internal/config/... 2>&1
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git -C /Users/sarath.m/workspace/work/paisa add internal/config/config.go internal/config/schema.json
git -C /Users/sarath.m/workspace/work/paisa commit -m "feat: add ImportRule to config struct and JSON schema"
```

---

## Task 2: Create the importrule model package

**Files:**
- Create: `internal/model/importrule/importrule.go`
- Create: `internal/model/importrule/importrule_test.go`

This package follows the exact pattern of `internal/model/template/template.go`.

- [ ] **Step 1: Write the failing test**

Create `internal/model/importrule/importrule_test.go`:

```go
package importrule

import (
	"os"
	"testing"

	"github.com/ananthakumaran/paisa/internal/config"
	"github.com/stretchr/testify/assert"
)

func setupConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	configPath := dir + "/paisa.yaml"
	err := os.WriteFile(configPath, []byte("journal_path: /tmp/test.ledger\n"), 0644)
	assert.NoError(t, err)
	config.LoadConfigFile(configPath)
}

func TestAll_Empty(t *testing.T) {
	setupConfig(t)
	rules := All()
	assert.Empty(t, rules)
}

func TestUpsertAndAll(t *testing.T) {
	setupConfig(t)
	rule := config.ImportRule{Name: "Swiggy", Match: "swiggy", Account: "Expenses:Food"}
	Upsert(rule)
	rules := All()
	assert.Len(t, rules, 1)
	assert.Equal(t, "Swiggy", rules[0].Name)
	assert.Equal(t, "swiggy", rules[0].Match)
	assert.Equal(t, "Expenses:Food", rules[0].Account)
}

func TestUpsert_UpdateExisting(t *testing.T) {
	setupConfig(t)
	Upsert(config.ImportRule{Name: "Swiggy", Match: "swiggy", Account: "Expenses:Food"})
	Upsert(config.ImportRule{Name: "Swiggy", Match: "swiggy.*food", Account: "Expenses:Food:Delivery"})
	rules := All()
	assert.Len(t, rules, 1)
	assert.Equal(t, "swiggy.*food", rules[0].Match)
	assert.Equal(t, "Expenses:Food:Delivery", rules[0].Account)
}

func TestDelete(t *testing.T) {
	setupConfig(t)
	Upsert(config.ImportRule{Name: "Swiggy", Match: "swiggy", Account: "Expenses:Food"})
	Upsert(config.ImportRule{Name: "Salary", Match: "salary", Account: "Income:Salary"})
	Delete("Swiggy")
	rules := All()
	assert.Len(t, rules, 1)
	assert.Equal(t, "Salary", rules[0].Name)
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/model/importrule/... -v 2>&1 | head -20
```

Expected: FAIL — package `importrule` does not exist yet.

- [ ] **Step 3: Implement `importrule.go`**

Create `internal/model/importrule/importrule.go`:

```go
package importrule

import (
	"github.com/ananthakumaran/paisa/internal/config"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
)

func All() []config.ImportRule {
	return config.GetConfig().ImportRules
}

func Upsert(rule config.ImportRule) config.ImportRule {
	Delete(rule.Name)
	cfg := config.GetConfig()
	cfg.ImportRules = append(cfg.ImportRules, rule)
	if err := config.SaveConfigObject(cfg); err != nil {
		log.Fatal(err)
	}
	return rule
}

func Delete(name string) {
	cfg := config.GetConfig()
	cfg.ImportRules = lo.Filter(cfg.ImportRules, func(r config.ImportRule, _ int) bool {
		return r.Name != name
	})
	if err := config.SaveConfigObject(cfg); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/model/importrule/... -v 2>&1
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/sarath.m/workspace/work/paisa add internal/model/importrule/
git -C /Users/sarath.m/workspace/work/paisa commit -m "feat: add importrule model package with All/Upsert/Delete"
```

---

## Task 3: Add API routes to server.go

**Files:**
- Modify: `internal/server/server.go` — import block + 3 new routes after the templates routes (line ~359)

- [ ] **Step 1: Add the `importrule` import to server.go**

In `internal/server/server.go`, find the import block (around line 15). Add the importrule package after the template import:

```go
"github.com/ananthakumaran/paisa/internal/model/importrule"
```

The import block should look like:

```go
import (
    // ... existing imports ...
    "github.com/ananthakumaran/paisa/internal/model/template"
    "github.com/ananthakumaran/paisa/internal/model/importrule"
    // ... rest of imports ...
)
```

- [ ] **Step 2: Add the 3 routes after the templates routes**

In `internal/server/server.go`, find the end of the `templates/delete` route block (around line 359) and add immediately after it:

```go
	router.GET("/api/import/rules", func(c *gin.Context) {
		c.JSON(200, gin.H{"rules": importrule.All()})
	})

	router.POST("/api/import/rules/upsert", func(c *gin.Context) {
		if config.GetConfig().Readonly {
			c.JSON(200, gin.H{"saved": false, "message": "Readonly mode"})
			return
		}
		var rule config.ImportRule
		if err := c.ShouldBindJSON(&rule); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"rule": importrule.Upsert(rule), "saved": true})
	})

	router.POST("/api/import/rules/delete", func(c *gin.Context) {
		if config.GetConfig().Readonly {
			c.JSON(200, gin.H{"success": false, "message": "Readonly mode"})
			return
		}
		var rule config.ImportRule
		if err := c.ShouldBindJSON(&rule); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		importrule.Delete(rule.Name)
		c.JSON(200, gin.H{"success": true})
	})
```

- [ ] **Step 3: Build to confirm compilation**

```bash
cd /Users/sarath.m/workspace/work/paisa && go build github.com/ananthakumaran/paisa/internal/... 2>&1 | grep -v "no matching files"
```

Expected: no errors.

- [ ] **Step 4: Run full Go test suite**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/... 2>&1 | grep -E "FAIL|ok|---"
```

Expected: no new failures.

- [ ] **Step 5: Commit**

```bash
git -C /Users/sarath.m/workspace/work/paisa add internal/server/server.go
git -C /Users/sarath.m/workspace/work/paisa commit -m "feat: add /api/import/rules GET/upsert/delete routes"
```

---

## Task 4: Add `ImportRule` type and ajax overloads to `src/lib/utils.ts`

**Files:**
- Modify: `src/lib/utils.ts:483` — add `ImportRule` interface after `ImportTemplate`
- Modify: `src/lib/utils.ts:686` — add 3 ajax overloads after the templates overloads

- [ ] **Step 1: Add the `ImportRule` interface**

In `src/lib/utils.ts`, find the `ImportTemplate` interface (line 483):

```typescript
export interface ImportTemplate {
  id: string;
  name: string;
  content: string;
  template_type: string;
}
```

Add the `ImportRule` interface immediately after it:

```typescript
export interface ImportRule {
  name: string;
  match: string;
  account: string;
}
```

- [ ] **Step 2: Add the 3 ajax overloads**

In `src/lib/utils.ts`, find the templates ajax overloads (around line 675–686):

```typescript
export function ajax(
  route: "/api/templates/delete",
  options?: RequestOptions
): Promise<{ success: boolean; message?: string }>;
```

Add the following 3 overloads immediately after:

```typescript
export function ajax(
  route: "/api/import/rules",
  options?: RequestOptions
): Promise<{ rules: ImportRule[] }>;
export function ajax(
  route: "/api/import/rules/upsert",
  options?: RequestOptions
): Promise<{ rule: ImportRule; saved: boolean; message?: string }>;
export function ajax(
  route: "/api/import/rules/delete",
  options?: RequestOptions
): Promise<{ success: boolean; message?: string }>;
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd /Users/sarath.m/workspace/work/paisa && npx tsc --noEmit 2>&1 | head -30
```

Expected: no new errors (zero errors, or same pre-existing errors as before).

- [ ] **Step 4: Commit**

```bash
git -C /Users/sarath.m/workspace/work/paisa add src/lib/utils.ts
git -C /Users/sarath.m/workspace/work/paisa commit -m "feat: add ImportRule type and ajax overloads for import rules API"
```

---

## Task 5: Add `applyRules()` to `src/lib/spreadsheet.ts`

**Files:**
- Modify: `src/lib/spreadsheet.ts` — add `applyRules` function
- Create: `src/lib/spreadsheet_rules.test.ts` — bun unit tests

- [ ] **Step 1: Write the failing test**

Create `src/lib/spreadsheet_rules.test.ts`:

```typescript
import { describe, expect, test } from "bun:test";
import { applyRules } from "./spreadsheet";
import type { ImportRule } from "./utils";

describe("applyRules", () => {
  const rules: ImportRule[] = [
    { name: "Food delivery", match: "swiggy|zomato", account: "Expenses:Food:Delivery" },
    { name: "Salary", match: "(?i)salary", account: "Income:Salary" }
  ];

  test("matches first rule when pattern found anywhere in row", () => {
    const rows = [{ A: "SWIGGY ORDER 12345", B: "200", index: 0 }];
    const result = applyRules(rows, rules);
    expect(result[0].ACCOUNT).toBe("Expenses:Food:Delivery");
  });

  test("matches second rule when first does not match", () => {
    const rows = [{ A: "Monthly salary credit", B: "50000", index: 0 }];
    const result = applyRules(rows, rules);
    expect(result[0].ACCOUNT).toBe("Income:Salary");
  });

  test("sets empty string when no rule matches", () => {
    const rows = [{ A: "ATM withdrawal", B: "1000", index: 0 }];
    const result = applyRules(rows, rules);
    expect(result[0].ACCOUNT).toBe("");
  });

  test("first-match-wins when multiple rules could match", () => {
    const overlapping: ImportRule[] = [
      { name: "First", match: "credit", account: "Income:Other" },
      { name: "Second", match: "salary credit", account: "Income:Salary" }
    ];
    const rows = [{ A: "salary credit", B: "50000", index: 0 }];
    const result = applyRules(rows, overlapping);
    expect(result[0].ACCOUNT).toBe("Income:Other");
  });

  test("skips invalid regex without throwing", () => {
    const badRules: ImportRule[] = [
      { name: "Bad", match: "[invalid", account: "Expenses:Bad" }
    ];
    const rows = [{ A: "some text", B: "100", index: 0 }];
    const result = applyRules(rows, badRules);
    expect(result[0].ACCOUNT).toBe("");
  });

  test("matches against all columns concatenated", () => {
    const rows = [{ A: "2024-01-01", B: "REF123", C: "Swiggy", index: 0 }];
    const result = applyRules(rows, rules);
    expect(result[0].ACCOUNT).toBe("Expenses:Food:Delivery");
  });

  test("preserves existing row fields", () => {
    const rows = [{ A: "some text", B: "100", index: 0 }];
    const result = applyRules(rows, rules);
    expect(result[0].A).toBe("some text");
    expect(result[0].B).toBe("100");
    expect(result[0].index).toBe(0);
  });
});
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/sarath.m/workspace/work/paisa && bun test src/lib/spreadsheet_rules.test.ts 2>&1 | head -20
```

Expected: FAIL — `applyRules` is not exported from `./spreadsheet`.

- [ ] **Step 3: Implement `applyRules` in `src/lib/spreadsheet.ts`**

Add this function at the end of `src/lib/spreadsheet.ts`, before any closing exports:

```typescript
export function applyRules(
  rows: Array<Record<string, any>>,
  rules: Array<{ name: string; match: string; account: string }>
): Array<Record<string, any>> {
  return rows.map((row) => {
    const rowText = Object.values(row).join(" ");
    const matched = rules.find((r) => {
      try {
        return new RegExp(r.match, "i").test(rowText);
      } catch {
        return false;
      }
    });
    return { ...row, ACCOUNT: matched?.account ?? "" };
  });
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd /Users/sarath.m/workspace/work/paisa && bun test src/lib/spreadsheet_rules.test.ts 2>&1
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Run all frontend tests to check for regressions**

```bash
cd /Users/sarath.m/workspace/work/paisa && bun test 2>&1 | tail -10
```

Expected: no failures.

- [ ] **Step 6: Commit**

```bash
git -C /Users/sarath.m/workspace/work/paisa add src/lib/spreadsheet.ts src/lib/spreadsheet_rules.test.ts
git -C /Users/sarath.m/workspace/work/paisa commit -m "feat: add applyRules() to spreadsheet — matches CSV rows against import rules"
```

---

## Task 6: Update the import page — ACCOUNT column + rules panel

**Files:**
- Modify: `src/routes/(app)/ledger/import/+page.svelte`

This task wires up the frontend: loads rules at init, applies them to uploaded data, shows the ACCOUNT column in the preview, and adds the CRUD management panel.

- [ ] **Step 1: Add imports and state variables**

In `src/routes/(app)/ledger/import/+page.svelte`, find the `<script lang="ts">` block. After the existing imports, add:

```typescript
  import { applyRules } from "$lib/spreadsheet";
  import type { ImportRule } from "$lib/utils";
```

After the existing `let` declarations (after `let options = ...`), add:

```typescript
  let rules: ImportRule[] = [];
  let newRule: ImportRule = { name: "", match: "", account: "" };
  let newRuleRegexError: string = null;
  let accountList: string[] = [];
```

- [ ] **Step 2: Load rules at `onMount` and build account list**

In `onMount`, after `({ templates } = await ajax("/api/templates"));`, add:

```typescript
    ({ rules } = await ajax("/api/import/rules"));
    accountList = Object.keys($accountTfIdf?.index?.docs ?? {}).sort();
```

- [ ] **Step 3: Apply rules reactively when data or rules change**

Find the existing reactive block:

```typescript
  $: if (!_.isEmpty(data) && $templateEditorState.template) {
```

Before it, add a new reactive block that applies rules whenever `rows` or `rules` change:

```typescript
  let enrichedRows: Array<Record<string, any>> = [];
  $: enrichedRows = applyRules(rows, rules);
```

Then update the `renderJournal` call inside the existing reactive block — change `rows` to `enrichedRows`:

```typescript
        preview = renderJournal(enrichedRows, $templateEditorState.template, {
```

- [ ] **Step 4: Add ACCOUNT column to the preview table**

In the `{#if !_.isEmpty(data)}` block, find the `<thead>` row:

```svelte
              <thead>
                <tr>
                  <th />
                  {#each _.range(0, columnCount) as ci}
                    <th class="has-background-light">{String.fromCharCode(65 + ci)}</th>
                  {/each}
                </tr>
              </thead>
```

Replace it with:

```svelte
              <thead>
                <tr>
                  <th />
                  {#each _.range(0, columnCount) as ci}
                    <th class="has-background-light">{String.fromCharCode(65 + ci)}</th>
                  {/each}
                  <th class="has-background-light">ACCOUNT</th>
                </tr>
              </thead>
```

Find the `<tbody>` rows block:

```svelte
              <tbody>
                {#each data as row, ri}
                  <tr>
                    <th class="has-background-light"><b>{ri}</b></th>
                    {#each row as cell}
                      <td>{cell || ""}</td>
                    {/each}
                  </tr>
                {/each}
              </tbody>
```

Replace it with:

```svelte
              <tbody>
                {#each data as row, ri}
                  <tr>
                    <th class="has-background-light"><b>{ri}</b></th>
                    {#each row as cell}
                      <td>{cell || ""}</td>
                    {/each}
                    <td>
                      {#if enrichedRows[ri]?.ACCOUNT}
                        <span class="tag is-success is-light is-small">{enrichedRows[ri].ACCOUNT}</span>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
```

- [ ] **Step 5: Add rule CRUD functions**

After the `remove()` function (around line 114), add:

```typescript
  function validateRegex(pattern: string): string | null {
    try {
      new RegExp(pattern);
      return null;
    } catch (e) {
      return e instanceof Error ? e.message : "Invalid regex";
    }
  }

  async function addRule() {
    newRuleRegexError = validateRegex(newRule.match);
    if (newRuleRegexError) return;
    const { saved, message } = await ajax("/api/import/rules/upsert", {
      method: "POST",
      body: JSON.stringify(newRule),
      background: true
    });
    if (!saved) {
      toast.toast({ message: `Failed to save rule. ${message}`, type: "is-danger", duration: 5000 });
      return;
    }
    ({ rules } = await ajax("/api/import/rules", { background: true }));
    newRule = { name: "", match: "", account: "" };
    newRuleRegexError = null;
  }

  async function deleteRule(name: string) {
    await ajax("/api/import/rules/delete", {
      method: "POST",
      body: JSON.stringify({ name }),
      background: true
    });
    ({ rules } = await ajax("/api/import/rules", { background: true }));
  }
```

- [ ] **Step 6: Add the rules management panel to the template**

In the HTML template, find the closing `</div>` of the template editor box (the `<div class="box py-0">` that wraps the template editor, around line 329). Add the rules panel immediately after that closing `</div>`:

```svelte
        <div class="box p-3 mt-3">
          <p class="has-text-weight-semibold mb-2">Import Rules</p>
          {#if !_.isEmpty(rules)}
            <table class="table is-narrow is-fullwidth is-size-7 mb-3">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Match (regex)</th>
                  <th>Account</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {#each rules as rule}
                  <tr>
                    <td>{rule.name}</td>
                    <td><code>{rule.match}</code></td>
                    <td>{rule.account}</td>
                    <td>
                      <button
                        class="button is-small is-danger is-light"
                        on:click={() => deleteRule(rule.name)}
                        data-tippy-content="Delete rule"
                      >
                        <span class="icon is-small"><i class="fas fa-trash-can" /></span>
                      </button>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
          <div class="field is-grouped is-grouped-multiline">
            <div class="control">
              <input class="input is-small" type="text" placeholder="Name" bind:value={newRule.name} />
            </div>
            <div class="control is-expanded">
              <input
                class="input is-small"
                class:is-danger={newRuleRegexError}
                type="text"
                placeholder="Regex (e.g. swiggy|zomato)"
                bind:value={newRule.match}
                on:blur={() => { newRuleRegexError = validateRegex(newRule.match); }}
              />
              {#if newRuleRegexError}
                <p class="help is-danger">{newRuleRegexError}</p>
              {/if}
            </div>
            <div class="control is-expanded">
              <input
                class="input is-small"
                type="text"
                placeholder="Account (e.g. Expenses:Food)"
                list="import-rules-accounts"
                bind:value={newRule.account}
              />
              <datalist id="import-rules-accounts">
                {#each accountList as account}
                  <option value={account} />
                {/each}
              </datalist>
            </div>
            <div class="control">
              <button
                class="button is-small is-primary"
                disabled={_.isEmpty(newRule.name) || _.isEmpty(newRule.match) || _.isEmpty(newRule.account) || !!newRuleRegexError}
                on:click={addRule}
              >
                Add Rule
              </button>
            </div>
          </div>
        </div>
```

- [ ] **Step 7: Verify TypeScript compiles**

```bash
cd /Users/sarath.m/workspace/work/paisa && npx tsc --noEmit 2>&1 | head -30
```

Expected: no new errors.

- [ ] **Step 8: Manual smoke test**

Start the server:

```bash
cd /Users/sarath.m/workspace/work/paisa && go run . serve
```

Open `http://localhost:7500/ledger/import` and verify:

- [ ] Import page loads without errors
- [ ] Rules panel is visible below the template editor (empty by default)
- [ ] Add a rule: Name = "Test", Match = `coffee|cafe`, Account = `Expenses:Food`. Click **Add Rule**. Rule appears in the table.
- [ ] Upload a CSV that contains the word "coffee" in one row. That row's ACCOUNT column shows a green `Expenses:Food` badge; other rows are blank.
- [ ] In the template editor, add `{{ROW.ACCOUNT}}` somewhere. Verify the preview pane renders the matched account for matched rows.
- [ ] Delete the rule. The green badge disappears from the preview table.
- [ ] Enter an invalid regex (e.g. `[bad`). The Save button is disabled and an error message appears.
- [ ] Reload the page. Rules are still there (persisted in `paisa.yaml`).

- [ ] **Step 9: Commit**

```bash
git -C /Users/sarath.m/workspace/work/paisa add "src/routes/(app)/ledger/import/+page.svelte"
git -C /Users/sarath.m/workspace/work/paisa commit -m "feat: add import rules panel and ACCOUNT column to import page"
```

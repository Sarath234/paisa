# Editor Files Performance Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the 74-second `/api/editor/files` outlier and reduce its baseline cost by fixing SQLite lock contention, capping backup file accumulation, and deferring file content reads in the editor.

**Architecture:** Three independent fixes: (1) enable WAL journal mode and a busy timeout so concurrent reads never block behind commodity sync writes; (2) trim backup files to the last 10 on each save so the directory walk stays small; (3) add a `?metadata_only=true` query param to `GET /api/editor/files` and update the editor page to load selected-file content lazily via the existing `POST /api/editor/file` endpoint, leaving every other page that calls `editor/files` unchanged.

**Tech Stack:** Go (GORM, mattn/go-sqlite3), Svelte/TypeScript.

---

## Files

- Modify: `internal/utils/utils.go:287-290` — `OpenDB`, add WAL + busy_timeout pragmas
- Modify: `internal/server/editor.go:28-47` — `GetFiles`, add `metadataOnly` param
- Modify: `internal/server/editor.go:72-130` — `SaveFile`, trim old backups after write
- Modify: `internal/server/editor.go:172-199` — add `readLedgerFileMetadata` helper
- Modify: `internal/server/server.go:235-237` — pass `metadata_only` query param to `GetFiles`
- Modify: `src/routes/(app)/ledger/editor/[slug]/+page.svelte:87-101` — `loadFiles` + `selectFile`
- Test: `internal/ledger/ledger_test.go` — existing test file for WAL verification
- Test: `internal/server/editor_test.go` — new file for backup retention

---

## Task 1: Enable WAL mode and busy timeout

**Files:**
- Modify: `internal/utils/utils.go:287-290`

WAL mode lets readers proceed concurrently with commodity sync writes. The busy timeout makes write-lock retries wait up to 5 s instead of immediately returning `SQLITE_BUSY`.

- [ ] **Step 1: Write the failing test**

Create `internal/utils/utils_test.go`:

```go
package utils

import (
	"os"
	"testing"

	"github.com/ananthakumaran/paisa/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestOpenDBUsesWAL(t *testing.T) {
	dir := t.TempDir()
	config.SetDBPath(dir + "/test.db")
	defer os.Remove(dir + "/test.db")

	db, err := OpenDB()
	assert.NoError(t, err)

	var mode string
	db.Raw("PRAGMA journal_mode").Scan(&mode)
	assert.Equal(t, "wal", mode)

	var timeout int
	db.Raw("PRAGMA busy_timeout").Scan(&timeout)
	assert.Equal(t, 5000, timeout)
}
```

- [ ] **Step 2: Check whether `config.SetDBPath` exists**

```bash
grep -n "SetDBPath\|DBPath\|GetDBPath" /Users/sarath.m/workspace/work/paisa/internal/config/*.go | head -20
```

If `SetDBPath` does not exist, the test must use a different mechanism to set the DB path. In that case replace the config lines with:

```go
// open a direct gorm connection to a temp file
import "gorm.io/driver/sqlite"
import "gorm.io/gorm"

db, err := gorm.Open(sqlite.Open(dir+"/test.db"), &gorm.Config{})
assert.NoError(t, err)
sqlDB, _ := db.DB()
_, err = sqlDB.Exec("PRAGMA journal_mode = WAL")
assert.NoError(t, err)
_, err = sqlDB.Exec("PRAGMA busy_timeout = 5000")
assert.NoError(t, err)

var mode string
db.Raw("PRAGMA journal_mode").Scan(&mode)
assert.Equal(t, "wal", mode)
```

Adjust accordingly, then re-read the test file before saving.

- [ ] **Step 3: Run the test to confirm it fails**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/utils/... -run TestOpenDBUsesWAL -v
```

Expected: FAIL — `journal_mode` is `delete`, not `wal`.

- [ ] **Step 4: Implement WAL mode in `OpenDB`**

Replace `internal/utils/utils.go:287-290`:

```go
func OpenDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(config.GetDBPath()), &gorm.Config{Logger: gorm_logrus.New()})
	if err != nil {
		return db, err
	}
	db.Exec("PRAGMA journal_mode = WAL")
	db.Exec("PRAGMA busy_timeout = 5000")
	return db, nil
}
```

- [ ] **Step 5: Run test to confirm it passes**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/utils/... -run TestOpenDBUsesWAL -v
```

Expected: PASS.

- [ ] **Step 6: Run full test suite to check for regressions**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/... 2>&1 | grep -E "FAIL|ok|---"
```

Expected: no new failures.

- [ ] **Step 7: Build the ledger package to confirm compilation**

```bash
cd /Users/sarath.m/workspace/work/paisa && go build github.com/ananthakumaran/paisa/internal/...  2>&1 | grep -v "no matching files"
```

Expected: no errors (the `web` embed error for missing static files is pre-existing and can be ignored).

- [ ] **Step 8: Commit**

```bash
git -C /Users/sarath.m/workspace/work/paisa/.worktrees/editor-files-perf add internal/utils/utils.go internal/utils/utils_test.go
git -C /Users/sarath.m/workspace/work/paisa/.worktrees/editor-files-perf commit -m "perf: enable WAL journal mode and 5s busy timeout on SQLite"
```

---

## Task 2: Backup retention limit (keep last 10)

**Files:**
- Modify: `internal/server/editor.go:72-130` — `SaveFile`
- Create: `internal/server/editor_test.go`

Each `SaveFile` call creates one `.backup.YYYY-MM-DD-HH-MM-SS.mmm` file. After 4259 backups the directory walk is measurably slow. Trim to the last 10 after every save.

- [ ] **Step 1: Write the failing test**

Create `internal/server/editor_test.go`:

```go
package server

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSaveFilePrunesOldBackups(t *testing.T) {
	dir := t.TempDir()

	// Create a fake journal file
	journalPath := filepath.Join(dir, "transactions.ledger")
	err := os.WriteFile(journalPath, []byte("original"), 0644)
	assert.NoError(t, err)

	// Pre-create 15 backup files (simulating previous saves)
	for i := 0; i < 15; i++ {
		ts := time.Now().Add(time.Duration(i) * time.Second).Format("2006-01-02-15-04-05.000")
		backupPath := journalPath + ".backup." + ts
		err = os.WriteFile(backupPath, []byte(fmt.Sprintf("backup-%d", i)), 0644)
		assert.NoError(t, err)
	}

	// Verify 15 backups exist before calling pruneOldBackups
	before, _ := filepath.Glob(journalPath + ".backup.*")
	assert.Len(t, before, 15)

	// Call the prune function (to be implemented)
	pruneOldBackups(journalPath, 10)

	// Verify only 10 remain
	after, _ := filepath.Glob(journalPath + ".backup.*")
	assert.Len(t, after, 10)
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/server/... -run TestSaveFilePrunesOldBackups -v
```

Expected: FAIL — `pruneOldBackups` undefined.

- [ ] **Step 3: Implement `pruneOldBackups` and call it in `SaveFile`**

In `internal/server/editor.go`, add the helper function (place it before `readLedgerFile`):

```go
func pruneOldBackups(filePath string, maxBackups int) {
	backups, _ := filepath.Glob(filePath + ".backup.*")
	sort.Strings(backups) // lexicographic order matches chronological for our timestamp format
	if len(backups) > maxBackups {
		for _, old := range backups[:len(backups)-maxBackups] {
			os.Remove(old)
		}
	}
}
```

`sort` is already imported. Add the call at the end of `SaveFile`, just before the final `return`, after the `os.WriteFile` call:

```go
	err = os.WriteFile(filePath, []byte(file.Content), perm)
	if err != nil {
		log.Warn(err)
		return gin.H{"errors": errors, "saved": false, "message": "Failed to write file"}
	}

	pruneOldBackups(filePath, 10)  // ← add this line

	Sync(db, SyncRequest{Journal: true})
```

- [ ] **Step 4: Add `"sort"` to imports if missing**

Check the import block in `editor.go`. `"sort"` is already present (used in `readLedgerFileWithVersions`). No change needed.

- [ ] **Step 5: Run test to confirm it passes**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/server/... -run TestSaveFilePrunesOldBackups -v
```

Expected: PASS.

- [ ] **Step 6: Run full test suite**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/... 2>&1 | grep -E "FAIL|ok|---"
```

Expected: no new failures.

- [ ] **Step 7: Commit**

```bash
git -C /Users/sarath.m/workspace/work/paisa/.worktrees/editor-files-perf add internal/server/editor.go internal/server/editor_test.go
git -C /Users/sarath.m/workspace/work/paisa/.worktrees/editor-files-perf commit -m "perf: prune old backup files to last 10 on each editor save"
```

---

## Task 3: Lazy content loading in the editor page

**Files:**
- Modify: `internal/server/editor.go:28-47` — `GetFiles` and add `readLedgerFileMetadata`
- Modify: `internal/server/server.go:235-237` — route reads `metadata_only` query param
- Modify: `src/routes/(app)/ledger/editor/[slug]/+page.svelte:87-101` — `loadFiles` + `selectFile`

`GET /api/editor/files?metadata_only=true` returns filenames + versions but no content. The editor page uses this for the sidebar, then loads the selected file's content via `POST /api/editor/file`. All other callers (`/ledger/transaction`, `/ledger/posting`, `/more/sheets`) continue to call without the param and receive full content as before.

- [ ] **Step 1: Add `readLedgerFileMetadata` to `editor.go`**

Add this function immediately before `readLedgerFile` in `internal/server/editor.go`:

```go
func readLedgerFileMetadata(dir string, path string) *LedgerFile {
	versions, _ := filepath.Glob(filepath.Join(filepath.Dir(path), filepath.Base(path)+".backup.*"))
	versionPaths := lo.Map(versions, func(p string, _ int) string {
		name, _ := filepath.Rel(dir, p)
		return name
	})
	sort.Sort(sort.Reverse(sort.StringSlice(versionPaths)))

	name, _ := filepath.Rel(dir, path)
	return &LedgerFile{Name: name, Versions: versionPaths}
}
```

- [ ] **Step 2: Update `GetFiles` to accept `metadataOnly bool`**

Replace the existing `GetFiles` signature and body in `internal/server/editor.go`:

```go
func GetFiles(db *gorm.DB, metadataOnly bool) gin.H {
	var accounts []string
	var payees []string
	var commodities []string
	db.Model(&posting.Posting{}).Distinct().Pluck("Account", &accounts)
	db.Model(&posting.Posting{}).Distinct().Pluck("Payee", &payees)
	db.Model(&posting.Posting{}).Distinct().Pluck("Commodity", &commodities)

	path := config.GetJournalPath()
	files := []*LedgerFile{}
	dir := filepath.Dir(path)
	paths, _ := doublestar.FilepathGlob(dir + "/**/*" + filepath.Ext(path))

	for _, path = range paths {
		if metadataOnly {
			files = append(files, readLedgerFileMetadata(dir, path))
		} else {
			files = append(files, readLedgerFileWithVersions(dir, path))
		}
	}

	return gin.H{"files": files, "accounts": accounts, "payees": payees, "commodities": commodities}
}
```

- [ ] **Step 3: Update the route in `server.go` to pass `metadata_only`**

Replace the route in `internal/server/server.go:235-237`:

```go
router.GET("/api/editor/files", func(c *gin.Context) {
    metadataOnly := c.Query("metadata_only") == "true"
    c.JSON(200, GetFiles(db, metadataOnly))
})
```

- [ ] **Step 4: Build to confirm backend compiles**

```bash
cd /Users/sarath.m/workspace/work/paisa && go build github.com/ananthakumaran/paisa/internal/... 2>&1 | grep -v "no matching files"
```

Expected: no errors.

- [ ] **Step 5: Run tests**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test ./internal/... 2>&1 | grep -E "FAIL|ok|---"
```

Expected: no failures.

- [ ] **Step 6: Update `loadFiles` and `selectFile` in the editor Svelte page**

In `src/routes/(app)/ledger/editor/[slug]/+page.svelte`, replace the `loadFiles` function (lines 87–94) and `selectFile` function (lines 96–101):

```typescript
  async function loadFiles(selectedFileName: string) {
    let files;
    ({ files, accounts, commodities, payees } = await ajax("/api/editor/files?metadata_only=true"));
    filesMap = _.fromPairs(_.map(files, (f) => [f.name, f]));
    if (!_.isEmpty(files)) {
      const target = _.find(files, (f) => f.name == selectedFileName) || files[0];
      const { file: loadedFile } = await ajax("/api/editor/file", {
        method: "POST",
        body: JSON.stringify({ name: target.name }),
        background: true
      });
      selectedFile = { ...target, content: loadedFile.content };
    }
  }

  async function selectFile(file: LedgerFile) {
    const success = await navigate(`/ledger/editor/${encodeURIComponent(file.name)}`);
    if (success) {
      const { file: loadedFile } = await ajax("/api/editor/file", {
        method: "POST",
        body: JSON.stringify({ name: file.name }),
        background: true
      });
      selectedFile = { ...file, content: loadedFile.content };
    }
  }
```

- [ ] **Step 7: Verify TypeScript compiles**

```bash
cd /Users/sarath.m/workspace/work/paisa && npx tsc --noEmit 2>&1 | head -30
```

Expected: no errors (or pre-existing errors only — zero new ones).

- [ ] **Step 8: Manual smoke test**

Start the server:
```bash
cd /Users/sarath.m/workspace/work/paisa && go run . serve
```

Open `http://localhost:7500/ledger/editor/` and verify:
- [ ] File list loads quickly (no file content in the initial request — verify in browser DevTools Network tab that `editor/files?metadata_only=true` response has empty `content` fields)
- [ ] Selecting a file shows the correct content
- [ ] Switching between files shows correct content for each
- [ ] Saving a file still works and shows the file content after save
- [ ] Revert to a backup version still works

- [ ] **Step 9: Commit**

```bash
git -C /Users/sarath.m/workspace/work/paisa/.worktrees/editor-files-perf add \
  internal/server/editor.go \
  internal/server/server.go \
  src/routes/\(app\)/ledger/editor/\[slug\]/+page.svelte
git -C /Users/sarath.m/workspace/work/paisa/.worktrees/editor-files-perf commit -m "perf: lazy-load file content in editor, metadata_only param for GetFiles"
```

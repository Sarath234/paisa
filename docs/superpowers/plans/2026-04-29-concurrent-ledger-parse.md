# Concurrent Ledger Journal Parsing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run the regular and budget journal parse subprocess calls in parallel for both `LedgerCLI` and `HLedgerCLI`, halving the subprocess I/O time on every sync.

**Architecture:** Both `LedgerCLI.Parse` and `HLedgerCLI.Parse` currently call their respective `exec*Command` helper twice sequentially — once for regular postings, once for forecast/budget postings. Since these two calls are fully independent (separate subprocess, separate output buffer, read-only input), they can be dispatched as concurrent goroutines and collected via buffered channels. No new dependency is needed — `utils.Exec` creates its own `exec.Command` and its own `bytes.Buffer` per call, making it safe to invoke concurrently.

**Tech Stack:** Go standard library (`sync`, goroutines, buffered channels). No new imports required beyond what already exists in the file.

---

## Files

- Modify: `internal/ledger/ledger.go:88-107` — `LedgerCLI.Parse`
- Modify: `internal/ledger/ledger.go:170-186` — `HLedgerCLI.Parse`
- Test: `internal/ledger/ledger_test.go` — run with `-race` (no unit-testable boundary to add here; the functions require live binaries)

---

## Task 1: Concurrent `LedgerCLI.Parse`

**Files:**
- Modify: `internal/ledger/ledger.go:88-107`

The current implementation calls `execLedgerCommand` twice in sequence. Replace with two goroutines sending results over buffered channels.

Note: the original code also has a minor ordering bug — `lo.Filter` is called on `budgetPostings` *before* the `err != nil` check. The rewrite fixes this naturally.

- [ ] **Step 1: Replace the sequential body of `LedgerCLI.Parse` with concurrent goroutines**

Replace lines 88–107 in `internal/ledger/ledger.go`:

```go
func (LedgerCLI) Parse(journalPath string, _prices []price.Price) ([]*posting.Posting, error) {
	type result struct {
		postings []*posting.Posting
		err      error
	}

	ch1 := make(chan result, 1)
	ch2 := make(chan result, 1)

	go func() {
		ps, err := execLedgerCommand(journalPath, []string{})
		ch1 <- result{ps, err}
	}()

	go func() {
		ps, err := execLedgerCommand(journalPath, []string{"--now", strconv.Itoa(utils.Now().Year() + 3), "--budget"})
		ch2 <- result{ps, err}
	}()

	r1 := <-ch1
	if r1.err != nil {
		return nil, r1.err
	}

	r2 := <-ch2
	if r2.err != nil {
		return nil, r2.err
	}

	budgetPostings := lo.Filter(r2.postings, func(p *posting.Posting, _ int) bool {
		return p.Payee == "Budget transaction"
	})

	return append(r1.postings, budgetPostings...), nil
}
```

No new imports are needed — `strconv` and `lo` are already imported.

- [ ] **Step 2: Run existing unit tests with the race detector**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test -race ./internal/ledger/...
```

Expected: all tests pass, no race conditions reported.

- [ ] **Step 3: Build the whole project to confirm no compilation errors**

```bash
cd /Users/sarath.m/workspace/work/paisa && go build ./...
```

Expected: exits 0 with no output.

- [ ] **Step 4: Commit**

```bash
git add internal/ledger/ledger.go
git commit -m "perf: run LedgerCLI regular and budget parse concurrently"
```

---

## Task 2: Concurrent `HLedgerCLI.Parse`

**Files:**
- Modify: `internal/ledger/ledger.go:170-186`

Same pattern as Task 1. The second `execHLedgerCommand` call already filters by tag (`tag:_generated-transaction`) via a CLI flag, so all returned postings from that call are forecast entries — no post-filter needed.

- [ ] **Step 1: Replace the sequential body of `HLedgerCLI.Parse` with concurrent goroutines**

Replace lines 170–186 in `internal/ledger/ledger.go`:

```go
func (HLedgerCLI) Parse(journalPath string, prices []price.Price) ([]*posting.Posting, error) {
	type result struct {
		postings []*posting.Posting
		err      error
	}

	ch1 := make(chan result, 1)
	ch2 := make(chan result, 1)

	go func() {
		ps, err := execHLedgerCommand(journalPath, prices, []string{})
		ch1 <- result{ps, err}
	}()

	timeRange := fmt.Sprintf("%d..%d", utils.Now().Year()-3, utils.Now().Year()+3)
	go func() {
		ps, err := execHLedgerCommand(journalPath, prices, []string{"--ignore-assertions", "--forecast=" + timeRange, "tag:_generated-transaction"})
		ch2 <- result{ps, err}
	}()

	r1 := <-ch1
	if r1.err != nil {
		return nil, r1.err
	}

	r2 := <-ch2
	if r2.err != nil {
		return nil, r2.err
	}

	return append(r1.postings, r2.postings...), nil
}
```

No new imports needed.

- [ ] **Step 2: Run existing unit tests with the race detector**

```bash
cd /Users/sarath.m/workspace/work/paisa && go test -race ./internal/ledger/...
```

Expected: all tests pass, no race conditions reported.

- [ ] **Step 3: Build the whole project**

```bash
cd /Users/sarath.m/workspace/work/paisa && go build ./...
```

Expected: exits 0 with no output.

- [ ] **Step 4: Commit**

```bash
git add internal/ledger/ledger.go
git commit -m "perf: run HLedgerCLI regular and forecast parse concurrently"
```

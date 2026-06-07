// internal/agent/ledger/appender_test.go
package ledger_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/ledger"
	"github.com/stretchr/testify/assert"
)

func TestEnsureFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	err := ledger.EnsureFile(dir)
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "auto-import.ledger"))
	assert.NoError(t, err, "auto-import.ledger should exist")
}

func TestEnsureFile_AddsInclude(t *testing.T) {
	dir := t.TempDir()
	mainJournal := filepath.Join(dir, "main.ledger")
	err := os.WriteFile(mainJournal, []byte("; main journal\n"), 0644)
	assert.NoError(t, err)

	err = ledger.EnsureFile(dir)
	assert.NoError(t, err)

	content, _ := os.ReadFile(mainJournal)
	assert.Contains(t, string(content), "include auto-import.ledger")
}

func TestEnsureFile_IdempotentInclude(t *testing.T) {
	dir := t.TempDir()
	mainJournal := filepath.Join(dir, "main.ledger")
	err := os.WriteFile(mainJournal, []byte("include auto-import.ledger\n"), 0644)
	assert.NoError(t, err)

	err = ledger.EnsureFile(dir)
	assert.NoError(t, err)

	content, _ := os.ReadFile(mainJournal)
	assert.Equal(t, 1, strings.Count(string(content), "include auto-import.ledger"))
}

func TestAppend_WritesLedgerBlock(t *testing.T) {
	dir := t.TempDir()
	err := ledger.EnsureFile(dir)
	assert.NoError(t, err)

	e := &ledger.Entry{
		Date: "2026/06/03",
		Desc: "Food Swiggy",
		Src:  "Assets:Checking:FC2148",
		Amt:  "-215.00 INR",
		Dest: "Expenses:Food:Hyd",
	}
	err = ledger.Append(dir, e)
	assert.NoError(t, err)

	content, _ := os.ReadFile(filepath.Join(dir, "auto-import.ledger"))
	s := string(content)
	assert.Contains(t, s, "2026/06/03 Food Swiggy")
	assert.Contains(t, s, "Assets:Checking:FC2148")
	assert.Contains(t, s, "-215.00 INR")
	assert.Contains(t, s, "Expenses:Food:Hyd")
}

func TestIsDuplicate_DetectsSameEntry(t *testing.T) {
	dir := t.TempDir()
	ledger.EnsureFile(dir)
	e := &ledger.Entry{Date: "2026/06/03", Desc: "Food Swiggy", Src: "Assets:Checking:FC2148", Amt: "-215.00 INR", Dest: "Expenses:Food:Hyd"}
	ledger.Append(dir, e)

	dup, err := ledger.IsDuplicate(dir, e)
	assert.NoError(t, err)
	assert.True(t, dup)
}

func TestIsDuplicate_DifferentEntry(t *testing.T) {
	dir := t.TempDir()
	ledger.EnsureFile(dir)
	e1 := &ledger.Entry{Date: "2026/06/03", Desc: "Food Swiggy", Src: "Assets:Checking:FC2148", Amt: "-215.00 INR", Dest: "Expenses:Food:Hyd"}
	e2 := &ledger.Entry{Date: "2026/06/04", Desc: "Food Zomato", Src: "Assets:Checking:FC2148", Amt: "-327.25 INR", Dest: "Expenses:Food:Hyd"}
	ledger.Append(dir, e1)

	dup, err := ledger.IsDuplicate(dir, e2)
	assert.NoError(t, err)
	assert.False(t, dup)
}

// internal/agent/journaledit/journaledit_test.go
package journaledit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestFindEntryUnique(t *testing.T) {
	dir := t.TempDir()
	journal := `2026/06/20 Coffee
    Liabilities:CreditCard:ICIC6009               -240.00 INR
    Expenses:Food:Hyd

; source: telegram_approved
2026/06/22 Dup Spend
    Liabilities:CreditCard:ICIC6009               -999.00 INR
    Expenses:Food:Hyd
`
	os.WriteFile(filepath.Join(dir, "auto-import.ledger"), []byte(journal), 0644)
	block, file, err := FindEntry(dir, day("2026-06-22"), -999.00, "Liabilities:CreditCard:ICIC6009")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(block, "Dup Spend") || !strings.Contains(block, "; source:") {
		t.Fatalf("block: %q", block)
	}
	if filepath.Base(file) != "auto-import.ledger" {
		t.Fatalf("file: %s", file)
	}
}

func TestFindEntryAmbiguous(t *testing.T) {
	dir := t.TempDir()
	entry := "2026/06/22 Same\n    Liabilities:CreditCard:ICIC6009               -999.00 INR\n    Expenses:Food:Hyd\n"
	os.WriteFile(filepath.Join(dir, "a.ledger"), []byte(entry+"\n"+entry), 0644)
	_, _, err := FindEntry(dir, day("2026-06-22"), -999.00, "Liabilities:CreditCard:ICIC6009")
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("want ErrAmbiguous, got %v", err)
	}
}

func TestFindEntryNotFound(t *testing.T) {
	_, _, err := FindEntry(t.TempDir(), day("2026-06-22"), -1.00, "X:Y")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestRemoveBlockBacksUpAndRemoves(t *testing.T) {
	dir := t.TempDir()
	keep := "2026/06/20 Coffee\n    L:C               -240.00 INR\n    E:F\n"
	dup := "2026/06/22 Dup\n    L:C               -999.00 INR\n    E:F\n"
	path := filepath.Join(dir, "a.ledger")
	os.WriteFile(path, []byte(keep+"\n"+dup), 0644)
	if err := RemoveBlock(dir, path, dup); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if strings.Contains(string(after), "Dup") || !strings.Contains(string(after), "Coffee") {
		t.Fatalf("after: %q", after)
	}
	baks, _ := filepath.Glob(path + ".*.bak")
	if len(baks) != 1 {
		t.Fatal("backup missing")
	}
}

func TestRemoveBlockRefusesNonUnique(t *testing.T) {
	dir := t.TempDir()
	dup := "2026/06/22 Dup\n    L:C               -999.00 INR\n    E:F\n"
	path := filepath.Join(dir, "a.ledger")
	os.WriteFile(path, []byte(dup+"\n"+dup), 0644)
	if err := RemoveBlock(dir, path, dup); err == nil {
		t.Fatal("must refuse when block occurs twice")
	}
}

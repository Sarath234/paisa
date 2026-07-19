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

// TestRemoveBlockRefusesPrefixOfLargerBlock: if the entry gained a posting
// line externally since FindEntry captured it, the captured block is a strict
// PREFIX of the (now larger) real block. Substring counting would still see
// exactly one occurrence and delete just the prefix, orphaning the added
// line. RemoveBlock must refuse and leave the file byte-identical.
func TestRemoveBlockRefusesPrefixOfLargerBlock(t *testing.T) {
	dir := t.TempDir()
	captured := "2026/06/22 Dup\n    L:C               -999.00 INR\n    E:F"
	larger := captured + "\n    Assets:Adjustment               1.00 INR\n"
	keep := "2026/06/20 Coffee\n    L:C               -240.00 INR\n    E:F\n"
	path := filepath.Join(dir, "a.ledger")
	os.WriteFile(path, []byte(keep+"\n"+larger), 0644)
	before, _ := os.ReadFile(path)
	if err := RemoveBlock(dir, path, captured); err == nil {
		t.Fatal("must refuse when captured block is only a prefix of a larger block")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatalf("file must be unchanged:\nbefore: %q\nafter:  %q", before, after)
	}
}

func TestFindEntryAmountBoundaryDisambiguates(t *testing.T) {
	dir := t.TempDir()
	journal := "2026/06/22 Big Credit\n    Assets:Bank               140.00 INR\n    Income:X\n\n" +
		"2026/06/22 Small Credit\n    Assets:Bank               40.00 INR\n    Income:X\n"
	os.WriteFile(filepath.Join(dir, "a.ledger"), []byte(journal), 0644)
	block, _, err := FindEntry(dir, day("2026-06-22"), 40.00, "Assets:Bank")
	if err != nil {
		t.Fatalf("40.00 must match uniquely despite 140.00 present: %v", err)
	}
	if !strings.Contains(block, "Small Credit") {
		t.Fatalf("wrong block: %q", block)
	}
}

func TestFindEntryAmountNoSubstringMatch(t *testing.T) {
	dir := t.TempDir()
	journal := "2026/06/22 Big Credit\n    Assets:Bank               140.00 INR\n    Income:X\n"
	os.WriteFile(filepath.Join(dir, "a.ledger"), []byte(journal), 0644)
	_, _, err := FindEntry(dir, day("2026-06-22"), 40.00, "Assets:Bank")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("40.00 must not match inside 140.00; want ErrNotFound, got %v", err)
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

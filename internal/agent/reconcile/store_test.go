// internal/agent/reconcile/store_test.go
package reconcile

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/statement"
)

func TestStore_WriteRead(t *testing.T) {
	dir := t.TempDir()

	rec := Record{
		Period:      "2026-05",
		GeneratedAt: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
		Diff: Diff{
			Account:        "Assets:Checking:AXIS6386",
			Month:          5,
			Year:           2026,
			StatementClose: 9000.00,
			Missing: []statement.Transaction{
				{Date: date(2026, 5, 3), Description: "AMAZON", Debit: 1200},
			},
			Extra: nil,
		},
	}

	if err := Write(dir, rec); err != nil {
		t.Fatalf("Write: %v", err)
	}

	records, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len=%d want 1", len(records))
	}
	if records[0].Period != "2026-05" {
		t.Errorf("Period=%q want 2026-05", records[0].Period)
	}
	if records[0].Diff.StatementClose != 9000.00 {
		t.Errorf("StatementClose=%.2f want 9000.00", records[0].Diff.StatementClose)
	}
	if len(records[0].Diff.Missing) != 1 {
		t.Errorf("Missing=%d want 1", len(records[0].Diff.Missing))
	}
}

func TestStore_UpsertReplacesExistingPeriod(t *testing.T) {
	dir := t.TempDir()

	r1 := Record{Period: "2026-05", Diff: Diff{StatementClose: 1000}}
	r2 := Record{Period: "2026-05", Diff: Diff{StatementClose: 2000}}
	r3 := Record{Period: "2026-04", Diff: Diff{StatementClose: 500}}

	_ = Write(dir, r1)
	_ = Write(dir, r3)
	_ = Write(dir, r2) // overwrites r1

	records, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len=%d want 2", len(records))
	}
	var found bool
	for _, r := range records {
		if r.Period == "2026-05" {
			if r.Diff.StatementClose != 2000 {
				t.Errorf("upsert: StatementClose=%.0f want 2000", r.Diff.StatementClose)
			}
			found = true
		}
	}
	if !found {
		t.Error("2026-05 record not found after upsert")
	}
}

func TestStore_ReadAll_missingFile(t *testing.T) {
	dir := t.TempDir()
	records, err := ReadAll(dir)
	if err != nil {
		t.Errorf("ReadAll on missing file: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("len=%d want 0", len(records))
	}
}

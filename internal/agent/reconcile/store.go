// internal/agent/reconcile/store.go
package reconcile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const storeFile = "reconciliation.json"

// Record is one reconciliation result, keyed by account+period.
type Record struct {
	Period      string    `json:"period"` // "YYYY-MM"
	GeneratedAt time.Time `json:"generated_at"`
	Diff        Diff      `json:"diff"`
}

// Write upserts rec into <journalDir>/reconciliation.json keyed by Period.
func Write(journalDir string, rec Record) error {
	path := filepath.Join(journalDir, storeFile)
	records, err := ReadAll(journalDir)
	if err != nil {
		return err
	}

	replaced := false
	for i, r := range records {
		if r.Period == rec.Period {
			records[i] = rec
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, rec)
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// ReadAll reads all reconciliation records from <journalDir>/reconciliation.json.
// Returns an empty slice (no error) if the file does not exist.
func ReadAll(journalDir string) ([]Record, error) {
	path := filepath.Join(journalDir, storeFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

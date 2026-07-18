// internal/agent/journaledit/journaledit.go
// Package journaledit performs the ONLY destructive journal operation the
// agent has: removing a confirmed-duplicate entry. Every path is guarded:
// unique match required, timestamped backup written before any rewrite.
package journaledit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("journaledit: entry not found")
	ErrAmbiguous = errors.New("journaledit: multiple matching entries")
)

// FindEntry scans *.ledger files under journalDir (top level only) for a
// block whose first line starts with the entry date (YYYY/MM/DD), containing
// account with the formatted amount on the same line.
func FindEntry(journalDir string, date time.Time, amount float64, account string) (string, string, error) {
	amtStr := fmt.Sprintf("%.2f", amount)
	dateStr := date.Format("2006/01/02")
	files, err := filepath.Glob(filepath.Join(journalDir, "*.ledger"))
	if err != nil {
		return "", "", err
	}
	var foundBlock, foundFile string
	count := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", "", err
		}
		for _, block := range splitBlocks(string(data)) {
			lines := strings.Split(block, "\n")
			first := lines[0]
			if strings.HasPrefix(first, "; ") && len(lines) > 1 {
				first = lines[1]
			}
			if !strings.HasPrefix(first, dateStr) {
				continue
			}
			matched := false
			for _, l := range lines[1:] {
				if strings.Contains(l, account) && strings.Contains(l, amtStr) {
					matched = true
					break
				}
			}
			if matched {
				count++
				foundBlock, foundFile = block, f
			}
		}
	}
	switch count {
	case 0:
		return "", "", ErrNotFound
	case 1:
		return foundBlock, foundFile, nil
	default:
		return "", "", ErrAmbiguous
	}
}

// splitBlocks splits journal text into blank-line-separated blocks,
// keeping leading comment lines attached to their entry.
func splitBlocks(text string) []string {
	var blocks []string
	for _, raw := range regexp.MustCompile(`\n\s*\n`).Split(text, -1) {
		b := strings.Trim(raw, "\n")
		if strings.TrimSpace(b) != "" {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// RemoveBlock deletes block from file (must occur exactly once), writing
// <file>.<ts>.bak first.
func RemoveBlock(journalDir, file, block string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	text := string(data)
	if n := strings.Count(text, block); n != 1 {
		return fmt.Errorf("journaledit: block occurs %d times in %s — refusing to edit", n, filepath.Base(file))
	}
	bak := fmt.Sprintf("%s.%d.bak", file, time.Now().Unix())
	if err := os.WriteFile(bak, data, 0644); err != nil {
		return fmt.Errorf("journaledit: backup: %w", err)
	}
	text = strings.Replace(text, block, "", 1)
	// collapse the residual double blank line
	text = regexp.MustCompile(`\n\s*\n\s*\n`).ReplaceAllString(text, "\n\n")
	if err := os.WriteFile(file, []byte(text), 0644); err != nil {
		return fmt.Errorf("journaledit: rewrite: %w", err)
	}
	return nil
}

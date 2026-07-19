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
// account with the formatted amount on the same line. The amount match is
// boundary-anchored (whitespace before, whitespace/EOL after) so 40.00 never
// matches inside 140.00.
func FindEntry(journalDir string, date time.Time, amount float64, account string) (string, string, error) {
	amtStr := fmt.Sprintf("%.2f", amount)
	amtRe := regexp.MustCompile(`(^|\s)` + regexp.QuoteMeta(amtStr) + `(\s|$)`)
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
				if strings.Contains(l, account) && amtRe.MatchString(l) {
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

// RemoveBlock deletes block from file, writing <file>.<ts>.bak first.
//
// Re-verification is block-boundary-aware, not substring-based: the captured
// block must equal exactly one COMPLETE block of the file's current contents
// (per splitBlocks). A substring count would still see one occurrence when
// the entry gained a posting line externally — the captured block becomes a
// strict prefix of the larger real block — and deleting the prefix would
// orphan the added line. If the block is not exactly one whole block,
// RemoveBlock refuses and the file is left untouched (no backup written).
func RemoveBlock(journalDir, file, block string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	// FindEntry blocks come from splitBlocks (newline-trimmed); trim the
	// caller's block the same way so equality means block-for-block.
	target := strings.Trim(block, "\n")
	blocks := splitBlocks(string(data))
	count := 0
	for _, b := range blocks {
		if b == target {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("journaledit: block occurs %d times as a complete block in %s — refusing to edit", count, filepath.Base(file))
	}
	bak := fmt.Sprintf("%s.%d.bak", file, time.Now().Unix())
	if err := os.WriteFile(bak, data, 0644); err != nil {
		return fmt.Errorf("journaledit: backup: %w", err)
	}
	kept := make([]string, 0, len(blocks)-1)
	removed := false
	for _, b := range blocks {
		if !removed && b == target {
			removed = true
			continue
		}
		kept = append(kept, b)
	}
	out := strings.Join(kept, "\n\n")
	if out != "" {
		out += "\n"
	}
	if err := os.WriteFile(file, []byte(out), 0644); err != nil {
		return fmt.Errorf("journaledit: rewrite: %w", err)
	}
	return nil
}

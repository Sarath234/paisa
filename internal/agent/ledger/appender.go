// internal/agent/ledger/appender.go
package ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

const autoImportFile = "auto-import.ledger"
const dupScanLines = 500

var appendMu sync.Mutex

// EnsureFile creates auto-import.ledger if absent, then ensures the main
// journal in journalDir has an include directive for it.
func EnsureFile(journalDir string) error {
	autoPath := filepath.Join(journalDir, autoImportFile)
	if _, err := os.Stat(autoPath); os.IsNotExist(err) {
		log.Infof("ledger: creating %s", autoPath)
		if err := os.WriteFile(autoPath, []byte("; Auto-imported transactions\n\n"), 0644); err != nil {
			return fmt.Errorf("create %s: %w", autoImportFile, err)
		}
	}
	return ensureInclude(journalDir)
}

func ensureInclude(journalDir string) error {
	entries, err := os.ReadDir(journalDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == autoImportFile {
			continue
		}
		if strings.HasSuffix(name, ".ledger") || strings.HasSuffix(name, ".journal") {
			mainPath := filepath.Join(journalDir, name)
			content, err := os.ReadFile(mainPath)
			if err != nil {
				return err
			}
			includeLine := fmt.Sprintf("include %s", autoImportFile)
			if strings.Contains(string(content), includeLine) {
				return nil
			}
			f, err := os.OpenFile(mainPath, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = fmt.Fprintf(f, "\n%s\n", includeLine)
			return err
		}
	}
	return nil
}

// IsDuplicate scans the last dupScanLines lines of auto-import.ledger for an
// entry with the same date, src account, and amount.
func IsDuplicate(journalDir string, e *Entry) (bool, error) {
	autoPath := filepath.Join(journalDir, autoImportFile)
	data, err := os.ReadFile(autoPath)
	if os.IsNotExist(err) {
		log.Debugf("dedup: %s not found — treating as no duplicate", autoImportFile)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(data), "\n")
	start := 0
	if len(lines) > dupScanLines {
		start = len(lines) - dupScanLines
		log.Debugf("dedup: scanning last %d of %d lines for date=%q src=%q amt=%q",
			dupScanLines, len(lines), e.Date, e.Src, e.Amt)
	} else {
		log.Debugf("dedup: scanning %d lines for date=%q src=%q amt=%q", len(lines), e.Date, e.Src, e.Amt)
	}
	for i, line := range lines[start:] {
		if strings.HasPrefix(line, e.Date) {
			end := i + 4
			if end > len(lines[start:]) {
				end = len(lines[start:])
			}
			for _, nextLine := range lines[start:][i+1 : end] {
				if strings.Contains(nextLine, e.Src) && strings.Contains(nextLine, e.Amt) {
					log.Warnf("dedup: duplicate found — date=%q src=%q amt=%q", e.Date, e.Src, e.Amt)
					return true, nil
				}
			}
		}
	}
	log.Debugf("dedup: no duplicate found")
	return false, nil
}

// Append writes the ledger entry block to auto-import.ledger under a mutex.
func Append(journalDir string, e *Entry) error {
	appendMu.Lock()
	defer appendMu.Unlock()
	autoPath := filepath.Join(journalDir, autoImportFile)
	f, err := os.OpenFile(autoPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("open %s: %w", autoImportFile, err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n%s\n", e.Format())
	if err != nil {
		log.Errorf("ledger: write failed to %s: %v", autoPath, err)
		return err
	}
	log.Infof("ledger: appended entry date=%q desc=%q amt=%q src=%q dest=%q",
		e.Date, e.Desc, e.Amt, e.Src, e.Dest)
	return nil
}

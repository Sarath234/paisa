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

	journalPath := filepath.Join(dir, "transactions.ledger")
	err := os.WriteFile(journalPath, []byte("original"), 0644)
	assert.NoError(t, err)

	for i := 0; i < 15; i++ {
		ts := time.Now().Add(time.Duration(i) * time.Second).Format("2006-01-02-15-04-05.000")
		backupPath := journalPath + ".backup." + ts
		err = os.WriteFile(backupPath, []byte(fmt.Sprintf("backup-%d", i)), 0644)
		assert.NoError(t, err)
	}

	before, _ := filepath.Glob(journalPath + ".backup.*")
	assert.Len(t, before, 15)

	pruneOldBackups(journalPath, 10)

	after, _ := filepath.Glob(journalPath + ".backup.*")
	assert.Len(t, after, 10)
}

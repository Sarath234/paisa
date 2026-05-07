package utils

import (
	"os"
	"testing"

	"github.com/ananthakumaran/paisa/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestOpenDBUsesWAL(t *testing.T) {
	dir := t.TempDir()
	config.SetDBPath(dir + "/test.db")
	defer os.Remove(dir + "/test.db")

	db, err := OpenDB()
	assert.NoError(t, err)

	var mode string
	db.Raw("PRAGMA journal_mode").Scan(&mode)
	assert.Equal(t, "wal", mode)

	var timeout int
	db.Raw("PRAGMA busy_timeout").Scan(&timeout)
	assert.Equal(t, 5000, timeout)
}

func TestBuildSubPath(t *testing.T) {
	path, err := BuildSubPath("/usr/home/john/paisa", "main.ledger")
	assert.Nil(t, err)
	assert.Equal(t, "/usr/home/john/paisa/main.ledger", path)

	path, err = BuildSubPath("/usr/home/john/paisa", "subfolder/main.ledger")
	assert.Nil(t, err)
	assert.Equal(t, "/usr/home/john/paisa/subfolder/main.ledger", path)

	path, err = BuildSubPath("/usr/home/john/paisa", "../../../subfolder/travel.ledger")
	assert.Error(t, err)

	path, err = BuildSubPath("/usr/home/john/paisa", "..")
	assert.Error(t, err)

	path, err = BuildSubPath("/usr/home/john/paisa", "./..")
	assert.Error(t, err)

	path, err = BuildSubPath("/usr/home/john/paisa", "./../test.ledger")
	assert.Error(t, err)
}

package dedup

import (
	"path/filepath"
	"testing"

	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := agentdb.Open(filepath.Join(t.TempDir(), "test.db"))
	assert.NoError(t, err)
	return db
}

func TestCheck_DuplicateByRefID(t *testing.T) {
	db := openTestDB(t)
	_ = Record(db, "REF001", "2025-05-14", "HDFC:1234", "sms", 2450)

	result := Check(db, "REF001", "2025-05-14", "HDFC:1234", 2450)
	assert.Equal(t, Duplicate, result)
}

func TestCheck_FuzzyByAmountAndDate(t *testing.T) {
	db := openTestDB(t)
	_ = Record(db, "", "2025-05-14", "HDFC:1234", "sms", 2450)

	result := Check(db, "", "2025-05-15", "HDFC:1234", 2450)
	assert.Equal(t, Fuzzy, result)
}

func TestCheck_NewTransaction(t *testing.T) {
	db := openTestDB(t)
	result := Check(db, "REF999", "2025-05-14", "HDFC:1234", 2450)
	assert.Equal(t, New, result)
}

func TestCheck_DifferentAmountIsNew(t *testing.T) {
	db := openTestDB(t)
	_ = Record(db, "", "2025-05-14", "HDFC:1234", "sms", 2450)

	result := Check(db, "", "2025-05-14", "HDFC:1234", 3000)
	assert.Equal(t, New, result)
}

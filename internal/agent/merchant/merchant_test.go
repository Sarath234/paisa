package merchant

import (
	"os"
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

func TestGate_UnknownMerchant(t *testing.T) {
	db := openTestDB(t)
	assert.Equal(t, NeedsApproval, Gate(db, "NewShop", 500, 0.9, 10000, 3))
}

func TestGate_KnownMerchantAutoApprove(t *testing.T) {
	db := openTestDB(t)
	db.Create(&agentdb.MerchantRule{Merchant: "Swiggy", Account: "Expenses:Food:Dining", ApproveCount: 5, AutoApprove: true})
	assert.Equal(t, AutoPost, Gate(db, "Swiggy", 500, 0.9, 10000, 3))
}

func TestGate_HighValueAlwaysApproval(t *testing.T) {
	db := openTestDB(t)
	db.Create(&agentdb.MerchantRule{Merchant: "Amazon", Account: "Expenses:Shopping", ApproveCount: 10, AutoApprove: true})
	assert.Equal(t, NeedsApproval, Gate(db, "Amazon", 15000, 0.9, 10000, 3))
}

func TestGate_LowConfidenceAlwaysApproval(t *testing.T) {
	db := openTestDB(t)
	db.Create(&agentdb.MerchantRule{Merchant: "Swiggy", Account: "Expenses:Food:Dining", ApproveCount: 5, AutoApprove: true})
	assert.Equal(t, NeedsApproval, Gate(db, "Swiggy", 500, 0.5, 10000, 3))
}

func TestRecordApproval_PromotesAfterThreshold(t *testing.T) {
	db := openTestDB(t)
	for i := 0; i < 3; i++ {
		_ = RecordApproval(db, "NewShop", "Expenses:Shopping", 3)
	}
	var rule agentdb.MerchantRule
	db.First(&rule, "merchant = ?", "NewShop")
	assert.True(t, rule.AutoApprove)
	assert.Equal(t, 3, rule.ApproveCount)
}

func TestBootstrap_SeedsFromJournal(t *testing.T) {
	dir := t.TempDir()
	journal := `
2025/04/01 Swiggy
    Expenses:Food:Dining    INR 350.00
    Assets:HDFC:Savings

2025/04/02 Amazon
    Expenses:Shopping    INR 1200.00
    Assets:HDFC:Savings
`
	journalPath := filepath.Join(dir, "main.journal")
	_ = os.WriteFile(journalPath, []byte(journal), 0644)

	db := openTestDB(t)
	err := Bootstrap(db, journalPath, 3)
	assert.NoError(t, err)

	var swiggy agentdb.MerchantRule
	assert.NoError(t, db.First(&swiggy, "merchant = ?", "Swiggy").Error)
	assert.Equal(t, "Expenses:Food:Dining", swiggy.Account)
	assert.True(t, swiggy.AutoApprove)
}

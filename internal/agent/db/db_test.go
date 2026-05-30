package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenAndMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")
	gdb, err := Open(path)
	assert.NoError(t, err)
	assert.NotNil(t, gdb)

	ref := ImportedRef{RefID: "REF001", Date: "2025-05-14", Amount: 2450, Account: "HDFC:1234", Source: "sms"}
	assert.NoError(t, gdb.Create(&ref).Error)

	var got ImportedRef
	assert.NoError(t, gdb.First(&got, "ref_id = ?", "REF001").Error)
	assert.Equal(t, "2025-05-14", got.Date)

	rule := MerchantRule{Merchant: "Swiggy", Account: "Expenses:Food:Dining", ApproveCount: 3, AutoApprove: true}
	assert.NoError(t, gdb.Create(&rule).Error)

	var gotRule MerchantRule
	assert.NoError(t, gdb.First(&gotRule, "merchant = ?", "Swiggy").Error)
	assert.True(t, gotRule.AutoApprove)
}

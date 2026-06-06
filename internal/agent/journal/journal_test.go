package journal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

func TestFormat_DebitTransaction(t *testing.T) {
	tx := parser.ParsedTransaction{
		Date:                   "2025-05-14",
		Amount:                 -2450.00,
		Currency:               "INR",
		Merchant:               "Swiggy",
		AccountLast4:           "1234",
		Bank:                   "HDFC",
		RefID:                  "47291830",
		SuggestedLedgerAccount: "Expenses:Food:Dining",
	}
	entry := Format(tx, "sms", "Assets:HDFC:Savings")
	assert.Contains(t, entry, "2025/05/14 Swiggy")
	assert.Contains(t, entry, "Expenses:Food:Dining")
	assert.Contains(t, entry, "INR 2450.00")
	assert.Contains(t, entry, "Assets:HDFC:Savings")
	assert.Contains(t, entry, "; ref: 47291830")
	assert.Contains(t, entry, "; source: sms")
}

func TestFormat_CreditTransaction(t *testing.T) {
	tx := parser.ParsedTransaction{
		Date:                   "2025-05-01",
		Amount:                 50000.00,
		Currency:               "INR",
		Merchant:               "Employer",
		AccountLast4:           "1234",
		Bank:                   "HDFC",
		SuggestedLedgerAccount: "Income:Salary",
	}
	entry := Format(tx, "gmail_alert", "Assets:HDFC:Savings")
	assert.Contains(t, entry, "Income:Salary")
	assert.Contains(t, entry, "Assets:HDFC:Savings")
	assert.Contains(t, entry, "INR 50000.00")
}

func TestFormat_DateConvertedToSlashFormat(t *testing.T) {
	tx := parser.ParsedTransaction{
		Date: "2025-05-14", Amount: -100, Currency: "INR",
		Merchant: "Test", SuggestedLedgerAccount: "Expenses:Test",
	}
	entry := Format(tx, "sms", "Assets:HDFC:Savings")
	assert.True(t, strings.Contains(entry, "2025/05/14"), "date must use slash format")
}

func TestAppend_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	entry := "2025/05/14 Swiggy\n    Expenses:Food:Dining    INR 2450.00\n    Assets:HDFC:Savings\n\n"
	err := Append(dir, entry)
	assert.NoError(t, err)

	data, _ := os.ReadFile(filepath.Join(dir, "auto-import.ledger"))
	assert.Contains(t, string(data), "Swiggy")

	_ = Append(dir, "2025/05/15 Amazon\n    Expenses:Shopping    INR 500.00\n    Assets:HDFC:Savings\n\n")
	data, _ = os.ReadFile(filepath.Join(dir, "auto-import.ledger"))
	assert.Contains(t, string(data), "Swiggy")
	assert.Contains(t, string(data), "Amazon")
}

func TestTriggerSync_CallsPaisaSync(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/sync" && r.Method == "POST" {
			called = true
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	err := TriggerSync(srv.URL, "")
	assert.NoError(t, err)
	assert.True(t, called)
}

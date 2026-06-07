// internal/agent/telegram/format_test.go
package telegram_test

import (
	"testing"
	"github.com/ananthakumaran/paisa/internal/agent/ledger"
	"github.com/ananthakumaran/paisa/internal/agent/telegram"
	"github.com/stretchr/testify/assert"
)

var testEntry = ledger.Entry{
	Date: "2026/06/03",
	Desc: "Food Swiggy",
	Src:  "Assets:Checking:FC2148",
	Amt:  "-215.00 INR",
	Dest: "Expenses:Food:Hyd",
}

func TestFormatDraft(t *testing.T) {
	msg := telegram.FormatDraft(testEntry)
	assert.Contains(t, msg, "📨 New Transaction")
	assert.Contains(t, msg, "desc: Food Swiggy")
	assert.Contains(t, msg, "date: 2026/06/03")
	assert.Contains(t, msg, "src:  Assets:Checking:FC2148")
	assert.Contains(t, msg, "amt:  -215.00 INR")
	assert.Contains(t, msg, "dest: Expenses:Food:Hyd")
}

func TestFormatEditTemplate(t *testing.T) {
	msg := telegram.FormatEditTemplate(testEntry)
	assert.Contains(t, msg, "Edit and send back")
	assert.Contains(t, msg, "desc: Food Swiggy")
}

func TestParseEditReply_PartialUpdate(t *testing.T) {
	reply := "desc: Groceries Zepto\ndest: Expenses:Groceries:Hyd"
	result := telegram.ParseEditReply(reply, testEntry)
	assert.Equal(t, "Groceries Zepto", result.Desc)
	assert.Equal(t, "Expenses:Groceries:Hyd", result.Dest)
	// Unchanged fields preserved
	assert.Equal(t, "2026/06/03", result.Date)
	assert.Equal(t, "Assets:Checking:FC2148", result.Src)
	assert.Equal(t, "-215.00 INR", result.Amt)
}

func TestParseEditReply_AllFields(t *testing.T) {
	reply := "desc: New Desc\ndate: 2026/06/10\nsrc: Liabilities:CC:HDFC2527\namt: -999.00 INR\ndest: Expenses:Utils:Hyd"
	result := telegram.ParseEditReply(reply, testEntry)
	assert.Equal(t, "New Desc", result.Desc)
	assert.Equal(t, "2026/06/10", result.Date)
	assert.Equal(t, "Liabilities:CC:HDFC2527", result.Src)
	assert.Equal(t, "-999.00 INR", result.Amt)
	assert.Equal(t, "Expenses:Utils:Hyd", result.Dest)
}

func TestParseEditReply_IgnoresBlankLines(t *testing.T) {
	reply := "\ndesc: Updated\n\n"
	result := telegram.ParseEditReply(reply, testEntry)
	assert.Equal(t, "Updated", result.Desc)
	assert.Equal(t, testEntry.Date, result.Date)
}

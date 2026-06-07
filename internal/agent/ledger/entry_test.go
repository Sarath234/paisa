// internal/agent/ledger/entry_test.go
package ledger_test

import (
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/ledger"
	"github.com/stretchr/testify/assert"
)

func TestEntryFormat_Debit(t *testing.T) {
	e := ledger.Entry{
		Date: "2026/06/03",
		Desc: "Food Swiggy",
		Src:  "Assets:Checking:FC2148",
		Amt:  "-215.00 INR",
		Dest: "Expenses:Food:Hyd",
	}
	got := e.Format()
	assert.Contains(t, got, "2026/06/03 Food Swiggy")
	assert.Contains(t, got, "Assets:Checking:FC2148")
	assert.Contains(t, got, "-215.00 INR")
	assert.Contains(t, got, "Expenses:Food:Hyd")
}

func TestEntryFormat_Credit(t *testing.T) {
	e := ledger.Entry{
		Date: "2026/06/01",
		Desc: "Rent from Haritha",
		Src:  "Assets:Checking:AXIS6386",
		Amt:  "30000.00 INR",
		Dest: "Assets:Checking:AXISHARITHA",
	}
	got := e.Format()
	assert.Contains(t, got, "2026/06/01 Rent from Haritha")
	assert.Contains(t, got, "30000.00 INR")
}

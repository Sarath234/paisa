// internal/agent/ledger/entry.go
package ledger

import "fmt"

type Entry struct {
	Date string // "2026/06/03"
	Desc string // "Food Swiggy"
	Src  string // first posting account (= YAML destinations)
	Amt  string // "-215.00 INR"
	Dest string // second posting account, auto-balanced (= YAML src)
}

// Format returns the ledger journal block for this entry.
func (e Entry) Format() string {
	return fmt.Sprintf("%s %s\n    %-44s  %s\n    %s\n",
		e.Date, e.Desc, e.Src, e.Amt, e.Dest)
}

// internal/agent/ledger/entry.go
package ledger

import "fmt"

type Entry struct {
	Date string `json:"date"` // "2026/06/03"
	Desc string `json:"desc"` // "Food Swiggy"
	Src  string `json:"src"`  // first posting account (= YAML destinations)
	Amt  string `json:"amt"`  // "-215.00 INR"
	Dest string `json:"dest"` // second posting account, auto-balanced (= YAML src)
}

// Format returns the ledger journal block for this entry.
func (e Entry) Format() string {
	return fmt.Sprintf("%s %s\n    %-44s  %s\n    %s\n",
		e.Date, e.Desc, e.Src, e.Amt, e.Dest)
}

// internal/agent/statement/parser.go
package statement

import "time"

// Transaction is one row from a bank statement.
type Transaction struct {
	Date        time.Time
	Description string
	Debit       float64
	Credit      float64
}

// ParseResult is the output of parsing a statement PDF.
type ParseResult struct {
	Transactions   []Transaction
	ClosingBalance float64
	Account        string     // detected from statement (e.g. "AXIS6386")
	Month          time.Month // statement period month
	Year           int        // statement period year
}

// Parser extracts transactions from a bank statement PDF.
type Parser interface {
	Name() string
	// Detect returns true when emailSubject belongs to this bank/account.
	Detect(emailSubject string) bool
	// Parse extracts transactions from raw PDF bytes.
	Parse(pdfBytes []byte) (ParseResult, error)
}

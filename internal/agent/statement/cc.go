// internal/agent/statement/cc.go
package statement

import (
	"regexp"
	"time"
)

type CCTransaction struct {
	Transaction
	IsInterestOrFee bool
}

type CCResult struct {
	Last4        string
	PeriodStart  time.Time
	PeriodEnd    time.Time
	DueDate      time.Time
	TotalDue     float64
	MinDue       float64
	Transactions []CCTransaction
}

type CCParser interface {
	Name() string
	// Parse takes already-decrypted PDF bytes.
	Parse(pdfBytes []byte) (CCResult, error)
}

var interestRe = regexp.MustCompile(`(?i)interest|late\s+(payment\s+)?fee|finance\s+charge|overlimit|gst\s+on\s+fee|service\s+charge`)

func interestOrFee(desc string) bool { return interestRe.MatchString(desc) }

// internal/agent/statement/icici_cc.go
package statement

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ICICICCParser parses ICICI credit card statements (text layer).
type ICICICCParser struct{}

func (p *ICICICCParser) Name() string { return "icici_cc" }

func (p *ICICICCParser) Parse(pdfBytes []byte) (CCResult, error) {
	text, err := extractPDFText(pdfBytes)
	if err != nil {
		return CCResult{}, err
	}
	return p.parseText(text)
}

var (
	iciciLast4  = regexp.MustCompile(`Card Number:.*?(\d{4})\b`)
	iciciPeriod = regexp.MustCompile(`Statement Period:\s*(\d{2}/\d{2}/\d{4})\s*to\s*(\d{2}/\d{2}/\d{4})`)
	iciciDue    = regexp.MustCompile(`Payment Due Date:\s*(\d{2}/\d{2}/\d{4})`)
	iciciTotal  = regexp.MustCompile(`Total Amount Due:\s*([\d,]+\.?\d*)`)
	iciciMin    = regexp.MustCompile(`Minimum Amount Due:\s*([\d,]+\.?\d*)`)
	iciciTxn    = regexp.MustCompile(`(?m)^(\d{2}/\d{2}/\d{4})\s+(.+?)\s+([\d,]+\.\d{2})\s+(DR|CR)\s*$`)
)

func (p *ICICICCParser) parseText(text string) (CCResult, error) {
	res := CCResult{}
	m := iciciLast4.FindStringSubmatch(text)
	if m == nil {
		return res, fmt.Errorf("icici_cc: card number not found")
	}
	res.Last4 = m[1]
	if m = iciciPeriod.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("icici_cc: statement period not found")
	}
	var err error
	if res.PeriodStart, err = time.Parse("02/01/2006", m[1]); err != nil {
		return res, fmt.Errorf("icici_cc: period start: %w", err)
	}
	if res.PeriodEnd, err = time.Parse("02/01/2006", m[2]); err != nil {
		return res, fmt.Errorf("icici_cc: period end: %w", err)
	}
	if m = iciciDue.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("icici_cc: due date not found")
	}
	if res.DueDate, err = time.Parse("02/01/2006", m[1]); err != nil {
		return res, fmt.Errorf("icici_cc: due date: %w", err)
	}
	if m = iciciTotal.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("icici_cc: total due not found")
	}
	res.TotalDue = parseIndianFloat(m[1])
	if m = iciciMin.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("icici_cc: minimum due not found")
	}
	res.MinDue = parseIndianFloat(m[1])

	for _, row := range iciciTxn.FindAllStringSubmatch(text, -1) {
		date, err := time.Parse("02/01/2006", row[1])
		if err != nil {
			continue
		}
		amt := parseIndianFloat(row[3])
		tx := CCTransaction{Transaction: Transaction{Date: date, Description: strings.TrimSpace(row[2])}}
		if row[4] == "CR" {
			tx.Credit = amt
		} else {
			tx.Debit = amt
		}
		tx.IsInterestOrFee = interestOrFee(tx.Description)
		res.Transactions = append(res.Transactions, tx)
	}
	return res, nil
}

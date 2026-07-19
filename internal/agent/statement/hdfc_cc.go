// internal/agent/statement/hdfc_cc.go
package statement

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// HDFCCCParser parses HDFC credit card statements (text layer).
type HDFCCCParser struct{}

func (p *HDFCCCParser) Name() string { return "hdfc_cc" }

func (p *HDFCCCParser) Parse(pdfBytes []byte) (CCResult, error) {
	text, err := extractPDFText(pdfBytes)
	if err != nil {
		return CCResult{}, err
	}
	return p.parseText(text)
}

var (
	hdfcLast4  = regexp.MustCompile(`Card Number\s*.*?(\d{4})\b`)
	hdfcPeriod = regexp.MustCompile(`Opening Period\s*(\d{2}/\d{2}/\d{4})\s*To\s*(\d{2}/\d{2}/\d{4})`)
	hdfcDue    = regexp.MustCompile(`Payment Due Date:\s*(\d{2}/\d{2}/\d{4})`)
	hdfcTotal  = regexp.MustCompile(`Total Dues\s*([\d,]+\.?\d*)`)
	hdfcMin    = regexp.MustCompile(`Minimum Amount Due\s*([\d,]+\.?\d*)`)
	hdfcTxn    = regexp.MustCompile(`(?m)^(\d{2}/\d{2}/\d{4})\s+(.+?)\s+([\d,]+\.\d{2})\s*(Cr)?\s*$`)
)

func (p *HDFCCCParser) parseText(text string) (CCResult, error) {
	res := CCResult{}
	m := hdfcLast4.FindStringSubmatch(text)
	if m == nil {
		return res, fmt.Errorf("hdfc_cc: card number not found")
	}
	res.Last4 = m[1]
	if m = hdfcPeriod.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("hdfc_cc: statement period not found")
	}
	var err error
	if res.PeriodStart, err = time.Parse("02/01/2006", m[1]); err != nil {
		return res, fmt.Errorf("hdfc_cc: period start: %w", err)
	}
	if res.PeriodEnd, err = time.Parse("02/01/2006", m[2]); err != nil {
		return res, fmt.Errorf("hdfc_cc: period end: %w", err)
	}
	if m = hdfcDue.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("hdfc_cc: due date not found")
	}
	if res.DueDate, err = time.Parse("02/01/2006", m[1]); err != nil {
		return res, fmt.Errorf("hdfc_cc: due date: %w", err)
	}
	if m = hdfcTotal.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("hdfc_cc: total due not found")
	}
	res.TotalDue = parseIndianFloat(m[1])
	if m = hdfcMin.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("hdfc_cc: minimum due not found")
	}
	res.MinDue = parseIndianFloat(m[1])

	for _, row := range hdfcTxn.FindAllStringSubmatch(text, -1) {
		date, err := time.Parse("02/01/2006", row[1])
		if err != nil {
			continue
		}
		amt := parseIndianFloat(row[3])
		tx := CCTransaction{Transaction: Transaction{Date: date, Description: strings.TrimSpace(row[2])}}
		if row[4] == "Cr" {
			tx.Credit = amt
		} else {
			tx.Debit = amt
		}
		tx.IsInterestOrFee = interestOrFee(tx.Description)
		res.Transactions = append(res.Transactions, tx)
	}
	return res, nil
}

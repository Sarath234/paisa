// internal/agent/statement/axis_cc.go
package statement

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// AxisCCParser parses Axis credit card statements (text layer).
type AxisCCParser struct{}

func (p *AxisCCParser) Name() string { return "axis_cc" }

func (p *AxisCCParser) Parse(pdfBytes []byte) (CCResult, error) {
	text, err := extractPDFText(pdfBytes)
	if err != nil {
		return CCResult{}, err
	}
	return p.parseText(text)
}

var (
	axisCCLast4  = regexp.MustCompile(`Card No:.*?(\d{4})\b`)
	axisCCPeriod = regexp.MustCompile(`Statement Period\s*(\d{2}/\d{2}/\d{4})\s*-\s*(\d{2}/\d{2}/\d{4})`)
	axisCCDue    = regexp.MustCompile(`Payment Due Date\s*(\d{2}/\d{2}/\d{4})`)
	axisCCTotal  = regexp.MustCompile(`Total Payment Due\s*([\d,]+\.?\d*)`)
	axisCCMin    = regexp.MustCompile(`Minimum Payment Due\s*([\d,]+\.?\d*)`)
	axisCCTxn    = regexp.MustCompile(`(?m)^(\d{2}/\d{2}/\d{4})\s+(.+?)\s+(Debit|Credit)\s+([\d,]+\.\d{2})\s*$`)
)

func (p *AxisCCParser) parseText(text string) (CCResult, error) {
	res := CCResult{}
	m := axisCCLast4.FindStringSubmatch(text)
	if m == nil {
		return res, fmt.Errorf("axis_cc: card number not found")
	}
	res.Last4 = m[1]
	if m = axisCCPeriod.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("axis_cc: statement period not found")
	}
	var err error
	if res.PeriodStart, err = time.Parse("02/01/2006", m[1]); err != nil {
		return res, fmt.Errorf("axis_cc: period start: %w", err)
	}
	if res.PeriodEnd, err = time.Parse("02/01/2006", m[2]); err != nil {
		return res, fmt.Errorf("axis_cc: period end: %w", err)
	}
	if m = axisCCDue.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("axis_cc: due date not found")
	}
	if res.DueDate, err = time.Parse("02/01/2006", m[1]); err != nil {
		return res, fmt.Errorf("axis_cc: due date: %w", err)
	}
	if m = axisCCTotal.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("axis_cc: total due not found")
	}
	res.TotalDue = parseIndianFloat(m[1])
	if m = axisCCMin.FindStringSubmatch(text); m == nil {
		return res, fmt.Errorf("axis_cc: minimum due not found")
	}
	res.MinDue = parseIndianFloat(m[1])

	for _, row := range axisCCTxn.FindAllStringSubmatch(text, -1) {
		date, err := time.Parse("02/01/2006", row[1])
		if err != nil {
			continue
		}
		amt := parseIndianFloat(row[4])
		tx := CCTransaction{Transaction: Transaction{Date: date, Description: strings.TrimSpace(row[2])}}
		if row[3] == "Credit" {
			tx.Credit = amt
		} else {
			tx.Debit = amt
		}
		tx.IsInterestOrFee = interestOrFee(tx.Description)
		res.Transactions = append(res.Transactions, tx)
	}
	return res, nil
}

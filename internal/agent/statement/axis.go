// internal/agent/statement/axis.go
package statement

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// AxisParser parses Axis Bank savings account statement PDFs.
type AxisParser struct{}

func (p *AxisParser) Name() string { return "axis_savings" }

func (p *AxisParser) Detect(subject string) bool {
	s := strings.ToLower(subject)
	return strings.Contains(s, "axis") || strings.Contains(s, "6386")
}

func (p *AxisParser) Parse(pdfBytes []byte) (ParseResult, error) {
	text, err := extractPDFText(pdfBytes)
	if err != nil {
		return ParseResult{}, fmt.Errorf("axis: extract text: %w", err)
	}
	return p.parseText(text)
}

// extractPDFText extracts all text from PDF bytes using ledongthuc/pdf.
func extractPDFText(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		pg := r.Page(i)
		if pg.V.IsNull() {
			continue
		}
		text, err := pg.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

var (
	// Matches the statement period line: "from DD-MM-YYYY to DD-MM-YYYY"
	periodRe = regexp.MustCompile(`from\s+(\d{2}-\d{2}-\d{4})\s+to\s+(\d{2}-\d{2}-\d{4})`)

	// Matches a transaction row starting with a date.
	// Groups: 1=date, 2=rest_of_line
	txnRe = regexp.MustCompile(`(\d{2}-\d{2}-\d{4})\s+\d{2}-\d{2}-\d{4}\s+(.+)`)

	// Extracts all decimal numbers (Indian format with commas).
	amountRe = regexp.MustCompile(`[\d,]+\.\d{2}`)

	// Section terminators: stop parsing transactions when these headers appear.
	sectionEndRe = regexp.MustCompile(`(?i)Credit Cards?|Deposits?\s+\(`)
)

const axisDateLayout = "02-01-2006"

// parseText is package-internal for unit testing without a real PDF.
func (p *AxisParser) parseText(text string) (ParseResult, error) {
	var result ParseResult

	// Extract period.
	if m := periodRe.FindStringSubmatch(text); len(m) == 3 {
		end, err := time.Parse(axisDateLayout, m[2])
		if err == nil {
			result.Month = end.Month()
			result.Year = end.Year()
		}
	}

	// Find the savings account section and stop at next major section.
	savingsIdx := strings.Index(strings.ToLower(text), "savings account")
	if savingsIdx < 0 {
		savingsIdx = 0 // fall back to whole text
	}
	section := text[savingsIdx:]
	if loc := sectionEndRe.FindStringIndex(section); loc != nil {
		section = section[:loc[0]]
	}

	var closingBalance float64
	lines := strings.Split(section, "\n")
	for _, line := range lines {
		m := txnRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		dateStr := m[1]
		rest := m[2]

		txDate, err := time.Parse(axisDateLayout, dateStr)
		if err != nil {
			continue
		}

		amounts := amountRe.FindAllString(rest, -1)
		if len(amounts) < 3 {
			continue // need at least withdrawal, credit, balance
		}

		// Last 3 numbers: withdrawal, credit, balance.
		n := len(amounts)
		withdrawal := parseIndianFloat(amounts[n-3])
		credit := parseIndianFloat(amounts[n-2])
		balance := parseIndianFloat(amounts[n-1])
		closingBalance = balance

		// Description is everything before the first numeric in rest.
		firstNum := amountRe.FindStringIndex(rest)
		desc := strings.TrimSpace(rest)
		if firstNum != nil {
			desc = strings.TrimSpace(rest[:firstNum[0]])
		}

		result.Transactions = append(result.Transactions, Transaction{
			Date:        txDate,
			Description: desc,
			Debit:       withdrawal,
			Credit:      credit,
		})
	}

	result.ClosingBalance = closingBalance
	return result, nil
}

// parseIndianFloat converts "9,24,764.20" → 924764.20.
func parseIndianFloat(s string) float64 {
	s = strings.ReplaceAll(s, ",", "")
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

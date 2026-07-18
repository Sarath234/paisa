// Package notices extracts bill facts from bank notice SMSes (statement
// generated, payment received) — as opposed to transaction SMSes, which are
// handled by the parser package. Patterns are per-bank and calibrated
// against real messages.
package notices

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type StatementNotice struct {
	Last4         string
	StatementDate time.Time
	TotalDue      float64
	MinDue        float64
	DueDate       time.Time
}

// statementPreamble marks an SMS as a statement notice (any bank).
var statementPreamble = regexp.MustCompile(`(?i)(credit\s+card).{0,80}(statement)|(statement).{0,80}(credit\s+card)`)

// statementAmountDue marks the presence of "total/min due" language.
var statementAmountDue = regexp.MustCompile(`(?i)(total|min).{0,20}due`)

// LooksLikeStatement is the cheap router Match check.
func LooksLikeStatement(sms string) bool {
	return statementPreamble.MatchString(sms) &&
		statementAmountDue.MatchString(sms)
}

var (
	reLast4    = regexp.MustCompile(`(?i)(?:XX|ending\s*|no\.\s*XX?)(\d{4})`)
	reStmtDate = regexp.MustCompile(`(?i)(?:statement|card).{0,100}?dated\s+([0-9]{2}[-/][A-Za-z0-9]{2,3}[-/][0-9]{2,4})`)
	reTotal    = regexp.MustCompile(`(?i)total\s+(?:amount\s+|amt\s+)?due\s*:?\s*(?:Rs\.?|INR)\s*([\d,]+\.?\d*)`)
	reMin      = regexp.MustCompile(`(?i)min(?:imum)?\s+(?:amount\s+|amt\s+)?due\s*:?\s*(?:Rs\.?|INR)\s*([\d,]+\.?\d*)`)
	reDue      = regexp.MustCompile(`(?i)(?:due\s+date|pay\s+by)\s*:?\s*([0-9]{2}[-/][A-Za-z0-9]{2,3}[-/][0-9]{2,4})`)
)

// ExtractStatement parses a statement-generated notice.
// (nil, nil) = not a statement notice. (nil, err) = recognized but a field
// failed; err names the field so the Telegram reply is actionable.
func ExtractStatement(sms string) (*StatementNotice, error) {
	if !LooksLikeStatement(sms) {
		return nil, nil
	}
	n := &StatementNotice{}
	m := reLast4.FindStringSubmatch(sms)
	if m == nil {
		return nil, fmt.Errorf("card last-4 not found")
	}
	n.Last4 = m[1]
	if m = reStmtDate.FindStringSubmatch(sms); m == nil {
		return nil, fmt.Errorf("statement date not found")
	}
	d, err := parseNoticeDate(m[1])
	if err != nil {
		return nil, fmt.Errorf("statement date: %w", err)
	}
	n.StatementDate = d
	if m = reTotal.FindStringSubmatch(sms); m == nil {
		return nil, fmt.Errorf("total due not found")
	}
	n.TotalDue = parseAmount(m[1])
	if m = reMin.FindStringSubmatch(sms); m == nil {
		return nil, fmt.Errorf("minimum due not found")
	}
	n.MinDue = parseAmount(m[1])
	if m = reDue.FindStringSubmatch(sms); m == nil {
		return nil, fmt.Errorf("due date not found")
	}
	if d, err = parseNoticeDate(m[1]); err != nil {
		return nil, fmt.Errorf("due date: %w", err)
	}
	n.DueDate = d
	return n, nil
}

// parseNoticeDate handles 10-Jul-26, 10-Jul-2026, 19-07-2026, 14/07/2026.
func parseNoticeDate(s string) (time.Time, error) {
	s = strings.ReplaceAll(s, "/", "-")
	for _, layout := range []string{"02-Jan-06", "02-Jan-2006", "02-01-2006", "02-01-06"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable date %q", s)
}

func parseAmount(s string) float64 {
	f, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return f
}

type PaymentNotice struct {
	Last4  string
	Amount float64
	Date   time.Time
}

// paymentPreamble: "payment ... received" or "received payment ... towards
// your ... credit card". Excludes statement notices (checked first by caller,
// but keep the patterns disjoint anyway).
var paymentPreamble = regexp.MustCompile(`(?i)payment\s+of\s+(?:Rs\.?|INR)|received\s+payment\s+of\s+(?:Rs\.?|INR)`)

func LooksLikePayment(sms string) bool {
	return paymentPreamble.MatchString(sms) &&
		regexp.MustCompile(`(?i)credit\s+card`).MatchString(sms) &&
		!LooksLikeStatement(sms)
}

var (
	rePayAmt  = regexp.MustCompile(`(?i)payment\s+of\s+(?:Rs\.?|INR)\s*([\d,]+\.?\d*)`)
	rePayDate = regexp.MustCompile(`(?i)on\s+([0-9]{2}[-/][A-Za-z0-9]{2,3}[-/][0-9]{2,4})\b\.?`)
)

func ExtractPayment(sms string) (*PaymentNotice, error) {
	if !LooksLikePayment(sms) {
		return nil, nil
	}
	n := &PaymentNotice{}
	m := reLast4.FindStringSubmatch(sms)
	if m == nil {
		return nil, fmt.Errorf("card last-4 not found")
	}
	n.Last4 = m[1]
	if m = rePayAmt.FindStringSubmatch(sms); m == nil {
		return nil, fmt.Errorf("payment amount not found")
	}
	n.Amount = parseAmount(m[1])
	// date: take the LAST date-looking token (the "on <date>" nearest the end;
	// some banks put the card number pattern XXnnnn earlier which never matches).
	all := rePayDate.FindAllStringSubmatch(sms, -1)
	if len(all) == 0 {
		return nil, fmt.Errorf("payment date not found")
	}
	d, err := parseNoticeDate(all[len(all)-1][1])
	if err != nil {
		return nil, fmt.Errorf("payment date: %w", err)
	}
	n.Date = d
	return n, nil
}

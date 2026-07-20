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
	// reTotal/reMin: the amount/amt/due keywords and the "Dr." debit-indicator
	// token are all optional-with-flexible-whitespace because bank wording
	// varies on which of them appear and whether a space or colon separates
	// them (e.g. ICICI's "Total Amount Due Rs 23,450.50" vs Axis's
	// "Total amt: INR  Dr. 24,567.89" — the latter has no "due" on the
	// Total line and a colon with no preceding space).
	reTotal = regexp.MustCompile(`(?i)total\s+(?:amount|amt)?\s*(?:due)?\s*:?\s*(?:Rs\.?|INR)\s*(?:Dr\.?\s*)?([\d,]+\.?\d*)`)
	reMin   = regexp.MustCompile(`(?i)min(?:imum)?\s+(?:amount|amt)?\s*due\s*:?\s*(?:Rs\.?|INR)\s*(?:Dr\.?\s*)?([\d,]+\.?\d*)`)
	reDue   = regexp.MustCompile(`(?i)(?:due\s+date|pay\s+by|due\s+on)\s*:?\s*([0-9]{2}[-/][A-Za-z0-9]{2,3}[-/][0-9]{2,4})`)
)

// ExtractStatement parses a statement-generated notice.
// (nil, nil) = not a statement notice. (nil, err) = recognized but a
// mandatory field failed; err names the field so the Telegram reply is
// actionable. StatementDate is the one OPTIONAL field: some banks' notices
// (e.g. Axis's "is generated" wording) never state it, only a due date.
// When absent, StatementDate is left zero — Capability.handleStatement
// substitutes the notice's receipt date as an approximation.
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

	if sm := reStmtDate.FindStringSubmatch(sms); sm != nil {
		d, err := parseNoticeDate(sm[1])
		if err != nil {
			return nil, fmt.Errorf("statement date: %w", err)
		}
		n.StatementDate = d
	}

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
	d, err := parseNoticeDate(m[1])
	if err != nil {
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

// paymentPreamble: "payment of Rs/INR ..." — also covers "received payment
// of ...". Excludes statement notices (checked first by caller, but keep the
// patterns disjoint anyway).
var paymentPreamble = regexp.MustCompile(`(?i)payment\s+of\s+(?:Rs\.?|INR)`)

var creditCardRe = regexp.MustCompile(`(?i)credit\s+card`)

// receiveDirection requires language that says money came IN against the
// card balance ("received", "credited", "towards your ... card"). Without
// this, "Payment of Rs.1200 made using your Credit Card XX1234 at MERCHANT"
// — a SPEND alert phrased with "payment of" — would be misread as a
// payment: it would fake PaidDate (killing cc_due reminders for a bill
// that isn't actually paid) and the spend itself would never be recorded.
var receiveDirection = regexp.MustCompile(`(?i)received|credited|towards\s+your`)

// spendPhrasing rejects spend-direction wording even when receiveDirection
// also happens to match (e.g. "received" appearing elsewhere in a longer
// message). Kept intentionally simple per the spend-alert shapes seen.
var spendPhrasing = regexp.MustCompile(`(?i)made\s+using|using\s+your|spent`)

func LooksLikePayment(sms string) bool {
	return paymentPreamble.MatchString(sms) &&
		creditCardRe.MatchString(sms) &&
		receiveDirection.MatchString(sms) &&
		!spendPhrasing.MatchString(sms) &&
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

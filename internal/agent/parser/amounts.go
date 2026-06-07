// internal/agent/parser/amounts.go
package parser

import (
	"fmt"
	"regexp"
	"strings"
)

var amountRe = regexp.MustCompile(`(?i)(?:INR\.?|Rs\.?)\s*([\d,]+(?:\.\d{1,2})?)`)

// NormaliseAmount strips Indian commas and ensures exactly 2 decimal places.
func NormaliseAmount(raw string) (string, error) {
	s := strings.ReplaceAll(raw, ",", "")
	if !strings.Contains(s, ".") {
		return s + ".00", nil
	}
	parts := strings.SplitN(s, ".", 2)
	switch len(parts[1]) {
	case 0:
		return parts[0] + ".00", nil
	case 1:
		return s + "0", nil
	default:
		return parts[0] + "." + parts[1][:2], nil
	}
}

// FormatEntryAmount returns the amount string for an Entry (e.g. "-215.00 INR").
// isDebit=true adds a negative sign; false gives a plain positive amount.
func FormatEntryAmount(normalised string, isDebit bool) string {
	if isDebit {
		return fmt.Sprintf("-%s INR", normalised)
	}
	return fmt.Sprintf("%s INR", normalised)
}

// ExtractAmountFromSMS finds the first INR/Rs amount in an SMS and detects debit/credit.
// Used by fixed-route parser where no bank-specific regex is available.
func ExtractAmountFromSMS(sms string) (normalised string, isDebit bool, err error) {
	m := amountRe.FindStringSubmatch(sms)
	if m == nil {
		return "", false, fmt.Errorf("no INR/Rs amount found in SMS")
	}
	norm, err := NormaliseAmount(m[1])
	if err != nil {
		return "", false, err
	}
	lower := strings.ToLower(sms)
	isDebit = strings.Contains(lower, "debited") ||
		strings.Contains(lower, "debit") ||
		strings.Contains(lower, "spent")
	return norm, isDebit, nil
}

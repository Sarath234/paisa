// internal/agent/parser/parser.go
package parser

import (
	"fmt"
	"strings"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/ledger"
)

// Classify scans the accounts list top-to-bottom and returns the first rule
// where ALL identifiers appear in the SMS. Fixed routes win because they are
// listed first in the YAML.
func Classify(sms string, accounts []config.AccountRule) (*config.AccountRule, error) {
	for i, rule := range accounts {
		if matchesAll(sms, rule.Identifiers) {
			return &accounts[i], nil
		}
	}
	return nil, fmt.Errorf("no matching account rule for SMS")
}

func matchesAll(sms string, identifiers []string) bool {
	for _, id := range identifiers {
		if !strings.Contains(sms, id) {
			return false
		}
	}
	return true
}

// Parse builds a ledger Entry from an SMS given the matched AccountRule.
// Fixed routes derive all fields from the YAML rule + generic amount/date extractor.
// Format routes call the bank-specific extractor then apply merchant routing.
func Parse(sms string, rule *config.AccountRule, merchants []config.MerchantRule) (*ledger.Entry, error) {
	if rule.Bank == "fixed" {
		return parseFixed(sms, rule)
	}
	return parseFormat(sms, rule, merchants)
}

func parseFixed(sms string, rule *config.AccountRule) (*ledger.Entry, error) {
	rawAmt, _, err := ExtractAmountFromSMS(sms)
	if err != nil {
		return nil, fmt.Errorf("fixed route: %w", err)
	}
	norm, err := NormaliseAmount(rawAmt)
	if err != nil {
		return nil, err
	}
	rawDate, err := ExtractDateFromSMS(sms)
	if err != nil {
		return nil, fmt.Errorf("fixed route: %w", err)
	}
	date, err := NormaliseDate(rawDate)
	if err != nil {
		return nil, err
	}
	// Fixed routes always use positive amount on destinations:
	// the sign is implied by the account pair (e.g. CC gets payment = positive).
	return &ledger.Entry{
		Date: date,
		Desc: rule.Description,
		Src:  rule.Destinations,
		Amt:  FormatEntryAmount(norm, false),
		Dest: rule.Src,
	}, nil
}

func parseFormat(sms string, rule *config.AccountRule, merchants []config.MerchantRule) (*ledger.Entry, error) {
	var merchant, rawDate, rawAmt string
	var isDebit bool
	var err error

	switch rule.Bank {
	case "icici_cc":
		merchant, rawDate, rawAmt, isDebit, err = ExtractIciciCC(sms)
	case "hdfc_debit":
		merchant, rawDate, rawAmt, isDebit, err = ExtractHdfcDebit(sms)
	case "hdfc_cc":
		merchant, rawDate, rawAmt, isDebit, err = ExtractHdfcCC(sms)
	case "axis_checking":
		merchant, rawDate, rawAmt, isDebit, err = ExtractAxisChecking(sms)
	case "axis_cc":
		merchant, rawDate, rawAmt, isDebit, err = ExtractAxisCC(sms)
	case "idfc_checking":
		merchant, rawDate, rawAmt, isDebit, err = ExtractIDFCChecking(sms)
	default:
		// Unknown bank: use generic extractor, LLM will fill merchant/dest
		rawAmt, isDebit, err = ExtractAmountFromSMS(sms)
		if err != nil {
			return nil, fmt.Errorf("unknown bank %q: %w", rule.Bank, err)
		}
		rawDate, err = ExtractDateFromSMS(sms)
		if err != nil {
			return nil, fmt.Errorf("unknown bank %q date: %w", rule.Bank, err)
		}
	}
	if err != nil {
		return nil, err
	}

	norm, err := NormaliseAmount(rawAmt)
	if err != nil {
		return nil, err
	}
	date, err := NormaliseDate(rawDate)
	if err != nil {
		return nil, err
	}

	account, desc := RouteMerchant(merchant, merchants)

	return &ledger.Entry{
		Date: date,
		Desc: desc,
		Src:  rule.Destinations,
		Amt:  FormatEntryAmount(norm, isDebit),
		Dest: account,
	}, nil
}

// internal/agent/parser/parser.go
package parser

import (
	"fmt"
	"strings"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/ledger"
	log "github.com/sirupsen/logrus"
)

// Classify scans the accounts list top-to-bottom and returns the first rule
// where ALL identifiers appear in the SMS. Fixed routes win because they are
// listed first in the YAML.
func Classify(sms string, accounts []config.AccountRule) (*config.AccountRule, error) {
	log.Debugf("classify: checking %d account rules", len(accounts))
	for i, rule := range accounts {
		missing := missingIdentifiers(sms, rule.Identifiers)
		if len(missing) == 0 {
			log.Infof("classify: matched rule bank=%q destinations=%q", rule.Bank, rule.Destinations)
			return &accounts[i], nil
		}
		log.Debugf("classify: rule bank=%q skipped — missing identifiers: %v", rule.Bank, missing)
	}
	log.Warnf("classify: no rule matched (SMS length=%d)", len(sms))
	return nil, fmt.Errorf("no matching account rule for SMS")
}

// missingIdentifiers returns which identifiers from the rule are absent in the SMS.
func missingIdentifiers(sms string, identifiers []string) []string {
	var missing []string
	for _, id := range identifiers {
		if !strings.Contains(sms, id) {
			missing = append(missing, id)
		}
	}
	return missing
}

func matchesAll(sms string, identifiers []string) bool {
	return len(missingIdentifiers(sms, identifiers)) == 0
}

// Parse builds a ledger Entry from an SMS given the matched AccountRule.
// Fixed routes derive all fields from the YAML rule + generic amount/date extractor.
// Format routes call the bank-specific extractor then apply merchant routing.
func Parse(sms string, rule *config.AccountRule, merchants []config.MerchantRule) (*ledger.Entry, error) {
	if rule.Bank == "fixed" {
		log.Debugf("parse: taking fixed route (desc=%q dest=%q)", rule.Description, rule.Src)
		return parseFixed(sms, rule)
	}
	log.Debugf("parse: taking format route for bank=%q", rule.Bank)
	return parseFormat(sms, rule, merchants)
}

func parseFixed(sms string, rule *config.AccountRule) (*ledger.Entry, error) {
	rawAmt, _, err := ExtractAmountFromSMS(sms)
	if err != nil {
		log.Warnf("parse/fixed: amount extraction failed: %v", err)
		return nil, fmt.Errorf("fixed route: %w", err)
	}
	norm, err := NormaliseAmount(rawAmt)
	if err != nil {
		return nil, err
	}
	rawDate, err := ExtractDateFromSMS(sms)
	if err != nil {
		log.Warnf("parse/fixed: date extraction failed: %v", err)
		return nil, fmt.Errorf("fixed route: %w", err)
	}
	date, err := NormaliseDate(rawDate)
	if err != nil {
		return nil, err
	}
	log.Debugf("parse/fixed: rawAmt=%q→%q rawDate=%q→%q", rawAmt, norm, rawDate, date)
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
		log.Debugf("parse/format: unknown bank %q — using generic extractor", rule.Bank)
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
		log.Warnf("parse/format bank=%q: extraction failed: %v", rule.Bank, err)
		return nil, err
	}

	log.Debugf("parse/format bank=%q: merchant=%q rawDate=%q rawAmt=%q isDebit=%v",
		rule.Bank, merchant, rawDate, rawAmt, isDebit)

	norm, err := NormaliseAmount(rawAmt)
	if err != nil {
		log.Warnf("parse/format bank=%q: amount normalise failed rawAmt=%q: %v", rule.Bank, rawAmt, err)
		return nil, err
	}
	date, err := NormaliseDate(rawDate)
	if err != nil {
		log.Warnf("parse/format bank=%q: date normalise failed rawDate=%q: %v", rule.Bank, rawDate, err)
		return nil, err
	}

	account, desc := RouteMerchant(merchant, merchants)
	if account == "" {
		log.Debugf("parse/format: merchant=%q — no rule matched, LLM will fill dest+desc", merchant)
	} else {
		log.Debugf("parse/format: merchant=%q → account=%q desc=%q", merchant, account, desc)
	}

	return &ledger.Entry{
		Date: date,
		Desc: desc,
		Src:  rule.Destinations,
		Amt:  FormatEntryAmount(norm, isDebit),
		Dest: account,
	}, nil
}

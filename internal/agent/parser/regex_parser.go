package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/config"
)

var (
	reDateDMY4   = regexp.MustCompile(`\bon\s+(\d{2})-(\d{2})-(\d{4})\b`)
	reDateDMY2   = regexp.MustCompile(`\b(\d{2})-(\d{2})-(\d{2})\b`)
	reDateMonDY  = regexp.MustCompile(`(?i)\bon\s+([A-Za-z]{3})\s+(\d{1,2}),\s*(\d{4})\b`)
	reDateDMONY2 = regexp.MustCompile(`(?i)\b(\d{2})(JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC)(\d{2})?\b`)
	reDateDMONY4 = regexp.MustCompile(`(?i)\bon\s+(\d{1,2})\s+(JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC),?\s*(\d{4})?\b`)
	reDateSlash  = regexp.MustCompile(`\bon\s+(\d{2})/(\d{2})/(\d{4})\b`)
	reDateFull   = regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2}),\s*(\d{4})\b`)

	reAmountRs  = regexp.MustCompile(`(?i)Rs\.?\s*([0-9,]+(?:\.[0-9]+)?)`)
	reAmountINR = regexp.MustCompile(`(?i)INR\s*([0-9,]+(?:\.[0-9]+)?)`)
	reTime      = regexp.MustCompile(`\b(\d{2}):(\d{2})(?::\d{2})?\b`)
)

var shortMonths = map[string]string{
	"jan": "01", "feb": "02", "mar": "03", "apr": "04",
	"may": "05", "jun": "06", "jul": "07", "aug": "08",
	"sep": "09", "oct": "10", "nov": "11", "dec": "12",
}

var fullMonths = map[string]string{
	"january": "01", "february": "02", "march": "03", "april": "04",
	"may": "05", "june": "06", "july": "07", "august": "08",
	"september": "09", "october": "10", "november": "11", "december": "12",
}

func centuryPrefix() string {
	return strconv.Itoa(time.Now().Year())[:2]
}

func lpad(s string) string {
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

func parseDate(msg string) (string, bool) {
	pfx := centuryPrefix()

	// Pattern 1: on DD-MM-YYYY
	if m := reDateDMY4.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("%s-%s-%s", m[3], lpad(m[2]), lpad(m[1])), true
	}
	// Pattern 2: DD-MM-YY (2-digit year)
	if m := reDateDMY2.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("%s%s-%s-%s", pfx, m[3], lpad(m[2]), lpad(m[1])), true
	}
	// Pattern 3: on Mon DD, YYYY
	if m := reDateMonDY.FindStringSubmatch(msg); m != nil {
		if mon, ok := shortMonths[strings.ToLower(m[1])]; ok {
			return fmt.Sprintf("%s-%s-%s", m[3], mon, lpad(m[2])), true
		}
	}
	// Pattern 4: DDMONYY (e.g. 14MAY25)
	if m := reDateDMONY2.FindStringSubmatch(msg); m != nil {
		if mon, ok := shortMonths[strings.ToLower(m[2])]; ok {
			year := pfx + m[3]
			if m[3] == "" {
				year = strconv.Itoa(time.Now().Year())
			}
			return fmt.Sprintf("%s-%s-%s", year, mon, lpad(m[1])), true
		}
	}
	// Pattern 5: on D MON,YYYY
	if m := reDateDMONY4.FindStringSubmatch(msg); m != nil {
		if mon, ok := shortMonths[strings.ToLower(m[2])]; ok {
			year := m[3]
			if year == "" {
				year = strconv.Itoa(time.Now().Year())
			}
			return fmt.Sprintf("%s-%s-%s", year, mon, lpad(m[1])), true
		}
	}
	// Pattern 6: on DD/MM/YYYY
	if m := reDateSlash.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("%s-%s-%s", m[3], lpad(m[2]), lpad(m[1])), true
	}
	// Pattern 7: Month D, YYYY (full month name)
	if m := reDateFull.FindStringSubmatch(msg); m != nil {
		if mon, ok := fullMonths[strings.ToLower(m[1])]; ok {
			return fmt.Sprintf("%s-%s-%s", m[3], mon, lpad(m[2])), true
		}
	}
	return "", false
}

func parseAmount(msg string) (float64, bool) {
	for _, re := range []*regexp.Regexp{reAmountRs, reAmountINR} {
		if m := re.FindStringSubmatch(msg); m != nil {
			if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64); err == nil {
				return v, true
			}
		}
	}
	return 0, false
}

func parseDayPart(msg string, dp config.DayPartsConfig) string {
	m := reTime.FindStringSubmatch(msg)
	if m == nil {
		return "Meal"
	}
	h, _ := strconv.Atoi(m[1])
	switch {
	case h < dp.BreakfastEnd:
		return "Breakfast"
	case h < dp.LunchEnd:
		return "Lunch"
	case h < dp.DinnerEnd:
		return "Dinner"
	default:
		return "Evening Snack"
	}
}

func matchMerchant(msg string, merchants []config.MerchantPattern, dayPart string) (string, string) {
	lower := strings.ToLower(msg)
	for _, m := range merchants {
		if m.Keyword == "" || strings.Contains(lower, strings.ToLower(m.Keyword)) {
			return strings.ReplaceAll(m.Description, "{day_part}", dayPart), m.Account
		}
	}
	return "Misc", "Expenses:Misc"
}

func matchSource(msg string, sources []config.SourceRule) (config.SourceRule, bool) {
	for _, s := range sources {
		matched := true
		for _, c := range s.Contains {
			if !strings.Contains(msg, c) {
				matched = false
				break
			}
		}
		if matched {
			return s, true
		}
	}
	return config.SourceRule{}, false
}

// RegexParse attempts rule-based parsing of a bank SMS/alert.
// Returns (zero, false) if no source rule matched or date/amount extraction failed.
func RegexParse(msg string, rules config.ParserRules) (ParsedTransaction, bool) {
	src, ok := matchSource(msg, rules.Sources)
	if !ok {
		return ParsedTransaction{}, false
	}
	date, ok := parseDate(msg)
	if !ok {
		return ParsedTransaction{}, false
	}
	amount, ok := parseAmount(msg)
	if !ok {
		return ParsedTransaction{}, false
	}

	var desc, destAccount string
	if src.DestAccount != "" {
		desc = src.Description
		destAccount = src.DestAccount
	} else {
		dayPart := parseDayPart(msg, rules.DayParts)
		desc, destAccount = matchMerchant(msg, rules.Merchants, dayPart)
	}

	return ParsedTransaction{
		Date:                   date,
		Amount:                 -amount,
		Currency:               "INR",
		Merchant:               desc,
		TxType:                 "debit",
		SuggestedLedgerAccount: destAccount,
		SourceAccount:          src.Account,
		Confidence:             1.0,
	}, true
}

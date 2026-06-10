// internal/agent/parser/dates.go
package parser

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// dateExtractPatterns try each pattern in order; first match wins.
var dateExtractPatterns = []*regexp.Regexp{
	// DD-Mon-YY or DD-MON-YY  (must come before DD-MM-YY to avoid month-abbrev clash)
	regexp.MustCompile(`(?i)\b(\d{2}-(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)-\d{2})\b`),
	// DD Mon,YYYY
	regexp.MustCompile(`(?i)\b(\d{1,2}\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec),\d{4})\b`),
	// YYYY-MM-DD  (must come before DD-MM-YY to avoid 4-digit year ambiguity)
	regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`),
	// DD/MM/YYYY  (must come before DD/MM/YY to avoid partial match on 4-digit year)
	regexp.MustCompile(`\b(\d{2}/\d{2}/\d{4})\b`),
	// DD-MM-YY
	regexp.MustCompile(`\b(\d{2}-\d{2}-\d{2})\b`),
	// DD/MM/YY
	regexp.MustCompile(`\b(\d{2}/\d{2}/\d{2})\b`),
}

// ExtractDateFromSMS finds the first recognisable date in an SMS string.
func ExtractDateFromSMS(sms string) (string, error) {
	for i, re := range dateExtractPatterns {
		if m := re.FindStringSubmatch(sms); m != nil {
			log.Debugf("date: pattern[%d] matched %q", i, m[1])
			return m[1], nil
		}
	}
	log.Debugf("date: no pattern matched (supported: DD-Mon-YY, DD Mon,YYYY, YYYY-MM-DD, DD/MM/YYYY, DD-MM-YY, DD/MM/YY)")
	return "", fmt.Errorf("no date found in SMS")
}

// NormaliseDate converts any supported raw date string to YYYY/MM/DD.
// Supported: DD-Mon-YY, DD Mon,YYYY, YYYY-MM-DD, DD-MM-YY, DD/MM/YY.
func NormaliseDate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	norm := normalizeMonthCase(raw)

	// DD-Mon-YY  (e.g. 03-Jun-26, 15-MAY-26)
	if t, err := time.Parse("02-Jan-06", norm); err == nil {
		return t.Format("2006/01/02"), nil
	}

	// DD Mon,YYYY  (e.g. 03 Jun,2026) — replace comma with space first
	withoutComma := strings.ReplaceAll(norm, ",", " ")
	if t, err := time.Parse("02 Jan 2006", withoutComma); err == nil {
		return t.Format("2006/01/02"), nil
	}

	// YYYY-MM-DD  (e.g. 2026-05-21)
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.Format("2006/01/02"), nil
	}

	// DD-MM-YY  (e.g. 03-06-26)
	if t, err := time.Parse("02-01-06", raw); err == nil {
		return t.Format("2006/01/02"), nil
	}

	// DD/MM/YYYY  (e.g. 08/06/2026)
	if t, err := time.Parse("02/01/2006", raw); err == nil {
		return t.Format("2006/01/02"), nil
	}

	// DD/MM/YY  (e.g. 09/04/26)
	if t, err := time.Parse("02/01/06", raw); err == nil {
		return t.Format("2006/01/02"), nil
	}

	return "", fmt.Errorf("unrecognised date format: %q", raw)
}

var monthNormRe = regexp.MustCompile(`(?i)\b(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)\b`)

// normalizeMonthCase title-cases month abbreviations so time.Parse accepts them.
func normalizeMonthCase(s string) string {
	return monthNormRe.ReplaceAllStringFunc(s, func(m string) string {
		lower := strings.ToLower(m)
		return strings.ToUpper(lower[:1]) + lower[1:]
	})
}

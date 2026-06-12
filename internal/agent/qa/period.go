// internal/agent/qa/period.go
package qa

import (
	"strings"
	"time"
)

// Period is a half-open date range [Start, End) with a human label.
type Period struct {
	Start time.Time
	End   time.Time
	Label string
}

func (p Period) IsSingleMonth() bool {
	return p.Start.AddDate(0, 1, 0).Equal(p.End)
}

func monthPeriod(year int, month time.Month, loc *time.Location) Period {
	start := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	return Period{Start: start, End: start.AddDate(0, 1, 0), Label: start.Format("Jan 2006")}
}

var monthNames = map[string]time.Month{
	"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3,
	"apr": 4, "april": 4, "may": 5, "jun": 6, "june": 6, "jul": 7, "july": 7,
	"aug": 8, "august": 8, "sep": 9, "september": 9, "oct": 10, "october": 10,
	"nov": 11, "november": 11, "dec": 12, "december": 12,
}

// ResolvePeriod turns an extracted period string into a date range.
// Unrecognized input returns the current month with ok=false so the caller
// can note the fallback in the reply.
func ResolvePeriod(s string, now time.Time) (Period, bool) {
	norm := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, "_", " ")))
	loc := now.Location()

	switch norm {
	case "", "this month", "current month":
		return monthPeriod(now.Year(), now.Month(), loc), true
	case "last month", "previous month":
		// time.Date normalizes month 0 to December of the previous year;
		// going via day 1 avoids AddDate's end-of-month overflow (Mar 31 → Mar 3).
		start := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, loc)
		return monthPeriod(start.Year(), start.Month(), loc), true
	case "this year", "current year":
		start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, loc)
		return Period{Start: start, End: start.AddDate(1, 0, 0), Label: start.Format("2006")}, true
	}

	// "may" or "may 2026"
	fields := strings.Fields(norm)
	if len(fields) >= 1 {
		if month, ok := monthNames[fields[0]]; ok {
			year := now.Year()
			if len(fields) == 2 {
				if y, err := time.Parse("2006", fields[1]); err == nil {
					year = y.Year()
				}
			} else if month > now.Month() {
				year-- // bare future month means the most recent occurrence; the current month stays in the current year
			}
			return monthPeriod(year, month, loc), true
		}
	}

	return monthPeriod(now.Year(), now.Month(), loc), false
}

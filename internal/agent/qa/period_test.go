// internal/agent/qa/period_test.go
package qa

import (
	"testing"
	"time"
)

func TestResolvePeriod(t *testing.T) {
	now := time.Date(2026, 6, 10, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		in        string
		wantStart string // YYYY-MM-DD
		wantEnd   string
		wantLabel string
		wantOK    bool
	}{
		{"", "2026-06-01", "2026-07-01", "Jun 2026", true},
		{"this_month", "2026-06-01", "2026-07-01", "Jun 2026", true},
		{"this month", "2026-06-01", "2026-07-01", "Jun 2026", true},
		{"last_month", "2026-05-01", "2026-06-01", "May 2026", true},
		{"this_year", "2026-01-01", "2027-01-01", "2026", true},
		{"may", "2026-05-01", "2026-06-01", "May 2026", true},
		{"May 2026", "2026-05-01", "2026-06-01", "May 2026", true},
		{"december", "2025-12-01", "2026-01-01", "Dec 2025", true}, // future month → previous year
		{"jan", "2026-01-01", "2026-02-01", "Jan 2026", true},
		{"gibberish", "2026-06-01", "2026-07-01", "Jun 2026", false}, // default + not-understood
		{"current month", "2026-06-01", "2026-07-01", "Jun 2026", true},
		{"previous month", "2026-05-01", "2026-06-01", "May 2026", true},
	}
	for _, c := range cases {
		p, ok := ResolvePeriod(c.in, now)
		if p.Start.Format("2006-01-02") != c.wantStart ||
			p.End.Format("2006-01-02") != c.wantEnd ||
			p.Label != c.wantLabel || ok != c.wantOK {
			t.Errorf("ResolvePeriod(%q) = {%s %s %q} ok=%v, want {%s %s %q} ok=%v",
				c.in, p.Start.Format("2006-01-02"), p.End.Format("2006-01-02"), p.Label, ok,
				c.wantStart, c.wantEnd, c.wantLabel, c.wantOK)
		}
	}
}

func TestResolvePeriodLastMonthOnMonthEnd(t *testing.T) {
	// AddDate(0,-1,0) from Mar 31 would normalize to Mar 3 — regression guard.
	cases := []struct {
		now       time.Time
		wantLabel string
	}{
		{time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC), "Feb 2026"},
		{time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), "Apr 2026"},
		{time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), "Nov 2026"},
		{time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), "Dec 2025"}, // year boundary
	}
	for _, c := range cases {
		p, ok := ResolvePeriod("last_month", c.now)
		if !ok || p.Label != c.wantLabel {
			t.Errorf("ResolvePeriod(last_month, %s) = %q ok=%v, want %q", c.now.Format("2006-01-02"), p.Label, ok, c.wantLabel)
		}
	}
}

func TestPeriodIsSingleMonth(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	m, _ := ResolvePeriod("this_month", now)
	if !m.IsSingleMonth() {
		t.Error("this_month should be a single month")
	}
	y, _ := ResolvePeriod("this_year", now)
	if y.IsSingleMonth() {
		t.Error("this_year should not be a single month")
	}
}

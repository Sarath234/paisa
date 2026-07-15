package monitor

import (
	"testing"
	"time"
)

func at(hhmm string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", "2026-07-15 "+hhmm)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDailyAt(t *testing.T) {
	due := DailyAt(8)
	cases := []struct {
		name         string
		now, lastRun time.Time
		want         bool
	}{
		{"before hour", at("07:45"), time.Time{}, false},
		{"at hour, never run", at("08:00"), time.Time{}, true},
		{"after hour, ran yesterday", at("08:15"), at("08:15").Add(-24 * time.Hour), true},
		{"after hour, already ran today", at("09:00"), at("08:15"), false},
		{"next day", at("08:01").Add(24 * time.Hour), at("08:15"), true},
	}
	for _, c := range cases {
		if got := due(c.now, c.lastRun); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestEveryInterval(t *testing.T) {
	due := EveryInterval(4 * time.Hour)
	if !due(at("12:00"), at("07:59")) {
		t.Error("4h+ elapsed: want true")
	}
	if due(at("12:00"), at("09:00")) {
		t.Error("3h elapsed: want false")
	}
}

func TestHelpers(t *testing.T) {
	if got := INR(23450.5); got != "₹23450.50" {
		t.Errorf("INR: %q", got)
	}
	if got := Short("Liabilities:CreditCard:Axis"); got != "Axis" {
		t.Errorf("Short: %q", got)
	}
	if got := Short("Axis"); got != "Axis" {
		t.Errorf("Short no colon: %q", got)
	}
	d := DateOnly(at("13:37"))
	if d.Hour() != 0 || d.Day() != 15 {
		t.Errorf("DateOnly: %v", d)
	}
}

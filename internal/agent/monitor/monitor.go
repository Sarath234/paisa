// Package monitor is a scheduled-insight framework for paisa-agent:
// monitors check financial state and emit deduplicated insights that are
// delivered via Telegram, immediately or in a daily digest.
package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Urgency int

const (
	// Immediate insights are sent the moment they are detected.
	Immediate Urgency = iota
	// Digest insights queue up and flush once daily at the digest hour.
	Digest
)

// Insight is one observation. Key must be stable: an insight Key is sent at
// most once, ever. Escalations must use distinct keys.
type Insight struct {
	Key     string
	Urgency Urgency
	Title   string
	Body    string
}

type Monitor interface {
	Name() string
	Due(now, lastRun time.Time) bool
	Check(ctx context.Context) ([]Insight, error)
}

// DailyAt returns a cadence that fires on the first check at/after hour
// (local time) each day.
func DailyAt(hour int) func(now, lastRun time.Time) bool {
	return func(now, lastRun time.Time) bool {
		threshold := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
		return !now.Before(threshold) && lastRun.Before(threshold)
	}
}

// EveryInterval returns a cadence that fires when at least d has elapsed
// since the last run.
func EveryInterval(d time.Duration) func(now, lastRun time.Time) bool {
	return func(now, lastRun time.Time) bool {
		return now.Sub(lastRun) >= d
	}
}

func INR(v float64) string {
	return fmt.Sprintf("₹%.2f", v)
}

// Short returns the last segment of a ledger account name.
func Short(account string) string {
	if idx := strings.LastIndex(account, ":"); idx >= 0 {
		return account[idx+1:]
	}
	return account
}

// DateOnly truncates t to midnight in its own location.
func DateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

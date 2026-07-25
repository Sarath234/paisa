// Package truthcompare provides shared primitives for comparing a
// computed value against a bank-truth value (from SMS/PDF statements)
// and labeling the result. It has no dependencies on internal/agent or
// internal/server, so it can be imported by both the agent binary and
// the core web server without coupling the two together.
package truthcompare

import "time"

// Authority ranks fact sources. Persisted as ints (in bill-truth.json)
// — never reorder.
type Authority int

const (
	AuthorityAPI Authority = iota // computed from paisa.yaml cycle days
	AuthoritySMS                  // bank statement/payment SMS
	AuthorityPDF                  // parsed statement PDF
)

// Status is the display label for a computed-vs-truth comparison.
type Status string

const (
	StatusComputed  Status = "computed"
	StatusConfirmed Status = "confirmed"
	StatusCorrected Status = "corrected"
)

// WithinWindow reports whether a and b are within window of each
// other, inclusive at the boundary.
func WithinWindow(a, b time.Time, window time.Duration) bool {
	diff := a.Sub(b)
	if diff < 0 {
		diff = -diff
	}
	return diff <= window
}

// SameDay reports whether a and b fall on the same calendar day.
func SameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// FieldStatus gates on authority, then labels by agreement: authority
// below AuthoritySMS means the field has never been set by anything
// but a computed fact, so it's always StatusComputed regardless of
// agrees. Otherwise StatusConfirmed if agrees, else StatusCorrected.
func FieldStatus(authority Authority, agrees bool) Status {
	if authority < AuthoritySMS {
		return StatusComputed
	}
	if agrees {
		return StatusConfirmed
	}
	return StatusCorrected
}

// ChannelLabel names the authority for display.
func ChannelLabel(authority Authority) string {
	if authority >= AuthorityPDF {
		return "pdf"
	}
	return "sms"
}

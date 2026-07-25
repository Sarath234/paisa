// Package billtruth stores per-card bill facts merged from bank statements
// (PDF), bank SMSes, and paisa's computed bills, with per-field authority:
// pdf > sms > api. Monitors read bills through this store so reminders use
// the bank's actual dates/amounts, not computed approximations.
package billtruth

import (
	"time"

	"github.com/ananthakumaran/paisa/internal/truthcompare"
)

// Authority ranks fact sources. Persisted as ints — never reorder.
// Aliased from truthcompare so bill-truth.json's encoding and every
// existing billtruth call site (AuthoritySMS, Bill.Sources, etc.) are
// unchanged; truthcompare is the canonical definition, shared with
// internal/server (see internal/truthcompare's package doc).
type Authority = truthcompare.Authority

const (
	AuthorityAPI = truthcompare.AuthorityAPI // computed from paisa.yaml cycle days
	AuthoritySMS = truthcompare.AuthoritySMS // bank statement/payment SMS
	AuthorityPDF = truthcompare.AuthorityPDF // parsed statement PDF
)

// Bill is one statement cycle's truth-merged facts. Sources records, per
// field name ("period_start", "period_end", "due_date", "total_due",
// "min_due", "paid_date", "paid_amount", "interest_total"), the authority
// that set it.
type Bill struct {
	Account       string               `json:"account"`
	PeriodStart   time.Time            `json:"periodStart"`
	PeriodEnd     time.Time            `json:"periodEnd"`
	DueDate       time.Time            `json:"dueDate"`
	TotalDue      float64              `json:"totalDue"`
	MinDue        float64              `json:"minDue"`
	PaidDate      *time.Time           `json:"paidDate"`
	PaidAmount    float64              `json:"paidAmount"`
	InterestTotal float64              `json:"interestTotal"`
	Sources       map[string]Authority `json:"sources"`
	UpdatedAt     time.Time            `json:"updatedAt"`
}

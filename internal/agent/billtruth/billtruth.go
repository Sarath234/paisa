// Package billtruth stores per-card bill facts merged from bank statements
// (PDF), bank SMSes, and paisa's computed bills, with per-field authority:
// pdf > sms > api. Monitors read bills through this store so reminders use
// the bank's actual dates/amounts, not computed approximations.
package billtruth

import "time"

// Authority ranks fact sources. Persisted as ints — never reorder.
type Authority int

const (
	AuthorityAPI Authority = iota // computed from paisa.yaml cycle days
	AuthoritySMS                  // bank statement/payment SMS
	AuthorityPDF                  // parsed statement PDF
)

// Bill is one statement cycle's truth-merged facts. Sources records, per
// field name ("period_start", "period_end", "due_date", "total_due",
// "min_due", "paid_date", "paid_amount"), the authority that set it.
type Bill struct {
	Account     string               `json:"account"`
	PeriodStart time.Time            `json:"periodStart"`
	PeriodEnd   time.Time            `json:"periodEnd"`
	DueDate     time.Time            `json:"dueDate"`
	TotalDue    float64              `json:"totalDue"`
	MinDue      float64              `json:"minDue"`
	PaidDate    *time.Time           `json:"paidDate"`
	PaidAmount  float64              `json:"paidAmount"`
	Sources     map[string]Authority `json:"sources"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}

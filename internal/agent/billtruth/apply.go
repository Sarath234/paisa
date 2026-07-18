package billtruth

import (
	"math"
	"time"

	log "github.com/sirupsen/logrus"
)

// Facts is one source's observation about a bill. Nil = "no claim".
// PeriodEnd nil means payment-only facts (attach to newest unpaid bill).
type Facts struct {
	Account     string
	PeriodStart *time.Time
	PeriodEnd   *time.Time
	DueDate     *time.Time
	TotalDue    *float64
	MinDue      *float64
	PaidDate    *time.Time
	PaidAmount  *float64
	Source      Authority
}

const samePeriodDays = 7

// Apply merges facts into the matching bill (creating one if needed) under
// the authority rules, saves, and reports which fields changed value.
func (s *Store) Apply(f Facts) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	bill := s.findBillLocked(f)
	if bill == nil {
		if f.PeriodEnd == nil {
			log.Warnf("billtruth: payment facts for %s with no unpaid bill — ignored", f.Account)
			return nil, nil
		}
		s.bills[f.Account] = append(s.bills[f.Account], Bill{
			Account: f.Account,
			Sources: map[string]Authority{},
		})
		bill = &s.bills[f.Account][len(s.bills[f.Account])-1]
	}
	if bill.Sources == nil {
		bill.Sources = map[string]Authority{}
	}

	var changed []string
	setT := func(field string, cur *time.Time, val *time.Time) {
		if val == nil {
			return
		}
		if have, ok := bill.Sources[field]; ok && f.Source < have {
			return
		}
		if !cur.Equal(*val) {
			*cur = *val
			changed = append(changed, field)
		}
		bill.Sources[field] = f.Source
	}
	setF := func(field string, cur *float64, val *float64) {
		if val == nil {
			return
		}
		if have, ok := bill.Sources[field]; ok && f.Source < have {
			return
		}
		if math.Abs(*cur-*val) > 0.001 {
			*cur = *val
			changed = append(changed, field)
		}
		bill.Sources[field] = f.Source
	}

	setT("period_start", &bill.PeriodStart, f.PeriodStart)
	setT("period_end", &bill.PeriodEnd, f.PeriodEnd)
	setT("due_date", &bill.DueDate, f.DueDate)
	setF("total_due", &bill.TotalDue, f.TotalDue)
	setF("min_due", &bill.MinDue, f.MinDue)
	if f.PaidDate != nil {
		if have, ok := bill.Sources["paid_date"]; !ok || f.Source >= have {
			if bill.PaidDate == nil || !bill.PaidDate.Equal(*f.PaidDate) {
				d := *f.PaidDate
				bill.PaidDate = &d
				changed = append(changed, "paid_date")
			}
			bill.Sources["paid_date"] = f.Source
		}
	}
	setF("paid_amount", &bill.PaidAmount, f.PaidAmount)

	if len(changed) > 0 {
		bill.UpdatedAt = s.Now()
		if err := s.saveLocked(); err != nil {
			return changed, err
		}
	}
	return changed, nil
}

// findBillLocked resolves bill identity: PeriodEnd within ±samePeriodDays,
// or (for payment facts) the newest unpaid bill.
func (s *Store) findBillLocked(f Facts) *Bill {
	bills := s.bills[f.Account]
	if f.PeriodEnd == nil {
		var newest *Bill
		for i := range bills {
			b := &bills[i]
			if b.PaidDate != nil {
				continue
			}
			if newest == nil || b.PeriodEnd.After(newest.PeriodEnd) {
				newest = b
			}
		}
		return newest
	}
	for i := range bills {
		b := &bills[i]
		diff := b.PeriodEnd.Sub(*f.PeriodEnd)
		if diff < 0 {
			diff = -diff
		}
		if diff <= samePeriodDays*24*time.Hour {
			return b
		}
	}
	return nil
}

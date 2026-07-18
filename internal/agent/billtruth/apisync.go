package billtruth

import (
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

type CreditCardLister interface {
	CreditCards() ([]paisaclient.CreditCardSummary, error)
}

// SyncFromAPI copies computed bills into the store at AuthorityAPI. The
// merge rules make this fill-holes-only: api never overwrites sms/pdf.
// Open cycles (future StatementEndDate) are skipped.
func SyncFromAPI(s *Store, client CreditCardLister) error {
	cards, err := client.CreditCards()
	if err != nil {
		return err
	}
	today := s.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	for _, card := range cards {
		for _, b := range card.Bills {
			if !today.After(b.StatementEndDate) {
				continue // open cycle
			}
			start, end, due := b.StatementStartDate, b.StatementEndDate, b.DueDate
			total := b.ClosingBalance
			f := Facts{
				Account:     card.Account,
				PeriodStart: &start,
				PeriodEnd:   &end,
				DueDate:     &due,
				TotalDue:    &total,
				Source:      AuthorityAPI,
			}
			if b.PaidDate != nil {
				f.PaidDate = b.PaidDate
			}
			if _, err := s.Apply(f); err != nil {
				return err
			}
		}
	}
	return nil
}

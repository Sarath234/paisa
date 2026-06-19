// internal/agent/reconcile/reconcile.go
package reconcile

import (
	"math"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/statement"
)

// LedgerEntry is a posting from the paisa ledger relevant to reconciliation.
type LedgerEntry struct {
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	Amount      float64   `json:"amount"` // negative = debit, positive = credit
}

// Diff is the result of comparing a statement against the ledger.
type Diff struct {
	Account        string                  `json:"account"`
	Month          int                     `json:"month"` // 1-12
	Year           int                     `json:"year"`
	StatementClose float64                 `json:"statement_close"`
	Missing        []statement.Transaction `json:"missing"` // in statement, not in ledger
	Extra          []LedgerEntry           `json:"extra"`   // in ledger, not in statement
}

const amountEpsilon = 0.01

// Compare matches statement transactions against ledger entries by date + |amount|.
// A statement debit of 500 matches a ledger entry with Amount -500 on the same date.
func Compare(result statement.ParseResult, ledger []LedgerEntry) Diff {
	diff := Diff{
		Month:          int(result.Month),
		Year:           result.Year,
		StatementClose: result.ClosingBalance,
	}

	ledgerUsed := make([]bool, len(ledger))

	for _, tx := range result.Transactions {
		txAmt := tx.Credit - tx.Debit // net: credit positive, debit negative
		matched := false
		for i, le := range ledger {
			if ledgerUsed[i] {
				continue
			}
			if !sameDay(tx.Date, le.Date) {
				continue
			}
			if math.Abs(le.Amount-txAmt) <= amountEpsilon {
				ledgerUsed[i] = true
				matched = true
				break
			}
		}
		if !matched {
			diff.Missing = append(diff.Missing, tx)
		}
	}

	for i, le := range ledger {
		if !ledgerUsed[i] {
			diff.Extra = append(diff.Extra, le)
		}
	}

	return diff
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

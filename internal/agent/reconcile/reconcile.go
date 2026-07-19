// internal/agent/reconcile/reconcile.go
package reconcile

import (
	"math"
	"sort"
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
	// Matched is how many statement transactions found a ledger partner —
	// persisted so the upload-status endpoint can report a summary without
	// recomputing. Additive: records written before this field read as 0.
	Matched int `json:"matched"`
}

const amountEpsilon = 0.01
const ccDateWindowDays = 3

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

	diff.Matched = len(result.Transactions) - len(diff.Missing)
	return diff
}

// CompareCC matches CC statement transactions against ledger entries:
// amount exact (±0.01) + date within ±3 days. Statement transactions are
// processed in date order and each takes the EARLIEST-dated unused ledger
// candidate in its window. Because window compatibility is contiguous in
// date (a convex bipartite graph), sorted-by-date + earliest-available is
// the standard optimal greedy for this matching: it finds a full pairing
// whenever one exists. Closest-date greedy is order-sensitive and can
// strand a pairable pair, yielding a false Missing + Extra.
func CompareCC(res statement.CCResult, ledger []LedgerEntry) Diff {
	diff := Diff{
		Month:          int(res.PeriodEnd.Month()),
		Year:           res.PeriodEnd.Year(),
		StatementClose: res.TotalDue,
	}

	// Process statement transactions in date order without mutating the input.
	order := make([]int, len(res.Transactions))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return res.Transactions[order[a]].Date.Before(res.Transactions[order[b]].Date)
	})

	used := make([]bool, len(ledger))
	window := ccDateWindowDays * 24 * time.Hour
	for _, ti := range order {
		tx := res.Transactions[ti]
		txAmt := tx.Credit - tx.Debit
		best := -1
		for i, le := range ledger {
			if used[i] {
				continue
			}
			if math.Abs(le.Amount-txAmt) > amountEpsilon {
				continue
			}
			gap := le.Date.Sub(tx.Date)
			if gap < 0 {
				gap = -gap
			}
			if gap > window {
				continue
			}
			// Earliest-dated candidate wins; equal dates: lowest index.
			if best < 0 || le.Date.Before(ledger[best].Date) {
				best = i
			}
		}
		if best >= 0 {
			used[best] = true
		} else {
			diff.Missing = append(diff.Missing, tx.Transaction)
		}
	}
	for i, le := range ledger {
		if !used[i] {
			diff.Extra = append(diff.Extra, le)
		}
	}
	diff.Matched = len(res.Transactions) - len(diff.Missing)
	return diff
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

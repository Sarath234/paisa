package journal

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/ananthakumaran/paisa/internal/agent/parser"
)

// Format produces an hledger journal entry for a parsed transaction.
// debitAccount is the bank account used as the balancing entry (e.g. "Assets:HDFC:Savings").
func Format(tx parser.ParsedTransaction, source, debitAccount string) string {
	absAmount := math.Abs(tx.Amount)
	// Convert ISO date YYYY-MM-DD to hledger YYYY/MM/DD
	date := strings.ReplaceAll(tx.Date, "-", "/")

	var entry strings.Builder
	entry.WriteString(fmt.Sprintf("; source: %s\n", source))
	if tx.RefID != "" {
		entry.WriteString(fmt.Sprintf("; ref: %s\n", tx.RefID))
	}
	entry.WriteString(fmt.Sprintf("%s %s\n", date, tx.Merchant))

	if tx.Amount < 0 {
		// Debit: money leaves bank account → expense/liability increases
		entry.WriteString(fmt.Sprintf("    %s    %s %.2f\n", tx.SuggestedLedgerAccount, tx.Currency, absAmount))
		entry.WriteString(fmt.Sprintf("    %s\n", debitAccount))
	} else {
		// Credit: money enters bank account ← income source
		entry.WriteString(fmt.Sprintf("    %s    %s %.2f\n", debitAccount, tx.Currency, absAmount))
		entry.WriteString(fmt.Sprintf("    %s\n", tx.SuggestedLedgerAccount))
	}
	entry.WriteString("\n")
	return entry.String()
}

// Append writes entry to <journalDir>/auto-import.ledger (creates if absent).
func Append(journalDir, entry string) error {
	path := filepath.Join(journalDir, "auto-import.ledger")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(entry)
	return err
}

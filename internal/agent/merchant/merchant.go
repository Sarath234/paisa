package merchant

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Decision int

const (
	AutoPost      Decision = iota
	NeedsApproval
)

func Gate(db *gorm.DB, merchantName string, amount, confidence, threshold float64, promoteAfter int) Decision {
	if confidence < 0.7 || amount >= threshold {
		return NeedsApproval
	}
	var rule agentdb.MerchantRule
	if err := db.First(&rule, "merchant = ?", merchantName).Error; err != nil {
		return NeedsApproval
	}
	if rule.AutoApprove {
		return AutoPost
	}
	return NeedsApproval
}

func RecordApproval(db *gorm.DB, merchantName, account string, promoteAfter int) error {
	var rule agentdb.MerchantRule
	db.Where(agentdb.MerchantRule{Merchant: merchantName}).FirstOrCreate(&rule)
	rule.Account = account
	rule.ApproveCount++
	if rule.ApproveCount >= promoteAfter {
		rule.AutoApprove = true
	}
	return db.Save(&rule).Error
}

var txHeaderRe = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2})\s+(.+)$`)
var postingRe = regexp.MustCompile(`^\s{4}(\S[^;]+?)\s{2,}`)

// Bootstrap scans an hledger journal file and pre-populates merchant rules
// for all payees that have expense/liability postings.
func Bootstrap(db *gorm.DB, journalPath string, promoteAfter int) error {
	f, err := os.Open(journalPath)
	if err != nil {
		return err
	}
	defer f.Close()

	type pair struct{ payee, account string }
	var pairs []pair
	var currentPayee string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := txHeaderRe.FindStringSubmatch(line); m != nil {
			currentPayee = strings.TrimSpace(m[2])
			continue
		}
		if currentPayee == "" {
			continue
		}
		if strings.TrimSpace(line) == "" {
			currentPayee = ""
			continue
		}
		if m := postingRe.FindStringSubmatch(line); m != nil {
			account := strings.Fields(strings.TrimSpace(m[1]))[0]
			if strings.HasPrefix(account, "Expenses:") || strings.HasPrefix(account, "Liabilities:") {
				pairs = append(pairs, pair{currentPayee, account})
			}
		}
	}

	for _, p := range pairs {
		rule := agentdb.MerchantRule{
			Merchant:     p.payee,
			Account:      p.account,
			ApproveCount: promoteAfter,
			AutoApprove:  true,
		}
		db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule)
	}
	return scanner.Err()
}

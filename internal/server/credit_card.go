package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ananthakumaran/paisa/internal/accounting"
	"github.com/ananthakumaran/paisa/internal/config"
	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/model/transaction"
	"github.com/ananthakumaran/paisa/internal/query"
	"github.com/ananthakumaran/paisa/internal/service"
	"github.com/ananthakumaran/paisa/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type CreditCardSummary struct {
	Account        string                                `json:"account"`
	Network        string                                `json:"network"`
	Number         string                                `json:"number"`
	Balance        decimal.Decimal                       `json:"balance"`
	Bills          []CreditCardBill                      `json:"bills"`
	CreditLimit    decimal.Decimal                       `json:"creditLimit"`
	YearlySpends   map[string]map[string]decimal.Decimal `json:"yearlySpends"`
	ExpirationDate time.Time                             `json:"expirationDate"`
}

type CreditCardBill struct {
	StatementStartDate   time.Time       `json:"statementStartDate"`
	StatementEndDate     time.Time       `json:"statementEndDate"`
	DueDate              time.Time       `json:"dueDate"`
	PaidDate             *time.Time      `json:"paidDate"`
	Credits              decimal.Decimal `json:"credits"`
	Debits               decimal.Decimal `json:"debits"`
	DebitsRunningBalance decimal.Decimal
	OpeningBalance       decimal.Decimal           `json:"openingBalance"`
	ClosingBalance       decimal.Decimal           `json:"closingBalance"`
	Postings             []posting.Posting         `json:"postings"`
	Transactions         []transaction.Transaction `json:"transactions"`

	DueDateStatus   string     `json:"dueDateStatus"` // "computed" | "confirmed" | "corrected"
	DueDateChannel  *string    `json:"dueDateChannel,omitempty"`
	ComputedDueDate *time.Time `json:"computedDueDate,omitempty"`
	TruthDueDate    *time.Time `json:"truthDueDate,omitempty"`

	ClosingBalanceStatus   string           `json:"closingBalanceStatus"`
	ClosingBalanceChannel  *string          `json:"closingBalanceChannel,omitempty"`
	ComputedClosingBalance *decimal.Decimal `json:"computedClosingBalance,omitempty"`
	TruthClosingBalance    *decimal.Decimal `json:"truthClosingBalance,omitempty"`

	PaidDateStatus   string     `json:"paidDateStatus"`
	PaidDateChannel  *string    `json:"paidDateChannel,omitempty"`
	ComputedPaidDate *time.Time `json:"computedPaidDate,omitempty"`
	TruthPaidDate    *time.Time `json:"truthPaidDate,omitempty"`
	UserPaidDate     *time.Time `json:"userPaidDate,omitempty"`
}

// truthBill is a local decode struct for one entry in bill-truth.json,
// written by paisa-agent's internal/agent/billtruth package. Deliberately
// NOT importing that package — core paisa stays decoupled from agent
// internals, matching doctor.go's reconciliation.json pattern exactly.
type truthBill struct {
	PeriodEnd    time.Time      `json:"periodEnd"`
	DueDate      time.Time      `json:"dueDate"`
	TotalDue     float64        `json:"totalDue"`
	PaidDate     *time.Time     `json:"paidDate"`
	UserPaidDate *time.Time     `json:"userPaidDate"` // self-reported via Telegram; no Sources entry
	Sources      map[string]int `json:"sources"`      // field name -> authority (0 api, 1 sms, 2 pdf)
}

// loadBillTruth reads bill-truth.json from journalDir and returns the
// account's bills, newest PeriodEnd first. A missing file, an unreadable
// file, or a corrupt file all return nil — never an error — mirroring
// doctor.go's ruleStatementReconciliation handling of reconciliation.json.
func loadBillTruth(journalDir, account string) []truthBill {
	path := filepath.Join(journalDir, "bill-truth.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var all map[string][]truthBill
	if err := json.Unmarshal(data, &all); err != nil {
		return nil
	}
	bills := all[account]
	sort.Slice(bills, func(i, j int) bool { return bills[i].PeriodEnd.After(bills[j].PeriodEnd) })
	return bills
}

// matchTruthBill finds the truth bill whose PeriodEnd is within ±7 days of
// statementEndDate — the same identity fuzz billtruth uses internally
// (apply.go's findBillLocked) — or nil. bills must already be sorted
// newest-PeriodEnd-first (loadBillTruth does this), so the first match is
// also the newest.
func matchTruthBill(bills []truthBill, statementEndDate time.Time) *truthBill {
	const window = 7 * 24 * time.Hour
	for i := range bills {
		diff := bills[i].PeriodEnd.Sub(statementEndDate)
		if diff < 0 {
			diff = -diff
		}
		if diff <= window {
			return &bills[i]
		}
	}
	return nil
}

// authoritySMS/authorityPDF duplicate billtruth.AuthoritySMS/AuthorityPDF's
// values deliberately — core paisa does not import internal/agent/billtruth
// (see loadBillTruth). Never reorder; persisted in bill-truth.json as ints.
const (
	authoritySMS = 1
	authorityPDF = 2
)

// applyTruth overlays bank-message-derived facts onto a computed
// CreditCardBill, per field, only when bill-truth.json's authority for
// that field is at least SMS — never for merely api-sourced truth, which
// is just this same computed value re-derived by the agent from this same
// server. truth may be nil (no bill-truth.json, or no bill within ±7 days
// of this cycle's close) — every field then stays "computed", identical
// to today's behavior.
func applyTruth(bill *CreditCardBill, truth *truthBill) {
	if truth == nil {
		bill.DueDateStatus = "computed"
		bill.ClosingBalanceStatus = "computed"
		bill.PaidDateStatus = "computed"
		return
	}

	dueAuthority := truth.Sources["due_date"]
	bill.DueDateStatus = fieldStatus(dueAuthority, bill.DueDate, truth.DueDate)
	if bill.DueDateStatus != "computed" {
		channel := channelLabel(dueAuthority)
		bill.DueDateChannel = &channel
		if bill.DueDateStatus == "corrected" {
			computed, truthVal := bill.DueDate, truth.DueDate
			bill.ComputedDueDate, bill.TruthDueDate = &computed, &truthVal
		}
		bill.DueDate = truth.DueDate
	}

	totalAuthority := truth.Sources["total_due"]
	bill.ClosingBalanceStatus = amountFieldStatus(totalAuthority, bill.ClosingBalance, truth.TotalDue)
	if bill.ClosingBalanceStatus != "computed" {
		channel := channelLabel(totalAuthority)
		bill.ClosingBalanceChannel = &channel
		truthAmt := decimal.NewFromFloat(truth.TotalDue)
		if bill.ClosingBalanceStatus == "corrected" {
			computed := bill.ClosingBalance
			bill.ComputedClosingBalance, bill.TruthClosingBalance = &computed, &truthAmt
		}
		bill.ClosingBalance = truthAmt
	}

	if truth.UserPaidDate != nil {
		bill.UserPaidDate = truth.UserPaidDate
	}

	paidAuthority := truth.Sources["paid_date"]
	bill.PaidDateStatus = paidDateStatus(paidAuthority, bill.PaidDate, truth.PaidDate, truth.UserPaidDate)
	switch bill.PaidDateStatus {
	case "computed", "self_reported":
		// self_reported sets only the status badge — there's no bank
		// channel/authority to attribute (truth.Sources has no "paid_date"
		// entry for a self-report), and truth.PaidDate is nil in this case,
		// so leave bill.PaidDate/PaidDateChannel exactly as computeBills
		// already set them rather than nulling PaidDate or mislabeling the
		// channel as "sms".
	default: // "confirmed" or "corrected"
		channel := channelLabel(paidAuthority)
		bill.PaidDateChannel = &channel
		if bill.PaidDateStatus == "corrected" {
			bill.ComputedPaidDate = bill.PaidDate // may be nil — that IS the mismatch
			bill.TruthPaidDate = truth.PaidDate
		}
		bill.PaidDate = truth.PaidDate
	}
}

// fieldStatus: authority below AuthoritySMS (including the zero-value for
// a field bill-truth.json never set) → "computed". Otherwise "corrected"
// if computed/truth disagree on calendar day, else "confirmed".
func fieldStatus(authority int, computed, truthVal time.Time) string {
	if authority < authoritySMS {
		return "computed"
	}
	if !sameDay(computed, truthVal) {
		return "corrected"
	}
	return "confirmed"
}

// amountFieldStatus: same authority gate; "confirmed" tolerates ≤ ₹1
// difference (matches the cc_statement Telegram monitor's own correction
// threshold), else "corrected".
func amountFieldStatus(authority int, computed decimal.Decimal, truthAmount float64) string {
	if authority < authoritySMS {
		return "computed"
	}
	diff := computed.Sub(decimal.NewFromFloat(truthAmount)).Abs()
	if diff.GreaterThan(decimal.NewFromInt(1)) {
		return "corrected"
	}
	return "confirmed"
}

// paidDateStatus: a self-reported (Telegram) paid mark with no bank
// confirmation yet is "self_reported" — checked before the authority gate,
// since self-reporting has no SMS/PDF authority at all. A real bank
// confirmation (truthPaidDate set) always takes priority once it arrives:
// presence/absence mismatch (nil vs set) is itself "corrected" — that's the
// whole point of forwarding a payment SMS/PDF.
func paidDateStatus(authority int, computedPaidDate, truthPaidDate, userPaidDate *time.Time) string {
	if truthPaidDate == nil && userPaidDate != nil {
		return "self_reported"
	}
	if authority < authoritySMS {
		return "computed"
	}
	switch {
	case truthPaidDate == nil && computedPaidDate == nil:
		return "confirmed"
	case truthPaidDate == nil || computedPaidDate == nil:
		return "corrected"
	case sameDay(*truthPaidDate, *computedPaidDate):
		return "confirmed"
	default:
		return "corrected"
	}
}

func channelLabel(authority int) string {
	if authority >= authorityPDF {
		return "pdf"
	}
	return "sms"
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func GetCreditCards(db *gorm.DB) gin.H {
	creditCards := []CreditCardSummary{}

	for _, creditCardConfig := range config.GetConfig().CreditCards {
		ps := query.Init(db).Where("account = ?", creditCardConfig.Account).All()
		creditCards = append(creditCards, buildCreditCard(db, creditCardConfig, ps, false))
	}

	return gin.H{"creditCards": creditCards}
}

func GetCreditCard(db *gorm.DB, account string) gin.H {
	for _, creditCardConfig := range config.GetConfig().CreditCards {
		if creditCardConfig.Account == account {
			ps := query.Init(db).Where("account = ?", creditCardConfig.Account).All()
			creditCard := buildCreditCard(db, creditCardConfig, ps, true)
			return gin.H{"creditCard": creditCard, "found": true}
		}
	}

	return gin.H{"found": false}
}

func yearlySpends(db *gorm.DB, date time.Time, postings []posting.Posting) map[string]map[string]decimal.Decimal {
	yearlySpends := make(map[string]map[string]decimal.Decimal)
	for year, ps := range utils.GroupByYearCutoffAt(postings, date) {
		spends := lo.Filter(ps, func(p posting.Posting, _ int) bool {
			return p.Amount.IsNegative() || service.IsContraPostingRefund(db, p)
		})

		yearlySpends[year] = make(map[string]decimal.Decimal)
		for month, ps := range utils.GroupByMonth(spends) {
			yearlySpends[year][month] = accounting.CostSum(ps).Neg()
		}
	}
	return yearlySpends
}

func buildCreditCard(db *gorm.DB, creditCardConfig config.CreditCard, ps []posting.Posting, includePostings bool) CreditCardSummary {
	bills := computeBills(db, creditCardConfig, ps, includePostings)

	journalDir := filepath.Dir(config.GetJournalPath())
	truthBills := loadBillTruth(journalDir, creditCardConfig.Account)
	for i := range bills {
		applyTruth(&bills[i], matchTruthBill(truthBills, bills[i].StatementEndDate))
	}

	balance := decimal.Zero
	if len(bills) > 0 {
		balance = bills[len(bills)-1].ClosingBalance
	}

	expirationDate, err := time.ParseInLocation("2006-01-02", creditCardConfig.ExpirationDate, config.TimeZone())
	if err != nil {
		log.Fatal(err)
	}

	ys := make(map[string]map[string]decimal.Decimal)
	if includePostings {
		ys = yearlySpends(db, expirationDate, ps)
	}
	return CreditCardSummary{
		Account:        creditCardConfig.Account,
		Network:        creditCardConfig.Network,
		Number:         creditCardConfig.Number,
		Balance:        balance,
		Bills:          bills,
		CreditLimit:    decimal.NewFromInt(int64(creditCardConfig.CreditLimit)),
		YearlySpends:   ys,
		ExpirationDate: expirationDate,
	}
}

func computeBills(db *gorm.DB, creditCardConfig config.CreditCard, ps []posting.Posting, includePostings bool) []CreditCardBill {
	bills := []CreditCardBill{}

	grouped := accounting.GroupByMonthlyBillingCycle(ps, creditCardConfig.StatementEndDay)

	balance := decimal.Zero
	creditsRunningBalance := decimal.Zero
	debitsRunningBalance := decimal.Zero
	unpaidBill := 0

	for _, month := range utils.SortedKeys(grouped) {
		statementEndDate, err := time.ParseInLocation("2006-01", month, config.TimeZone())
		if err != nil {
			log.Fatal(err)
		}

		statementEndDate = statementEndDate.AddDate(0, 0, creditCardConfig.StatementEndDay-1)
		statementStartDate := statementEndDate.AddDate(0, -1, 1)

		var dueDate time.Time
		if creditCardConfig.StatementEndDay < creditCardConfig.DueDay {
			dueDate = utils.BeginningOfMonth(statementEndDate).AddDate(0, 0, creditCardConfig.DueDay-1)
		} else {
			dueDate = utils.BeginningOfMonth(statementEndDate).AddDate(0, 1, creditCardConfig.DueDay-1)
		}

		bill := CreditCardBill{
			StatementStartDate: statementStartDate,
			StatementEndDate:   statementEndDate,
			DueDate:            dueDate,
			OpeningBalance:     balance,
			Postings:           []posting.Posting{},
			Transactions:       []transaction.Transaction{},
		}

		transactionIDs := map[string]bool{}

		for _, p := range grouped[month] {
			balance = balance.Add(p.Amount.Neg())

			if p.Amount.IsPositive() {
				creditsRunningBalance = creditsRunningBalance.Add(p.Amount)
				bill.Credits = bill.Credits.Add(p.Amount)
				for unpaidBill < len(bills) {
					if bills[unpaidBill].DebitsRunningBalance.LessThanOrEqual(creditsRunningBalance) {
						paidDate := p.Date
						bills[unpaidBill].PaidDate = &paidDate
						unpaidBill++
					} else {
						break
					}
				}
			} else {
				bill.Debits = bill.Debits.Add(p.Amount.Neg())
				debitsRunningBalance = debitsRunningBalance.Add(p.Amount.Neg())
			}

			if includePostings {
				bill.Postings = append(bill.Postings, p)
				transactionIDs[p.TransactionID] = true
			}

		}

		bill.DebitsRunningBalance = debitsRunningBalance
		bill.ClosingBalance = balance
		bill.Transactions = lo.Map(lo.Keys(transactionIDs), func(id string, _ int) transaction.Transaction {
			t, _ := transaction.GetById(db, id)
			return t
		})
		accounting.SortTransactionAsc(bill.Transactions)
		bills = append(bills, bill)
	}

	return bills
}

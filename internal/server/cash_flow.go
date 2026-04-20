package server

import (
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/accounting"
	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/query"
	"github.com/ananthakumaran/paisa/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type CashFlow struct {
	Date        time.Time       `json:"date"`
	Income      decimal.Decimal `json:"income"`
	Expenses    decimal.Decimal `json:"expenses"`
	Liabilities decimal.Decimal `json:"liabilities"`
	Investment  decimal.Decimal `json:"investment"`
	Tax         decimal.Decimal `json:"tax"`
	Checking    decimal.Decimal `json:"checking"`
	Balance     decimal.Decimal `json:"balance"`
}

func (c CashFlow) GroupDate() time.Time {
	return c.Date
}

func GetCashFlow(db *gorm.DB) gin.H {
	all := query.Init(db).All()
	return gin.H{"cash_flows": computeCashFlowFromPostings(all, decimal.Zero)}
}

func GetCurrentCashFlow(allPostings []posting.Posting) []CashFlow {
	monthStart := utils.BeginningOfMonth(utils.Now())
	windowStart := monthStart.AddDate(0, -2, 0)
	windowEnd := monthStart.AddDate(0, 1, 0)

	var balancePostings, windowPostings []posting.Posting
	for _, p := range allPostings {
		if p.Date.Before(windowStart) && accountPrefixMatch(p.Account, "Assets:Checking") {
			balancePostings = append(balancePostings, p)
		}
		if (p.Date.Equal(windowStart) || p.Date.After(windowStart)) && p.Date.Before(windowEnd) {
			windowPostings = append(windowPostings, p)
		}
	}
	balance := accounting.CostSum(balancePostings)
	return computeCashFlowFromPostings(windowPostings, balance)
}

// accountPrefixMatch returns true if account equals prefix or starts with "prefix:".
func accountPrefixMatch(account, prefix string) bool {
	return account == prefix || strings.HasPrefix(account, prefix+":")
}

func computeCashFlowFromPostings(all []posting.Posting, balance decimal.Decimal) []CashFlow {
	var cashFlows []CashFlow

	if len(all) == 0 {
		return []CashFlow{}
	}

	var expenseList, incomeList, liabilityList, investmentList, taxList, checkingList []posting.Posting
	for _, p := range all {
		a := p.Account
		switch {
		case accountPrefixMatch(a, "Expenses:Tax"):
			taxList = append(taxList, p)
		case strings.HasPrefix(a, "Expenses:"):
			expenseList = append(expenseList, p)
		case strings.HasPrefix(a, "Income:"):
			incomeList = append(incomeList, p)
		case strings.HasPrefix(a, "Liabilities:"):
			liabilityList = append(liabilityList, p)
		case accountPrefixMatch(a, "Assets:Checking"):
			checkingList = append(checkingList, p)
		case strings.HasPrefix(a, "Assets:"):
			investmentList = append(investmentList, p)
		}
	}

	expenses := utils.GroupByMonth(expenseList)
	incomes := utils.GroupByMonth(incomeList)
	liabilities := utils.GroupByMonth(liabilityList)
	investments := utils.GroupByMonth(investmentList)
	taxes := utils.GroupByMonth(taxList)
	checkings := utils.GroupByMonth(checkingList)

	end := utils.MaxTime(utils.EndOfToday(), all[len(all)-1].Date)
	for start := utils.BeginningOfMonth(all[0].Date); start.Before(end); start = start.AddDate(0, 1, 0) {
		cashFlow := CashFlow{Date: start}

		key := start.Format("2006-01")
		ps, ok := expenses[key]
		if ok {
			cashFlow.Expenses = accounting.CostSum(ps)
		}

		ps, ok = incomes[key]
		if ok {
			cashFlow.Income = accounting.CostSum(ps).Neg()
		}

		ps, ok = liabilities[key]
		if ok {
			cashFlow.Liabilities = accounting.CostSum(ps).Neg()
		}

		ps, ok = investments[key]
		if ok {
			cashFlow.Investment = accounting.CostSum(ps)
		}

		ps, ok = taxes[key]
		if ok {
			cashFlow.Tax = accounting.CostSum(ps)
		}

		ps, ok = checkings[key]
		if ok {
			cashFlow.Checking = accounting.CostSum(ps)
		}

		balance = balance.Add(cashFlow.Checking)
		cashFlow.Balance = balance

		cashFlows = append(cashFlows, cashFlow)
	}

	return cashFlows
}

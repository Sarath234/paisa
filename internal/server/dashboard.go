package server

import (
	"github.com/ananthakumaran/paisa/internal/query"
	"github.com/ananthakumaran/paisa/internal/server/assets"
	"github.com/ananthakumaran/paisa/internal/server/goal"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetDashboard(db *gorm.DB) gin.H {
	// Fetch all postings once; sub-functions filter in-memory.
	allPostings := query.Init(db).All()
	forecastExpenses := query.Init(db).Forecast().Like("Expenses:%").UntilThisMonthEnd().All()

	return gin.H{
		"checkingBalances":     assets.GetCheckingBalance(db, allPostings),
		"networth":             GetCurrentNetworth(db, allPostings),
		"expenses":             GetCurrentExpense(allPostings),
		"cashFlows":            GetCurrentCashFlow(allPostings),
		"transactionSequences": ComputeRecurringTransactions(allPostings),
		"transactions":         GetLatestTransactions(allPostings),
		"budget":               GetCurrentBudget(db, allPostings, forecastExpenses),
		"goalSummaries":        goal.GetGoalSummariesFromPostings(db, allPostings),
	}
}

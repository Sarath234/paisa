package server

import (
	"sync"

	"github.com/ananthakumaran/paisa/internal/accounting"
	"github.com/ananthakumaran/paisa/internal/query"
	"github.com/ananthakumaran/paisa/internal/server/assets"
	"github.com/ananthakumaran/paisa/internal/server/goal"
	"github.com/ananthakumaran/paisa/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetDashboard(db *gorm.DB) gin.H {
	// Warm service caches concurrently before the main computation.
	// SQLite runs in WAL mode so concurrent reads are safe.
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); service.PreloadInterestCache(db) }()
	go func() { defer wg.Done(); service.PreloadPriceCache(db) }()
	go func() { defer wg.Done(); accounting.AllAccounts(db) }()
	wg.Wait()

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

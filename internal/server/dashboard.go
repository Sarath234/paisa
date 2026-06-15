package server

import (
	"sync"

	"github.com/ananthakumaran/paisa/internal/accounting"
	"github.com/ananthakumaran/paisa/internal/model/transaction"
	"github.com/ananthakumaran/paisa/internal/query"
	"github.com/ananthakumaran/paisa/internal/server/assets"
	"github.com/ananthakumaran/paisa/internal/server/goal"
	"github.com/ananthakumaran/paisa/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetDashboard(db *gorm.DB) gin.H {
	// Fire interest and price cache loads in background goroutines.
	// SQLite WAL mode allows concurrent reads so these overlap safely.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); service.PreloadInterestCache(db) }()
	go func() { defer wg.Done(); service.PreloadPriceCache(db) }()

	// Load all postings in the main goroutine concurrently with the goroutines
	// above — four DB reads in flight at once instead of sequential.
	allPostings := query.Init(db).All()
	// Prime transaction cache from the postings we just loaded; zero extra DB
	// round-trip vs. the lazy load that used to fire mid-computation.
	transaction.PreloadCacheFromPostings(allPostings)
	// AllAccounts is a cheap Distinct Pluck; no dedicated goroutine needed.
	accounting.AllAccounts(db)

	wg.Wait()

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

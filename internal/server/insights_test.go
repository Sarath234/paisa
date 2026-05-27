package server

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func d(s string) decimal.Decimal {
	v, _ := decimal.NewFromString(s)
	return v
}

func p(account string, date time.Time, amount string) posting.Posting {
	return posting.Posting{Account: account, Date: date, Amount: d(amount), Commodity: "INR"}
}

// ---- spend_category ----

func TestComputeSpendCategory_UpIsNegative(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	prev := now.AddDate(0, -1, 0)
	postings := []posting.Posting{
		p("Expenses:Food", now, "12400"),
		p("Expenses:Food", prev, "10000"),
	}
	insights := computeSpendCategory(postings, now)
	require := assert.New(t)
	require.Len(insights, 1)
	require.Equal("Food", insights[0].Title)
	require.InDelta(24.0, insights[0].DeltaPct, 0.1)
	require.False(insights[0].Positive)   // up = bad
	require.False(insights[0].Suppress)  // |Δ%| >= 10%
}

func TestComputeSpendCategory_DownIsPositive(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	prev := now.AddDate(0, -1, 0)
	postings := []posting.Posting{
		p("Expenses:Food", now, "8000"),
		p("Expenses:Food", prev, "10000"),
	}
	insights := computeSpendCategory(postings, now)
	require := assert.New(t)
	require.Len(insights, 1)
	require.InDelta(-20.0, insights[0].DeltaPct, 0.1)
	require.True(insights[0].Positive)
	require.False(insights[0].Suppress)
}

func TestComputeSpendCategory_BelowThresholdSuppressed(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	prev := now.AddDate(0, -1, 0)
	postings := []posting.Posting{
		p("Expenses:Food", now, "10500"), // +5% — below 10% threshold
		p("Expenses:Food", prev, "10000"),
	}
	insights := computeSpendCategory(postings, now)
	assert.True(t, insights[0].Suppress)
}

func TestComputeSpendCategory_ExcludesTax(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	postings := []posting.Posting{
		p("Expenses:Tax", now, "50000"),
	}
	insights := computeSpendCategory(postings, now)
	assert.Empty(t, insights)
}

func TestComputeSpendCategory_NoPrev_NotSuppressed(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	postings := []posting.Posting{
		p("Expenses:Food", now, "5000"),
	}
	insights := computeSpendCategory(postings, now)
	assert.Len(t, insights, 1)
	assert.False(t, insights[0].Suppress)
}

// ---- savings_rate ----

func TestComputeSavingsRate_AboveAverageIsPositive(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	// Income postings are negative in Paisa's ledger (credit side).
	// Months 1-12: income -100000, expense 72000 → rate 28%
	// Month 0 (current): income -100000, expense 66000 → rate 34%
	var postings []posting.Posting
	for i := 1; i <= 12; i++ {
		m := now.AddDate(0, -i, 0)
		postings = append(postings, p("Income:Salary", m, "-100000"))
		postings = append(postings, p("Expenses:Rent", m, "72000"))
	}
	postings = append(postings, p("Income:Salary", now, "-100000"))
	postings = append(postings, p("Expenses:Rent", now, "66000"))

	insight := computeSavingsRate(postings, now)
	require := assert.New(t)
	require.InDelta(6.0, insight.DeltaPct, 0.5) // +6 pp above avg
	require.True(insight.Positive)
	require.False(insight.Suppress) // |Δpp| >= 5
}

func TestComputeSavingsRate_BelowThresholdSuppressed(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	var postings []posting.Posting
	for i := 0; i <= 12; i++ {
		m := now.AddDate(0, -i, 0)
		postings = append(postings, p("Income:Salary", m, "-100000"))
		postings = append(postings, p("Expenses:Rent", m, "72000")) // 28% every month → 0 pp delta
	}
	insight := computeSavingsRate(postings, now)
	assert.True(t, insight.Suppress) // |Δpp| < 5
}

// ---- budget ----

func TestComputeBudget_UnderBudgetIsPositive(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	budgeted := []posting.Posting{p("Expenses:Entertainment", now, "10000")}
	actual := []posting.Posting{p("Expenses:Entertainment", now, "6800")} // 32% under
	insights := computeBudgetInsights(budgeted, actual, now)
	require := assert.New(t)
	require.Len(insights, 1)
	require.Equal("Entertainment", insights[0].Title)
	require.True(insights[0].Positive)
	require.False(insights[0].Suppress) // variance > 5%
}

func TestComputeBudget_OverBudgetIsNegative(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	budgeted := []posting.Posting{p("Expenses:Food", now, "10000")}
	actual := []posting.Posting{p("Expenses:Food", now, "12000")}
	insights := computeBudgetInsights(budgeted, actual, now)
	assert.False(t, insights[0].Positive)
}

func TestComputeBudget_WithinThresholdSuppressed(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	budgeted := []posting.Posting{p("Expenses:Food", now, "10000")}
	actual := []posting.Posting{p("Expenses:Food", now, "9600")} // 4% under → suppressed
	insights := computeBudgetInsights(budgeted, actual, now)
	assert.True(t, insights[0].Suppress)
}

// ---- top_category ----

func TestComputeTopCategory_HighestSpend(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	postings := []posting.Posting{
		p("Expenses:Dining", now, "15000"),
		p("Expenses:Food", now, "5000"),
		p("Expenses:Transport", now, "3000"),
	}
	insight := computeTopCategory(postings, now)
	assert.Equal(t, "Dining", insight.Title)
	assert.True(t, insight.Positive)
	assert.False(t, insight.Suppress)
	assert.Equal(t, 0.0, insight.DeltaPct)
}

// ---- income ----

func TestComputeIncome_UpIsPositive(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	prev := now.AddDate(0, -1, 0)
	// Income postings are stored with negative Amount in Paisa
	postings := []posting.Posting{
		p("Income:Salary", now, "-85000"),
		p("Income:Salary", prev, "-75893"),
	}
	insight := computeIncome(postings, now)
	require := assert.New(t)
	require.InDelta(12.0, insight.DeltaPct, 0.5)
	require.True(insight.Positive)
	require.False(insight.Suppress) // |Δ%| >= 2%
}

func TestComputeIncome_BelowThresholdSuppressed(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	prev := now.AddDate(0, -1, 0)
	postings := []posting.Posting{
		p("Income:Salary", now, "-85000"),
		p("Income:Salary", prev, "-84500"), // ~0.6% change
	}
	insight := computeIncome(postings, now)
	assert.True(t, insight.Suppress)
}

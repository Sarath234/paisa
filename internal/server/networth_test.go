package server

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestComputeMonthlySavings_Average(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	var postings []posting.Posting
	// 12 complete months: income -100000, expenses 70000 → savings 30000/month
	for i := 1; i <= 12; i++ {
		m := now.AddDate(0, -i, 0)
		postings = append(postings, p("Income:Salary", m, "-100000"))
		postings = append(postings, p("Expenses:Rent", m, "70000"))
	}
	// Current partial month (should be excluded)
	postings = append(postings, p("Income:Salary", now, "-100000"))
	postings = append(postings, p("Expenses:Rent", now, "10000"))

	result := computeMonthlySavings(postings, now)
	expected, _ := decimal.NewFromString("30000")
	assert.True(t, result.Equal(expected), "expected 30000, got %s", result)
}

func TestComputeMonthlySavings_ZeroIfNoPostings(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	result := computeMonthlySavings([]posting.Posting{}, now)
	assert.True(t, result.IsZero())
}

func TestComputeMonthlySavings_NegativeWhenSpendingExceedsIncome(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	var postings []posting.Posting
	for i := 1; i <= 12; i++ {
		m := now.AddDate(0, -i, 0)
		postings = append(postings, p("Income:Salary", m, "-50000"))
		postings = append(postings, p("Expenses:Rent", m, "80000"))
	}
	result := computeMonthlySavings(postings, now)
	assert.True(t, result.IsNegative())
}

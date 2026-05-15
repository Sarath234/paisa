package server

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/utils"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// expPosting creates a posting monthOffset months from today (negative = past, positive = future).
func expPosting(monthOffset int, account string, amount float64) posting.Posting {
	t := utils.Now().AddDate(0, monthOffset, 0)
	date := time.Date(t.Year(), t.Month(), 15, 0, 0, 0, 0, t.Location())
	return posting.Posting{
		Date:      date,
		Account:   account,
		Amount:    decimal.NewFromFloat(amount),
		Commodity: "INR",
	}
}

func TestComputeExpenseForecast_HistoricalFallback(t *testing.T) {
	actuals := []posting.Posting{
		expPosting(-1, "Expenses:Food", 6000),
		expPosting(-2, "Expenses:Food", 9000),
		expPosting(-3, "Expenses:Food", 9000),
	}
	result := computeExpenseForecast(actuals, []posting.Posting{})

	foodPostings := filterByAccount(result, "Expenses:Food")
	assert.NotEmpty(t, foodPostings, "expected forecast postings for Expenses:Food")

	// avg = (6000 + 9000 + 9000) / 3 = 8000
	expected := decimal.NewFromInt(8000)
	assert.True(t, foodPostings[0].Amount.Equal(expected),
		"expected avg %s, got %s", expected, foodPostings[0].Amount)
	assert.Equal(t, "historical", foodPostings[0].Note)
	assert.True(t, foodPostings[0].Forecast)
}

func TestComputeExpenseForecast_BudgetTakesPriority(t *testing.T) {
	actuals := []posting.Posting{
		expPosting(-1, "Expenses:Food", 6000),
		expPosting(-2, "Expenses:Food", 9000),
		expPosting(-3, "Expenses:Food", 9000),
	}
	// Budget posting for next month
	nextMonth := utils.Now().AddDate(0, 1, 0)
	budgetDate := time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, nextMonth.Location())
	budgetPostings := []posting.Posting{
		{
			Date:      budgetDate,
			Account:   "Expenses:Food",
			Amount:    decimal.NewFromInt(5000),
			Commodity: "INR",
			Forecast:  true,
		},
	}

	result := computeExpenseForecast(actuals, budgetPostings)

	nextMonthStr := nextMonth.Format("2006-01")
	var nextMonthFood []posting.Posting
	for _, p := range result {
		if p.Account == "Expenses:Food" && p.Date.Format("2006-01") == nextMonthStr {
			nextMonthFood = append(nextMonthFood, p)
		}
	}
	assert.NotEmpty(t, nextMonthFood)
	assert.True(t, nextMonthFood[0].Amount.Equal(decimal.NewFromInt(5000)),
		"budget should override historical avg")
	assert.Equal(t, "budget", nextMonthFood[0].Note)
}

func TestComputeExpenseForecast_SixMonthsOneFuturePerAccount(t *testing.T) {
	actuals := []posting.Posting{
		expPosting(-1, "Expenses:Food", 5000),
		expPosting(-2, "Expenses:Food", 5000),
		expPosting(-3, "Expenses:Food", 5000),
	}
	result := computeExpenseForecast(actuals, []posting.Posting{})

	foodPostings := filterByAccount(result, "Expenses:Food")
	assert.Len(t, foodPostings, 6, "expected one posting per future month")

	months := make(map[string]bool)
	for _, p := range foodPostings {
		months[p.Date.Format("2006-01")] = true
	}
	assert.Len(t, months, 6, "expected 6 distinct future months")
}

func TestComputeExpenseForecast_EmptyActualsProducesNoForecast(t *testing.T) {
	result := computeExpenseForecast([]posting.Posting{}, []posting.Posting{})
	assert.Empty(t, result)
}

func filterByAccount(postings []posting.Posting, account string) []posting.Posting {
	var result []posting.Posting
	for _, p := range postings {
		if p.Account == account {
			result = append(result, p)
		}
	}
	return result
}

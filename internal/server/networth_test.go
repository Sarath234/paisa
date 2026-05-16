package server

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/utils"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestComputeNetworthForecast_ReturnsSixPoints(t *testing.T) {
	result := computeNetworthForecast([]posting.Posting{}, decimal.NewFromInt(100000))
	assert.Len(t, result, 6)
}

func TestComputeNetworthForecast_FlatBalanceWithNoRecurring(t *testing.T) {
	startBalance := decimal.NewFromInt(500000)
	result := computeNetworthForecast([]posting.Posting{}, startBalance)
	for _, nw := range result {
		assert.True(t, nw.InvestmentAmount.Equal(startBalance),
			"Expected flat balance %s, got %s", startBalance, nw.InvestmentAmount)
	}
}

func TestComputeNetworthForecast_DatesAreFutureEndOfMonth(t *testing.T) {
	now := utils.Now()
	result := computeNetworthForecast([]posting.Posting{}, decimal.NewFromInt(100000))
	for i, nw := range result {
		expectedMonth := now.AddDate(0, i+1, 0)
		expectedEnd := utils.EndOfMonth(expectedMonth)
		assert.Equal(t, expectedEnd.Year(), nw.Date.Year(), "month %d year mismatch", i+1)
		assert.Equal(t, expectedEnd.Month(), nw.Date.Month(), "month %d month mismatch", i+1)
		assert.Equal(t, expectedEnd.Day(), nw.Date.Day(), "month %d day mismatch", i+1)
	}
}

func TestComputeNetworthForecast_MonotonicDates(t *testing.T) {
	result := computeNetworthForecast([]posting.Posting{}, decimal.NewFromInt(100000))
	for i := 1; i < len(result); i++ {
		assert.True(t, result[i].Date.After(result[i-1].Date),
			"expected dates to be strictly increasing")
	}
}

func TestComputeNetworthForecast_OnlyInvestmentAmountSet(t *testing.T) {
	result := computeNetworthForecast([]posting.Posting{}, decimal.NewFromInt(100000))
	for _, nw := range result {
		assert.True(t, nw.GainAmount.IsZero(), "GainAmount should be zero")
		assert.True(t, nw.WithdrawalAmount.IsZero(), "WithdrawalAmount should be zero")
		assert.False(t, nw.InvestmentAmount.IsZero(), "InvestmentAmount should not be zero")
		_ = time.Time{} // ensure time import used
	}
}

package server

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/utils"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestComputeNetworthForecast_ReturnsSixPoints(t *testing.T) {
	result := computeNetworthForecast([]Networth{})
	assert.Len(t, result, 6)
}

func TestComputeNetworthForecast_FlatBalanceWithNoHistory(t *testing.T) {
	// Single point with no prior history → delta = 0 → flat projection
	nw := Networth{Date: utils.Now(), BalanceAmount: decimal.NewFromInt(500000)}
	result := computeNetworthForecast([]Networth{nw})
	for _, r := range result {
		assert.True(t, r.InvestmentAmount.Equal(decimal.NewFromInt(500000)),
			"expected flat balance 500000, got %s", r.InvestmentAmount)
	}
}

func TestComputeNetworthForecast_DatesAreFutureEndOfMonth(t *testing.T) {
	now := utils.Now()
	result := computeNetworthForecast([]Networth{})
	for i, nw := range result {
		expectedMonth := now.AddDate(0, i+1, 0)
		expectedEnd := utils.EndOfMonth(expectedMonth)
		assert.Equal(t, expectedEnd.Year(), nw.Date.Year(), "month %d year mismatch", i+1)
		assert.Equal(t, expectedEnd.Month(), nw.Date.Month(), "month %d month mismatch", i+1)
		assert.Equal(t, expectedEnd.Day(), nw.Date.Day(), "month %d day mismatch", i+1)
	}
}

func TestComputeNetworthForecast_MonotonicDates(t *testing.T) {
	result := computeNetworthForecast([]Networth{})
	for i := 1; i < len(result); i++ {
		assert.True(t, result[i].Date.After(result[i-1].Date),
			"expected dates to be strictly increasing")
	}
}

func TestComputeNetworthForecast_OnlyInvestmentAmountSet(t *testing.T) {
	nw := Networth{Date: utils.Now(), BalanceAmount: decimal.NewFromInt(100000)}
	result := computeNetworthForecast([]Networth{nw})
	for _, r := range result {
		assert.True(t, r.GainAmount.IsZero(), "GainAmount should be zero")
		assert.True(t, r.WithdrawalAmount.IsZero(), "WithdrawalAmount should be zero")
		assert.False(t, r.InvestmentAmount.IsZero(), "InvestmentAmount should not be zero")
		_ = time.Time{}
	}
}

func TestComputeNetworthForecast_PositiveTrend(t *testing.T) {
	// 3 months of growth: 900000 → 1200000 (+100000/month)
	now := utils.Now()
	timeline := []Networth{
		{Date: now.AddDate(0, -3, 0), BalanceAmount: decimal.NewFromInt(900000)},
		{Date: now, BalanceAmount: decimal.NewFromInt(1200000)},
	}
	result := computeNetworthForecast(timeline)
	// First forecast month: 1200000 + 100000 = 1300000
	expected := decimal.NewFromInt(1300000)
	assert.True(t, result[0].InvestmentAmount.Equal(expected),
		"expected %s, got %s", expected, result[0].InvestmentAmount)
	for i := 1; i < len(result); i++ {
		assert.True(t, result[i].InvestmentAmount.GreaterThan(result[i-1].InvestmentAmount))
	}
}

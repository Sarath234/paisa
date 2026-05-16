package server

import (
	"sort"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/model/price"
	"github.com/ananthakumaran/paisa/internal/query"
	"github.com/ananthakumaran/paisa/internal/service"
	"github.com/ananthakumaran/paisa/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Networth struct {
	Date                time.Time       `json:"date"`
	InvestmentAmount    decimal.Decimal `json:"investmentAmount"`
	WithdrawalAmount    decimal.Decimal `json:"withdrawalAmount"`
	GainAmount          decimal.Decimal `json:"gainAmount"`
	BalanceAmount       decimal.Decimal `json:"balanceAmount"`
	BalanceUnits        decimal.Decimal `json:"balanceUnits"`
	NetInvestmentAmount decimal.Decimal `json:"netInvestmentAmount"`
}

func GetNetworth(db *gorm.DB) gin.H {
	postings := query.Init(db).Like("Assets:%", "Income:CapitalGains:%", "Liabilities:%").UntilToday().All()

	postings = service.PopulateMarketPrice(db, postings)
	networthTimeline := computeNetworthTimeline(db, postings, false)
	xirr := service.XIRR(db, postings)
	forecast := computeNetworthForecast(networthTimeline)

	return gin.H{"networthTimeline": networthTimeline, "xirr": xirr, "forecast": forecast}
}

// computeNetworthForecast projects 6 months forward using the average monthly
// networth change observed over the last 3 calendar months. This captures all
// cash flows (income, expenses, SIPs, market gains) without requiring
// explicit recurring-transaction tags.
func computeNetworthForecast(networthTimeline []Networth) []Networth {
	now := utils.Now()
	forecast := make([]Networth, 6)

	var currentBalance decimal.Decimal
	var monthlyDelta decimal.Decimal

	if len(networthTimeline) > 0 {
		last := networthTimeline[len(networthTimeline)-1]
		currentBalance = last.BalanceAmount

		// Walk backward to find the balance ~3 months ago (oldest first in timeline).
		threeMonthsAgo := now.AddDate(0, -3, 0)
		pastBalance := currentBalance // fallback: flat projection if history < 3 months
		for _, nw := range networthTimeline {
			if !nw.Date.After(threeMonthsAgo) {
				pastBalance = nw.BalanceAmount
			} else {
				break
			}
		}

		monthlyDelta = currentBalance.Sub(pastBalance).Div(decimal.NewFromInt(3))
	}

	balance := currentBalance
	for i := 0; i < 6; i++ {
		balance = balance.Add(monthlyDelta)
		monthEnd := utils.EndOfMonth(now.AddDate(0, i+1, 0))
		forecast[i] = Networth{Date: monthEnd, InvestmentAmount: balance}
	}
	return forecast
}

func GetCurrentNetworth(db *gorm.DB, allPostings []posting.Posting) gin.H {
	today := utils.EndOfToday()
	var postings []posting.Posting
	for _, p := range allPostings {
		if p.Date.Before(today) &&
			(strings.HasPrefix(p.Account, "Assets:") ||
				strings.HasPrefix(p.Account, "Income:CapitalGains:") ||
				strings.HasPrefix(p.Account, "Liabilities:")) {
			postings = append(postings, p)
		}
	}
	postings = service.PopulateMarketPrice(db, postings)
	networth := computeNetworth(db, postings)
	xirr := service.XIRR(db, postings)
	return gin.H{"networth": networth, "xirr": xirr}
}

func computeNetworth(db *gorm.DB, postings []posting.Posting) Networth {
	var networth Networth

	if len(postings) == 0 {
		return networth
	}

	var investment decimal.Decimal = decimal.Zero
	var withdrawal decimal.Decimal = decimal.Zero
	var balance decimal.Decimal = decimal.Zero

	now := utils.EndOfToday()
	for _, p := range postings {
		isInterest := service.IsInterest(db, p)
		isInterestRepayment := service.IsInterestRepayment(db, p)
		isStockSplit := service.IsStockSplit(db, p)
		isCapitalGains := service.IsCapitalGains(p)

		if isInterest || isInterestRepayment {
			balance = balance.Add(p.Amount)
		} else if isCapitalGains {
			withdrawal = withdrawal.Add(p.Amount.Neg())
		} else {
			if p.Amount.GreaterThan(decimal.Zero) && !isStockSplit {
				investment = investment.Add(p.Amount)
			}

			if p.Amount.LessThan(decimal.Zero) && !isStockSplit {
				withdrawal = withdrawal.Add(p.Amount.Neg())
			}

			balance = balance.Add(service.GetMarketPrice(db, p, now))
		}
	}

	gain := balance.Add(withdrawal).Sub(investment)
	netInvestment := investment.Sub(withdrawal)
	networth = Networth{
		Date:                now,
		InvestmentAmount:    investment,
		WithdrawalAmount:    withdrawal,
		GainAmount:          gain,
		BalanceAmount:       balance,
		NetInvestmentAmount: netInvestment,
	}

	return networth
}

// priceFloor returns the newest price with date <= target from a descending-sorted slice.
func priceFloor(prices []price.Price, target time.Time) price.Price {
	i := sort.Search(len(prices), func(i int) bool {
		return !prices[i].Date.After(target)
	})
	if i < len(prices) {
		return prices[i]
	}
	return price.Price{}
}

func computeNetworthTimeline(db *gorm.DB, postings []posting.Posting, computeBalanceUnits bool) []Networth {
	var networths []Networth

	var p posting.Posting

	if len(postings) == 0 {
		return []Networth{}
	}

	// Preload full price series for every non-currency commodity once to avoid
	// a per-day BTree lookup (which would be O(days × commodities × log n)).
	commoditySet := make(map[string]struct{})
	for _, p := range postings {
		if !utils.IsCurrency(p.Commodity) {
			commoditySet[p.Commodity] = struct{}{}
		}
	}
	preloadedPrices := make(map[string][]price.Price, len(commoditySet))
	for commodity := range commoditySet {
		preloadedPrices[commodity] = service.GetAllPrices(db, commodity)
	}

	type RunningSum struct {
		investment   decimal.Decimal
		withdrawal   decimal.Decimal
		balance      decimal.Decimal
		balanceUnits decimal.Decimal
	}

	accumulator := make(map[string]RunningSum)

	end := utils.EndOfToday()
	for start := postings[0].Date; start.Before(end); start = start.AddDate(0, 0, 1) {
		for len(postings) > 0 && (postings[0].Date.Before(start) || postings[0].Date.Equal(start)) {
			p, postings = postings[0], postings[1:]
			rs := accumulator[p.Commodity]

			isInterest := service.IsInterest(db, p)
			isCapitalGains := service.IsCapitalGains(p)

			if p.Amount.GreaterThan(decimal.Zero) && !isInterest {
				rs.investment = rs.investment.Add(p.Amount)
			}

			if p.Amount.LessThan(decimal.Zero) && !isInterest {
				rs.withdrawal = rs.withdrawal.Add(p.Amount.Neg())
			}

			if !isCapitalGains {
				rs.balance = rs.balance.Add(service.GetMarketPrice(db, p, start))
				rs.balanceUnits = rs.balanceUnits.Add(p.Quantity)
			}

			accumulator[p.Commodity] = rs

		}

		var investment decimal.Decimal = decimal.Zero
		var withdrawal decimal.Decimal = decimal.Zero
		var balance decimal.Decimal = decimal.Zero
		var balanceUnits decimal.Decimal = decimal.Zero

		for commodity, rs := range accumulator {
			investment = investment.Add(rs.investment)
			withdrawal = withdrawal.Add(rs.withdrawal)

			if utils.IsCurrency(commodity) {
				balance = balance.Add(rs.balance)
			} else {
				if computeBalanceUnits {
					balanceUnits = balanceUnits.Add(rs.balanceUnits)
				}
				pc := priceFloor(preloadedPrices[commodity], start)
				if !pc.Value.Equal(decimal.Zero) {
					balance = balance.Add(rs.balanceUnits.Mul(pc.Value))
				} else {
					balance = balance.Add(rs.balance)
				}
			}

		}

		gain := balance.Add(withdrawal).Sub(investment)
		netInvestment := investment.Sub(withdrawal)
		networths = append(networths, Networth{
			Date:                start,
			InvestmentAmount:    investment,
			WithdrawalAmount:    withdrawal,
			GainAmount:          gain,
			BalanceAmount:       balance,
			BalanceUnits:        balanceUnits,
			NetInvestmentAmount: netInvestment,
		})

		if len(postings) == 0 && balance.Abs().LessThan(decimal.NewFromFloat(0.01)) {
			break
		}
	}
	return networths
}

package assets

import (
	"strings"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"

	"github.com/ananthakumaran/paisa/internal/accounting"
	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/query"
	"github.com/ananthakumaran/paisa/internal/service"
	"github.com/ananthakumaran/paisa/internal/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AssetBreakdown struct {
	Group            string          `json:"group"`
	InvestmentAmount decimal.Decimal `json:"investmentAmount"`
	WithdrawalAmount decimal.Decimal `json:"withdrawalAmount"`
	MarketAmount     decimal.Decimal `json:"marketAmount"`
	BalanceUnits     decimal.Decimal `json:"balanceUnits"`
	LatestPrice      decimal.Decimal `json:"latestPrice"`
	XIRR             decimal.Decimal `json:"xirr"`
	GainAmount       decimal.Decimal `json:"gainAmount"`
	AbsoluteReturn   decimal.Decimal `json:"absoluteReturn"`
}

func GetCheckingBalance(db *gorm.DB, allPostings []posting.Posting) gin.H {
	var ps []posting.Posting
	for _, p := range allPostings {
		if accountPrefixMatch(p.Account, "Assets:Checking") || strings.HasPrefix(p.Account, "Income:CapitalGains:") {
			ps = append(ps, p)
		}
	}
	ps = service.PopulateMarketPrice(db, ps)
	return gin.H{"asset_breakdowns": ComputeBreakdowns(db, ps, false)}
}

func accountPrefixMatch(account, prefix string) bool {
	return account == prefix || strings.HasPrefix(account, prefix+":")
}

func GetBalance(db *gorm.DB) gin.H {
	return doGetBalance(db, "Assets:%", true)
}

func doGetBalance(db *gorm.DB, pattern string, rollup bool) gin.H {
	postings := query.Init(db).Like(pattern, "Income:CapitalGains:%").All()
	postings = service.PopulateMarketPrice(db, postings)
	breakdowns := ComputeBreakdowns(db, postings, rollup)
	return gin.H{"asset_breakdowns": breakdowns}
}

func ComputeBreakdowns(db *gorm.DB, postings []posting.Posting, rollup bool) map[string]AssetBreakdown {
	accounts := make(map[string]bool)
	for _, p := range postings {
		if service.IsCapitalGains(p) {
			continue
		}

		if rollup {
			var parts []string
			for _, part := range strings.Split(p.Account, ":") {
				parts = append(parts, part)
				accounts[strings.Join(parts, ":")] = false
			}
		}
		accounts[p.Account] = true

	}

	// Build postingsByGroup in one O(n × depth) pass instead of O(n × accounts) per group.
	postingsByGroup := make(map[string][]posting.Posting, len(accounts))
	for _, p := range postings {
		account := p.Account
		if service.IsCapitalGains(p) {
			account = service.CapitalGainsSourceAccount(p.Account)
		}
		parts := strings.Split(account, ":")
		for i := 1; i <= len(parts); i++ {
			group := strings.Join(parts[:i], ":")
			if _, exists := accounts[group]; exists {
				postingsByGroup[group] = append(postingsByGroup[group], p)
			}
		}
	}

	result := make(map[string]AssetBreakdown)
	for group, leaf := range accounts {
		result[group] = ComputeBreakdown(db, postingsByGroup[group], leaf, group)
	}

	return result
}

func ComputeBreakdown(db *gorm.DB, ps []posting.Posting, leaf bool, group string) AssetBreakdown {
	investmentAmount := lo.Reduce(ps, func(acc decimal.Decimal, p posting.Posting, _ int) decimal.Decimal {
		if utils.IsCheckingAccount(p.Account) || p.Amount.LessThan(decimal.Zero) || service.IsInterest(db, p) || service.IsStockSplit(db, p) || service.IsCapitalGains(p) {
			return acc
		} else {
			return acc.Add(p.Amount)
		}
	}, decimal.Zero)
	withdrawalAmount := lo.Reduce(ps, func(acc decimal.Decimal, p posting.Posting, _ int) decimal.Decimal {
		if !service.IsCapitalGains(p) && (utils.IsCheckingAccount(p.Account) || p.Amount.GreaterThan(decimal.Zero) || service.IsInterest(db, p) || service.IsStockSplit(db, p)) {
			return acc
		} else {
			return acc.Add(p.Amount.Neg())
		}
	}, decimal.Zero)
	psWithoutCapitalGains := lo.Filter(ps, func(p posting.Posting, _ int) bool {
		return !service.IsCapitalGains(p)
	})
	marketAmount := accounting.CurrentBalance(psWithoutCapitalGains)
	var balanceUnits decimal.Decimal
	if leaf {
		balanceUnits = lo.Reduce(ps, func(acc decimal.Decimal, p posting.Posting, _ int) decimal.Decimal {
			if !utils.IsCurrency(p.Commodity) {
				return acc.Add(p.Quantity)
			}
			return decimal.Zero
		}, decimal.Zero)
	}

	xirr := service.XIRR(db, ps)
	netInvestment := investmentAmount.Sub(withdrawalAmount)
	gainAmount := marketAmount.Sub(netInvestment)
	absoluteReturn := decimal.Zero
	if !investmentAmount.IsZero() {
		absoluteReturn = marketAmount.Sub(netInvestment).Div(investmentAmount)
	}
	return AssetBreakdown{
		InvestmentAmount: investmentAmount,
		WithdrawalAmount: withdrawalAmount,
		MarketAmount:     marketAmount,
		XIRR:             xirr,
		Group:            group,
		BalanceUnits:     balanceUnits,
		GainAmount:       gainAmount,
		AbsoluteReturn:   absoluteReturn,
	}
}

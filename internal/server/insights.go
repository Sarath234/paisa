package server

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/query"
	"github.com/ananthakumaran/paisa/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Insight struct {
	Type     string  `json:"type"`
	Title    string  `json:"title"`
	Body     string  `json:"body"`
	DeltaPct float64 `json:"delta_pct"`
	Positive bool    `json:"positive"`
	Suppress bool    `json:"suppress"`
}

func GetInsights(db *gorm.DB) gin.H {
	now := utils.Now()

	expensePostings := query.Init(db).Like("Expenses:%").LastNMonths(13).All()
	incomePostings := query.Init(db).Like("Income:%").LastNMonths(13).All()
	forecastPostings := query.Init(db).Like("Expenses:%").Forecast().UntilThisMonthEnd().All()

	// Use last month as the reference for income-based insights until the
	// current month's salary arrives.
	ref := effectiveMonth(incomePostings, now)

	insights := []Insight{}
	insights = append(insights, computeSpendCategory(expensePostings, now)...)
	insights = append(insights, computeSpendCategoryWeekly(expensePostings, now)...)
	insights = append(insights, computeSavingsRate(append(incomePostings, expensePostings...), ref))
	insights = append(insights, computeBudgetInsights(forecastPostings, expensePostings, now)...)
	if top := computeTopCategory(expensePostings, now); top.Title != "" {
		insights = append(insights, top)
	}
	if topW := computeTopCategoryWeekly(expensePostings, now); topW.Title != "" {
		insights = append(insights, topW)
	}
	insights = append(insights, computeIncome(incomePostings, ref))

	sort.SliceStable(insights, func(i, j int) bool {
		return math.Abs(insights[i].DeltaPct) > math.Abs(insights[j].DeltaPct)
	})

	return gin.H{"insights": insights}
}

func topLevelCategory(account string) string {
	parts := strings.SplitN(account, ":", 3)
	if len(parts) < 2 {
		return account
	}
	return parts[1]
}

func filterMonth(postings []posting.Posting, year int, month time.Month) []posting.Posting {
	return lo.Filter(postings, func(p posting.Posting, _ int) bool {
		return p.Date.Year() == year && p.Date.Month() == month
	})
}

func filterDateRange(postings []posting.Posting, from, to time.Time) []posting.Posting {
	return lo.Filter(postings, func(p posting.Posting, _ int) bool {
		return !p.Date.Before(from) && !p.Date.After(to)
	})
}

// effectiveMonth returns the reference month for income-based insights.
// Uses the current month only if a salary posting (account contains "salary",
// case-insensitive) has been recorded this month — matching the logic in the
// savings rate widget on the dashboard.
func effectiveMonth(incomePostings []posting.Posting, now time.Time) time.Time {
	cur := filterMonth(incomePostings, now.Year(), now.Month())
	for _, p := range cur {
		if strings.Contains(strings.ToLower(p.Account), "salary") {
			return now
		}
	}
	return now.AddDate(0, -1, 0)
}

func computeSpendCategory(postings []posting.Posting, now time.Time) []Insight {
	cur := filterMonth(postings, now.Year(), now.Month())
	prev := filterMonth(postings, now.AddDate(0, -1, 0).Year(), now.AddDate(0, -1, 0).Month())

	cur = lo.Filter(cur, func(p posting.Posting, _ int) bool {
		return !utils.IsSameOrParent(p.Account, "Expenses:Tax")
	})
	prev = lo.Filter(prev, func(p posting.Posting, _ int) bool {
		return !utils.IsSameOrParent(p.Account, "Expenses:Tax")
	})

	curByCategory := make(map[string]decimal.Decimal)
	for _, p := range cur {
		cat := topLevelCategory(p.Account)
		curByCategory[cat] = curByCategory[cat].Add(p.Amount)
	}
	prevByCategory := make(map[string]decimal.Decimal)
	for _, p := range prev {
		cat := topLevelCategory(p.Account)
		prevByCategory[cat] = prevByCategory[cat].Add(p.Amount)
	}

	categories := lo.Uniq(append(lo.Keys(curByCategory), lo.Keys(prevByCategory)...))
	sort.Strings(categories)

	var insights []Insight
	for _, cat := range categories {
		curAmt := curByCategory[cat]
		prevAmt := prevByCategory[cat]

		var deltaPct float64
		var suppress bool
		if prevAmt.IsZero() {
			deltaPct = 100
			suppress = false
		} else {
			f, _ := curAmt.Sub(prevAmt).Div(prevAmt).Mul(decimal.NewFromInt(100)).Float64()
			deltaPct = f
			suppress = math.Abs(deltaPct) < 10
		}

		curF, _ := curAmt.Float64()
		var body string
		if prevAmt.IsZero() {
			body = fmt.Sprintf("%s spending ₹%.0f this month — new spend", cat, curF)
		} else {
			dir := "more"
			if deltaPct < 0 {
				dir = "less"
			}
			body = fmt.Sprintf("%s spending ₹%.0f this month — %.0f%% %s than last month", cat, curF, math.Abs(deltaPct), dir)
		}

		insights = append(insights, Insight{
			Type:     "spend_category",
			Title:    cat,
			Body:     body,
			DeltaPct: deltaPct,
			Positive: deltaPct <= 0,
			Suppress: suppress,
		})
	}
	return insights
}

func computeSavingsRate(postings []posting.Posting, now time.Time) Insight {
	rateForMonth := func(year int, month time.Month) float64 {
		monthly := filterMonth(postings, year, month)

		incTotal := decimal.Zero
		expTotal := decimal.Zero
		for _, p := range monthly {
			if utils.IsSameOrParent(p.Account, "Income") {
				incTotal = incTotal.Add(p.Amount.Neg())
			} else if utils.IsSameOrParent(p.Account, "Expenses") {
				expTotal = expTotal.Add(p.Amount)
			}
		}
		if incTotal.IsZero() {
			return 0
		}
		r, _ := incTotal.Sub(expTotal).Div(incTotal).Mul(decimal.NewFromInt(100)).Float64()
		return r
	}

	rates := make([]float64, 13)
	for i := 0; i < 13; i++ {
		m := now.AddDate(0, -i, 0)
		rates[i] = rateForMonth(m.Year(), m.Month())
	}

	current := rates[0]
	sum := 0.0
	for _, r := range rates[1:] {
		sum += r
	}
	avg := sum / 12.0
	deltaPP := current - avg

	label := now.Format("January")
	var body string
	if deltaPP >= 0 {
		body = fmt.Sprintf("Savings rate %.0f%% in %s — %.0f pp above your 12-month average of %.0f%%", current, label, deltaPP, avg)
	} else {
		body = fmt.Sprintf("Savings rate %.0f%% in %s — %.0f pp below your 12-month average of %.0f%%", current, label, math.Abs(deltaPP), avg)
	}

	return Insight{
		Type:     "savings_rate",
		Title:    "Savings Rate",
		Body:     body,
		DeltaPct: deltaPP,
		Positive: deltaPP >= 0,
		Suppress: math.Abs(deltaPP) < 5,
	}
}

func computeBudgetInsights(budgetedPostings, actualPostings []posting.Posting, now time.Time) []Insight {
	curBudgeted := filterMonth(budgetedPostings, now.Year(), now.Month())
	curActual := filterMonth(actualPostings, now.Year(), now.Month())

	budgetByCategory := make(map[string]decimal.Decimal)
	for _, p := range curBudgeted {
		cat := topLevelCategory(p.Account)
		budgetByCategory[cat] = budgetByCategory[cat].Add(p.Amount)
	}

	if len(budgetByCategory) == 0 {
		return nil
	}

	actualByCategory := make(map[string]decimal.Decimal)
	for _, p := range curActual {
		cat := topLevelCategory(p.Account)
		actualByCategory[cat] = actualByCategory[cat].Add(p.Amount)
	}

	budgetCategories := lo.Keys(budgetByCategory)
	sort.Strings(budgetCategories)
	var insights []Insight
	for _, cat := range budgetCategories {
		budgeted := budgetByCategory[cat]
		if budgeted.IsZero() {
			continue
		}
		actual := actualByCategory[cat]
		variance := budgeted.Sub(actual)
		variancePct, _ := variance.Div(budgeted).Mul(decimal.NewFromInt(100)).Float64()
		diffF, _ := variance.Abs().Float64()

		var body string
		if variancePct >= 0 {
			body = fmt.Sprintf("%s — ₹%.0f under budget this month (%.0f%% variance)", cat, diffF, math.Abs(variancePct))
		} else {
			body = fmt.Sprintf("%s — ₹%.0f over budget this month (%.0f%% variance)", cat, diffF, math.Abs(variancePct))
		}

		insights = append(insights, Insight{
			Type:     "budget",
			Title:    cat,
			Body:     body,
			DeltaPct: variancePct,
			Positive: variancePct >= 0,
			Suppress: math.Abs(variancePct) < 5,
		})
	}
	return insights
}

func computeTopCategory(postings []posting.Posting, now time.Time) Insight {
	cur := filterMonth(postings, now.Year(), now.Month())
	cur = lo.Filter(cur, func(p posting.Posting, _ int) bool {
		return !utils.IsSameOrParent(p.Account, "Expenses:Tax")
	})

	if len(cur) == 0 {
		return Insight{}
	}

	byCategory := make(map[string]decimal.Decimal)
	total := decimal.Zero
	for _, p := range cur {
		cat := topLevelCategory(p.Account)
		byCategory[cat] = byCategory[cat].Add(p.Amount)
		total = total.Add(p.Amount)
	}

	topCat := ""
	topAmt := decimal.Zero
	for cat, amt := range byCategory {
		if amt.GreaterThan(topAmt) {
			topAmt = amt
			topCat = cat
		}
	}

	pct, _ := topAmt.Div(total).Mul(decimal.NewFromInt(100)).Float64()
	body := fmt.Sprintf("%s is your #1 expense this month at %.0f%% of total spend", topCat, pct)

	return Insight{
		Type:     "top_category",
		Title:    topCat,
		Body:     body,
		DeltaPct: 0,
		Positive: true,
		Suppress: false,
	}
}

func computeSpendCategoryWeekly(postings []posting.Posting, now time.Time) []Insight {
	startOfDay := func(t time.Time) time.Time { return t.Truncate(24 * time.Hour) }
	curFrom := startOfDay(now.AddDate(0, 0, -6))
	curTo := utils.EndOfDay(now)
	prevFrom := startOfDay(now.AddDate(0, 0, -13))
	prevTo := utils.EndOfDay(now.AddDate(0, 0, -7))

	cur := filterDateRange(postings, curFrom, curTo)
	prev := filterDateRange(postings, prevFrom, prevTo)

	cur = lo.Filter(cur, func(p posting.Posting, _ int) bool {
		return !utils.IsSameOrParent(p.Account, "Expenses:Tax")
	})
	prev = lo.Filter(prev, func(p posting.Posting, _ int) bool {
		return !utils.IsSameOrParent(p.Account, "Expenses:Tax")
	})

	curByCategory := make(map[string]decimal.Decimal)
	for _, p := range cur {
		cat := topLevelCategory(p.Account)
		curByCategory[cat] = curByCategory[cat].Add(p.Amount)
	}
	prevByCategory := make(map[string]decimal.Decimal)
	for _, p := range prev {
		cat := topLevelCategory(p.Account)
		prevByCategory[cat] = prevByCategory[cat].Add(p.Amount)
	}

	categories := lo.Uniq(append(lo.Keys(curByCategory), lo.Keys(prevByCategory)...))
	sort.Strings(categories)

	var insights []Insight
	for _, cat := range categories {
		curAmt := curByCategory[cat]
		prevAmt := prevByCategory[cat]

		var deltaPct float64
		var suppress bool
		if prevAmt.IsZero() {
			deltaPct = 100
			suppress = false
		} else {
			f, _ := curAmt.Sub(prevAmt).Div(prevAmt).Mul(decimal.NewFromInt(100)).Float64()
			deltaPct = f
			suppress = math.Abs(deltaPct) < 15
		}

		curF, _ := curAmt.Float64()
		var body string
		if prevAmt.IsZero() {
			body = fmt.Sprintf("%s spending ₹%.0f this week — new spend", cat, curF)
		} else {
			dir := "more"
			if deltaPct < 0 {
				dir = "less"
			}
			body = fmt.Sprintf("%s spending ₹%.0f this week — %.0f%% %s than prior week", cat, curF, math.Abs(deltaPct), dir)
		}

		insights = append(insights, Insight{
			Type:     "spend_category_weekly",
			Title:    cat,
			Body:     body,
			DeltaPct: deltaPct,
			Positive: deltaPct <= 0,
			Suppress: suppress,
		})
	}
	return insights
}

func computeTopCategoryWeekly(postings []posting.Posting, now time.Time) Insight {
	startOfDay := func(t time.Time) time.Time { return t.Truncate(24 * time.Hour) }
	curFrom := startOfDay(now.AddDate(0, 0, -6))
	curTo := utils.EndOfDay(now)

	cur := filterDateRange(postings, curFrom, curTo)
	cur = lo.Filter(cur, func(p posting.Posting, _ int) bool {
		return !utils.IsSameOrParent(p.Account, "Expenses:Tax")
	})

	if len(cur) == 0 {
		return Insight{}
	}

	byCategory := make(map[string]decimal.Decimal)
	total := decimal.Zero
	for _, p := range cur {
		cat := topLevelCategory(p.Account)
		byCategory[cat] = byCategory[cat].Add(p.Amount)
		total = total.Add(p.Amount)
	}

	topCat := ""
	topAmt := decimal.Zero
	for cat, amt := range byCategory {
		if amt.GreaterThan(topAmt) {
			topAmt = amt
			topCat = cat
		}
	}

	pct, _ := topAmt.Div(total).Mul(decimal.NewFromInt(100)).Float64()
	body := fmt.Sprintf("%s is your #1 expense this week at %.0f%% of total spend", topCat, pct)

	return Insight{
		Type:     "top_category_weekly",
		Title:    topCat,
		Body:     body,
		DeltaPct: 0,
		Positive: true,
		Suppress: false,
	}
}

func computeIncome(postings []posting.Posting, now time.Time) Insight {
	sum := func(year int, month time.Month) decimal.Decimal {
		total := decimal.Zero
		for _, p := range filterMonth(postings, year, month) {
			total = total.Add(p.Amount.Neg())
		}
		return total
	}

	cur := sum(now.Year(), now.Month())
	prev := sum(now.AddDate(0, -1, 0).Year(), now.AddDate(0, -1, 0).Month())

	var deltaPct float64
	if !prev.IsZero() {
		f, _ := cur.Sub(prev).Div(prev).Mul(decimal.NewFromInt(100)).Float64()
		deltaPct = f
	}

	label := now.Format("January")
	curF, _ := cur.Float64()
	var body string
	if prev.IsZero() {
		body = fmt.Sprintf("Income ₹%.0f in %s", curF, label)
	} else {
		dir := "up"
		if deltaPct < 0 {
			dir = "down"
		}
		body = fmt.Sprintf("Income ₹%.0f in %s — %s %.0f%% vs prior month", curF, label, dir, math.Abs(deltaPct))
	}

	return Insight{
		Type:     "income",
		Title:    "Income",
		Body:     body,
		DeltaPct: deltaPct,
		Positive: deltaPct >= 0,
		Suppress: math.Abs(deltaPct) < 2,
	}
}

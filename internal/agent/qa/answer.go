// internal/agent/qa/answer.go
package qa

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

// Answerer resolves a Query against the paisa API and formats the reply.
// All arithmetic happens here, in Go — never in the LLM.
type Answerer struct {
	Client *paisaclient.Client
	Now    func() time.Time
}

func (a *Answerer) Answer(q *Query) (string, error) {
	switch q.Intent {
	case "expense_summary":
		return a.expenseSummary(q)
	case "networth":
		return a.networth()
	case "account_balance":
		return a.accountBalance(q)
	case "budget_status":
		return a.budgetStatus(q)
	default:
		return "", fmt.Errorf("unhandled intent %q", q.Intent)
	}
}

func (a *Answerer) expenseSummary(q *Query) (string, error) {
	period, known := ResolvePeriod(q.Period, a.Now())
	postings, err := a.Client.Expenses()
	if err != nil {
		return "", err
	}

	accounts := uniqueAccounts(postings)
	matched := accounts
	scope := "All expenses"
	if q.Category != "" {
		matched = MatchAccounts(q.Category, accounts)
		if len(matched) == 0 {
			return fmt.Sprintf("No expense account matches %q.\nKnown accounts: %s",
				q.Category, strings.Join(accounts, ", ")), nil
		}
		scope = strings.Join(matched, ", ")
	}

	total, txns := sumInPeriod(postings, matched, period)
	reply := fmt.Sprintf("💸 %s — %s: %s (%d txns", scope, period.Label, FormatINR(total), txns)

	if period.IsSingleMonth() {
		prevStart := period.Start.AddDate(0, -1, 0)
		prev := Period{Start: prevStart, End: period.Start, Label: prevStart.Format("Jan 2006")}
		prevTotal, _ := sumInPeriod(postings, matched, prev)
		reply += fmt.Sprintf("; %s: %s", prev.Label, FormatINR(prevTotal))
	}
	reply += ")"

	if !known && q.Period != "" {
		reply += fmt.Sprintf("\n(didn't understand period %q — showing %s)", q.Period, period.Label)
	}
	return reply, nil
}

func (a *Answerer) networth() (string, error) {
	nw, err := a.Client.Networth()
	if err != nil {
		return "", err
	}
	if len(nw.Timeline) == 0 {
		return "No net worth data yet — has paisa synced?", nil
	}
	last := nw.Timeline[len(nw.Timeline)-1]
	return fmt.Sprintf("💰 Net worth: %s\nInvested: %s | Gain: %s | XIRR: %.1f%%",
		FormatINR(last.BalanceAmount), FormatINR(last.NetInvestmentAmount),
		FormatINR(last.GainAmount), nw.XIRR), nil
}

func (a *Answerer) accountBalance(q *Query) (string, error) {
	breakdowns, err := a.Client.AssetBreakdowns()
	if err != nil {
		return "", err
	}
	groups := make([]string, 0, len(breakdowns))
	for g := range breakdowns {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	var selected []string
	if q.Account == "" {
		for _, g := range groups { // top-level groups only
			if !strings.Contains(g, ":") {
				selected = append(selected, g)
			}
		}
	} else {
		selected = MatchAccounts(q.Account, groups)
		if len(selected) == 0 {
			return fmt.Sprintf("No account matches %q.\nKnown accounts: %s",
				q.Account, strings.Join(groups, ", ")), nil
		}
	}

	var lines []string
	for _, g := range selected {
		lines = append(lines, fmt.Sprintf("%s: %s", g, FormatINR(breakdowns[g].MarketAmount)))
	}
	return "🏦 " + strings.Join(lines, "\n"), nil
}

func (a *Answerer) budgetStatus(q *Query) (string, error) {
	now := a.Now()
	month := now.Format("2006-01")
	budgets, err := a.Client.BudgetForMonth(month)
	if err != nil {
		return "", err
	}
	if len(budgets) == 0 {
		return fmt.Sprintf("No budget configured for %s.", now.Format("Jan 2006")), nil
	}

	if q.Category != "" {
		accounts := make([]string, len(budgets))
		for i, b := range budgets {
			accounts[i] = b.Account
		}
		matched := MatchAccounts(q.Category, accounts)
		if len(matched) == 0 {
			return fmt.Sprintf("No budget account matches %q.\nKnown accounts: %s",
				q.Category, strings.Join(accounts, ", ")), nil
		}
		matchedSet := make(map[string]bool, len(matched))
		for _, m := range matched {
			matchedSet[m] = true
		}
		var filtered []paisaclient.AccountBudget
		for _, b := range budgets {
			if matchedSet[b.Account] {
				filtered = append(filtered, b)
			}
		}
		budgets = filtered
	}

	lines := []string{fmt.Sprintf("📊 Budget — %s", now.Format("Jan 2006"))}
	for _, b := range budgets {
		line := fmt.Sprintf("%s: %s of %s", b.Account, FormatINR(b.Actual), FormatINR(b.Forecast))
		if b.Actual > b.Forecast {
			line += fmt.Sprintf(" ⚠️ over by %s", FormatINR(b.Actual-b.Forecast))
		} else {
			line += fmt.Sprintf(" (%s left)", FormatINR(b.Forecast-b.Actual))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func uniqueAccounts(postings []paisaclient.Posting) []string {
	seen := make(map[string]bool)
	var out []string
	for _, p := range postings {
		if !seen[p.Account] {
			seen[p.Account] = true
			out = append(out, p.Account)
		}
	}
	sort.Strings(out)
	return out
}

// sumInPeriod totals postings whose account is one of the matched accounts
// (or a child of one) and whose date falls in [period.Start, period.End).
// Returns the total and the number of distinct transactions.
func sumInPeriod(postings []paisaclient.Posting, accounts []string, period Period) (float64, int) {
	accountSet := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		accountSet[a] = true
	}
	inScope := func(account string) bool {
		if accountSet[account] {
			return true
		}
		for a := range accountSet {
			if strings.HasPrefix(account, a+":") {
				return true
			}
		}
		return false
	}

	var total float64
	txns := make(map[string]bool)
	for _, p := range postings {
		if !inScope(p.Account) {
			continue
		}
		if p.Date.Before(period.Start) || !p.Date.Before(period.End) {
			continue
		}
		total += p.Amount
		txns[p.TransactionID] = true
	}
	return total, len(txns)
}

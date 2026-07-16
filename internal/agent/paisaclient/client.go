// internal/agent/paisaclient/client.go
// Package paisaclient is a thin typed HTTP client for the paisa server API.
// Endpoints return full dumps; filtering/aggregation happens in the caller.
package paisaclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) get(path string, out any) error {
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("paisa %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("paisa %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("paisa %s: decode: %w", path, err)
	}
	return nil
}

// Posting is the subset of paisa's posting model the agent needs.
type Posting struct {
	Date          time.Time `json:"date"`
	Payee         string    `json:"payee"`
	Account       string    `json:"account"`
	Amount        float64   `json:"amount"`
	TransactionID string    `json:"transaction_id"`
}

// Expenses returns all expense postings (GET /api/expense).
func (c *Client) Expenses() ([]Posting, error) {
	var r struct {
		Expenses []Posting `json:"expenses"`
	}
	if err := c.get("/api/expense", &r); err != nil {
		return nil, err
	}
	return r.Expenses, nil
}

type NetworthPoint struct {
	Date                time.Time `json:"date"`
	BalanceAmount       float64   `json:"balanceAmount"`
	GainAmount          float64   `json:"gainAmount"`
	NetInvestmentAmount float64   `json:"netInvestmentAmount"`
}

type NetworthSummary struct {
	Timeline []NetworthPoint
	XIRR     float64
}

// Networth returns the networth timeline and overall XIRR (GET /api/networth).
func (c *Client) Networth() (*NetworthSummary, error) {
	var r struct {
		NetworthTimeline []NetworthPoint `json:"networthTimeline"`
		XIRR             float64         `json:"xirr"`
	}
	if err := c.get("/api/networth", &r); err != nil {
		return nil, err
	}
	return &NetworthSummary{Timeline: r.NetworthTimeline, XIRR: r.XIRR}, nil
}

type AssetBreakdown struct {
	Group        string  `json:"group"`
	MarketAmount float64 `json:"marketAmount"`
}

// AssetBreakdowns returns balances keyed by account group, including rollup
// parents like "Assets" and "Assets:Checking" (GET /api/assets/balance).
func (c *Client) AssetBreakdowns() (map[string]AssetBreakdown, error) {
	var r struct {
		AssetBreakdowns map[string]AssetBreakdown `json:"asset_breakdowns"`
	}
	if err := c.get("/api/assets/balance", &r); err != nil {
		return nil, err
	}
	return r.AssetBreakdowns, nil
}

type AccountBudget struct {
	Account   string  `json:"account"`
	Forecast  float64 `json:"forecast"`
	Actual    float64 `json:"actual"`
	Available float64 `json:"available"`
}

// BudgetForMonth returns per-account budgets for a month key like "2026-06".
// Returns nil (no error) when the month has no budget (GET /api/budget).
func (c *Client) BudgetForMonth(month string) ([]AccountBudget, error) {
	var r struct {
		BudgetsByMonth map[string]struct {
			Accounts []AccountBudget `json:"accounts"`
		} `json:"budgetsByMonth"`
	}
	if err := c.get("/api/budget", &r); err != nil {
		return nil, err
	}
	return r.BudgetsByMonth[month].Accounts, nil
}

// Postings returns all postings from the ledger (GET /api/ledger).
func (c *Client) Postings() ([]Posting, error) {
	var r struct {
		Postings []Posting `json:"postings"`
	}
	if err := c.get("/api/ledger", &r); err != nil {
		return nil, err
	}
	return r.Postings, nil
}

type CreditCardBill struct {
	StatementStartDate time.Time  `json:"statementStartDate"`
	StatementEndDate   time.Time  `json:"statementEndDate"`
	DueDate            time.Time  `json:"dueDate"`
	PaidDate           *time.Time `json:"paidDate"`
	ClosingBalance     float64    `json:"closingBalance"`
	Postings           []Posting  `json:"postings"`
}

type CreditCardSummary struct {
	Account     string           `json:"account"`
	Balance     float64          `json:"balance"`
	CreditLimit float64          `json:"creditLimit"`
	Bills       []CreditCardBill `json:"bills"`
}

// CreditCards returns configured credit cards with computed statement bills.
// Bills in this response have EMPTY Postings — the list endpoint omits them
// for performance. Use CreditCard(account) to fetch bill postings for a
// single card (GET /api/credit_cards).
func (c *Client) CreditCards() ([]CreditCardSummary, error) {
	var r struct {
		CreditCards []CreditCardSummary `json:"creditCards"`
	}
	if err := c.get("/api/credit_cards", &r); err != nil {
		return nil, err
	}
	return r.CreditCards, nil
}

// CreditCard returns one card's detail including bill postings
// (GET /api/credit_cards/:account). Returns (nil, nil) when the account
// isn't configured on the server.
func (c *Client) CreditCard(account string) (*CreditCardSummary, error) {
	var r struct {
		CreditCard *CreditCardSummary `json:"creditCard"`
		Found      bool               `json:"found"`
	}
	if err := c.get("/api/credit_cards/"+url.PathEscape(account), &r); err != nil {
		return nil, err
	}
	if !r.Found {
		return nil, nil
	}
	return r.CreditCard, nil
}

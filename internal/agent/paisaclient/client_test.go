// internal/agent/paisaclient/client_test.go
package paisaclient

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fixtureServer serves canned JSON per API path.
func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	fixtures := map[string]string{
		"/api/expense": `{"expenses": [
			{"date": "2026-06-03T00:00:00Z", "payee": "Swiggy", "account": "Expenses:Food:Hyd", "amount": 215.5, "transaction_id": "t1"},
			{"date": "2026-05-10T00:00:00Z", "payee": "BigBazaar", "account": "Expenses:Groceries", "amount": 1200, "transaction_id": "t2"}
		]}`,
		"/api/networth": `{"networthTimeline": [
			{"date": "2026-06-09T00:00:00Z", "balanceAmount": 500000, "gainAmount": 50000, "netInvestmentAmount": 450000}
		], "xirr": 12.5}`,
		"/api/assets/balance": `{"asset_breakdowns": {
			"Assets": {"group": "Assets", "marketAmount": 500000},
			"Assets:Checking:Axis": {"group": "Assets:Checking:Axis", "marketAmount": 80000}
		}}`,
		"/api/budget": `{"budgetsByMonth": {
			"2026-06": {"accounts": [
				{"account": "Expenses:Food", "forecast": 5000, "actual": 4520, "available": 480}
			]}
		}}`,
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := fixtures[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
}

func TestExpenses(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()
	c := New(srv.URL)
	got, err := c.Expenses()
	if err != nil {
		t.Fatalf("Expenses: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d postings, want 2", len(got))
	}
	if got[0].Account != "Expenses:Food:Hyd" || got[0].Amount != 215.5 || got[0].TransactionID != "t1" {
		t.Errorf("posting[0] = %+v", got[0])
	}
	if got[0].Date.Month() != 6 {
		t.Errorf("date parsed wrong: %v", got[0].Date)
	}
}

func TestNetworth(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()
	got, err := New(srv.URL).Networth()
	if err != nil {
		t.Fatalf("Networth: %v", err)
	}
	if got.XIRR != 12.5 || len(got.Timeline) != 1 || got.Timeline[0].BalanceAmount != 500000 {
		t.Errorf("got %+v", got)
	}
}

func TestAssetBreakdowns(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()
	got, err := New(srv.URL).AssetBreakdowns()
	if err != nil {
		t.Fatalf("AssetBreakdowns: %v", err)
	}
	if got["Assets:Checking:Axis"].MarketAmount != 80000 {
		t.Errorf("got %+v", got)
	}
}

func TestBudgetForMonth(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()
	got, err := New(srv.URL).BudgetForMonth("2026-06")
	if err != nil {
		t.Fatalf("BudgetForMonth: %v", err)
	}
	if len(got) != 1 || got[0].Account != "Expenses:Food" || got[0].Forecast != 5000 || got[0].Actual != 4520 {
		t.Errorf("got %+v", got)
	}
	empty, err := New(srv.URL).BudgetForMonth("2026-01")
	if err != nil {
		t.Fatalf("BudgetForMonth empty month: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("want empty for unbudgeted month, got %+v", empty)
	}
}

func TestServerUnreachable(t *testing.T) {
	_, err := New("http://127.0.0.1:1").Expenses()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	_, err := New(srv.URL).Expenses()
	if err == nil || !strings.Contains(err.Error(), "status 503") {
		t.Fatalf("want status-503 error, got %v", err)
	}
}

func TestDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>not json</html>"))
	}))
	defer srv.Close()
	_, err := New(srv.URL).Expenses()
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("want decode error, got %v", err)
	}
}

func TestCreditCards(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/credit_cards" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"creditCards":[{
			"account":"Liabilities:CreditCard:Axis",
			"balance":23450.5,
			"creditLimit":100000,
			"bills":[{
				"statementStartDate":"2026-06-16T00:00:00+05:30",
				"statementEndDate":"2026-07-15T00:00:00+05:30",
				"dueDate":"2026-07-28T00:00:00+05:30",
				"paidDate":null,
				"closingBalance":31200,
				"postings":[{"date":"2026-06-20T00:00:00+05:30","payee":"INTEREST CHARGE","account":"Liabilities:CreditCard:Axis","amount":-1240}]
			}]
		}]}`)
	}))
	defer srv.Close()

	cards, err := New(srv.URL).CreditCards()
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].Account != "Liabilities:CreditCard:Axis" {
		t.Fatalf("cards: %+v", cards)
	}
	c := cards[0]
	if c.Balance != 23450.5 || c.CreditLimit != 100000 {
		t.Fatalf("balance/limit: %+v", c)
	}
	if len(c.Bills) != 1 || c.Bills[0].PaidDate != nil || c.Bills[0].ClosingBalance != 31200 {
		t.Fatalf("bills: %+v", c.Bills)
	}
	if len(c.Bills[0].Postings) != 1 || c.Bills[0].Postings[0].Payee != "INTEREST CHARGE" {
		t.Fatalf("postings: %+v", c.Bills[0].Postings)
	}
}

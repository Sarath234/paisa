// internal/agent/qa/answer_test.go
package qa

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

func paisaStub(t *testing.T) *httptest.Server {
	t.Helper()
	fixtures := map[string]string{
		"/api/expense": `{"expenses": [
			{"date": "2026-06-03T00:00:00Z", "payee": "Swiggy", "account": "Expenses:Food:Hyd", "amount": 215.5, "transaction_id": "t1"},
			{"date": "2026-06-05T00:00:00Z", "payee": "Zomato", "account": "Expenses:Food:Hyd", "amount": 300, "transaction_id": "t2"},
			{"date": "2026-05-10T00:00:00Z", "payee": "Swiggy", "account": "Expenses:Food:Hyd", "amount": 980, "transaction_id": "t3"},
			{"date": "2026-06-07T00:00:00Z", "payee": "BigBazaar", "account": "Expenses:Groceries", "amount": 1200, "transaction_id": "t4"}
		]}`,
		"/api/networth": `{"networthTimeline": [
			{"date": "2026-06-01T00:00:00Z", "balanceAmount": 490000, "gainAmount": 40000, "netInvestmentAmount": 450000},
			{"date": "2026-06-09T00:00:00Z", "balanceAmount": 500000, "gainAmount": 50000, "netInvestmentAmount": 450000}
		], "xirr": 12.5}`,
		"/api/assets/balance": `{"asset_breakdowns": {
			"Assets": {"group": "Assets", "marketAmount": 500000},
			"Assets:Checking": {"group": "Assets:Checking", "marketAmount": 130000},
			"Assets:Checking:Axis": {"group": "Assets:Checking:Axis", "marketAmount": 80000},
			"Assets:Checking:HDFC": {"group": "Assets:Checking:HDFC", "marketAmount": 50000}
		}}`,
		"/api/budget": `{"budgetsByMonth": {
			"2026-06": {"accounts": [
				{"account": "Expenses:Food", "forecast": 5000, "actual": 4520, "available": 480},
				{"account": "Expenses:Groceries", "forecast": 1000, "actual": 1200, "available": -200}
			]}
		}}`,
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := fixtures[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
}

func testAnswerer(t *testing.T, srv *httptest.Server) *Answerer {
	t.Helper()
	return &Answerer{
		Client: paisaclient.New(srv.URL),
		Now:    func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
	}
}

func TestAnswerExpenseSummary(t *testing.T) {
	srv := paisaStub(t)
	defer srv.Close()
	a := testAnswerer(t, srv)

	got, err := a.Answer(&Query{Intent: "expense_summary", Category: "food", Period: "this_month"})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	// June: 215.5 + 300 = 515.50 across 2 txns; May comparison: 980
	for _, want := range []string{"Expenses:Food:Hyd", "Jun 2026", "₹515.50", "2 txns", "₹980"} {
		if !strings.Contains(got, want) {
			t.Errorf("reply missing %q:\n%s", want, got)
		}
	}
}

func TestAnswerExpenseSummaryAllCategories(t *testing.T) {
	srv := paisaStub(t)
	defer srv.Close()
	got, err := testAnswerer(t, srv).Answer(&Query{Intent: "expense_summary", Period: "this_month"})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	// 215.5 + 300 + 1200 = 1,715.50
	if !strings.Contains(got, "₹1,715.50") || !strings.Contains(got, "3 txns") {
		t.Errorf("got:\n%s", got)
	}
}

func TestAnswerExpenseSummaryNoMatch(t *testing.T) {
	srv := paisaStub(t)
	defer srv.Close()
	got, err := testAnswerer(t, srv).Answer(&Query{Intent: "expense_summary", Category: "travel"})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if !strings.Contains(got, "travel") || !strings.Contains(got, "Expenses:Food:Hyd") {
		t.Errorf("no-match reply should name the query and list known accounts:\n%s", got)
	}
}

func TestAnswerNetworth(t *testing.T) {
	srv := paisaStub(t)
	defer srv.Close()
	got, err := testAnswerer(t, srv).Answer(&Query{Intent: "networth"})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	for _, want := range []string{"₹5,00,000", "₹4,50,000", "₹50,000", "12.5%"} {
		if !strings.Contains(got, want) {
			t.Errorf("reply missing %q:\n%s", want, got)
		}
	}
}

func TestAnswerAccountBalance(t *testing.T) {
	srv := paisaStub(t)
	defer srv.Close()
	got, err := testAnswerer(t, srv).Answer(&Query{Intent: "account_balance", Account: "axis"})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if !strings.Contains(got, "Assets:Checking:Axis") || !strings.Contains(got, "₹80,000") {
		t.Errorf("got:\n%s", got)
	}
}

func TestAnswerAccountBalanceNoAccount(t *testing.T) {
	srv := paisaStub(t)
	defer srv.Close()
	got, err := testAnswerer(t, srv).Answer(&Query{Intent: "account_balance"})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	// no account specified → top-level groups only
	if !strings.Contains(got, "Assets") || strings.Contains(got, "Assets:Checking:Axis") {
		t.Errorf("want top-level summary only:\n%s", got)
	}
}

func TestAnswerBudgetStatus(t *testing.T) {
	srv := paisaStub(t)
	defer srv.Close()
	got, err := testAnswerer(t, srv).Answer(&Query{Intent: "budget_status"})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	for _, want := range []string{"Expenses:Food", "₹4,520", "₹5,000", "⚠️"} {
		if !strings.Contains(got, want) {
			t.Errorf("reply missing %q:\n%s", want, got)
		}
	}
}

func TestAnswerBudgetStatusNoMatch(t *testing.T) {
	srv := paisaStub(t)
	defer srv.Close()
	got, err := testAnswerer(t, srv).Answer(&Query{Intent: "budget_status", Category: "travel"})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if !strings.Contains(got, "travel") || !strings.Contains(got, "Expenses:Food") {
		t.Errorf("no-match reply should name the query and list budget accounts:\n%s", got)
	}
	if strings.Contains(got, "⚠️") {
		t.Errorf("no-match reply must not include the full budget table:\n%s", got)
	}
}

func TestAnswerServerDown(t *testing.T) {
	a := &Answerer{Client: paisaclient.New("http://127.0.0.1:1"), Now: time.Now}
	if _, err := a.Answer(&Query{Intent: "networth"}); err == nil {
		t.Fatal("expected error when paisa is unreachable")
	}
}

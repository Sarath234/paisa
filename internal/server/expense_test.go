package server

import (
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func txPosting(txID, account string, amount string) posting.Posting {
	v, _ := decimal.NewFromString(amount)
	return posting.Posting{
		Account:       account,
		Date:          time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Amount:        v,
		Commodity:     "INR",
		TransactionID: txID,
	}
}

// TestComputeSegmentedGraph_Refund verifies that a return/refund
// (Expenses → Assets) is correctly netted so sub-category amounts reflect
// net spend (purchases minus refunds), and no backward hierarchy links remain.
func TestComputeSegmentedGraph_Refund(t *testing.T) {
	postings := []posting.Posting{
		// Normal expense: ₹1000 food from checking
		txPosting("tx-food", "Expenses:Food:Restaurant", "1000"),
		txPosting("tx-food", "Assets:Checking", "-1000"),
		// Partial refund: ₹500 Amazon refund back to checking
		txPosting("tx-refund", "Expenses:Food:Restaurant", "-500"),
		txPosting("tx-refund", "Assets:Checking", "500"),
	}

	graph := computeSegmentedGraph(postings)

	nodeByName := make(map[string]uint)
	nodeNames := make(map[string]bool)
	for _, n := range graph.Nodes {
		nodeByName[n.Name] = n.ID
		nodeNames[n.Name] = true
	}

	assert.True(t, nodeNames["Expenses:1:Expenses"], "root Expenses node should exist")
	assert.True(t, nodeNames["Assets:1:Assets"], "Assets root node should exist")
	assert.True(t, nodeNames["Expenses:2:Food"], "Food sub-category should exist")

	// Net cross-root: Assets→Expenses = 1000 − 500 = 500
	assetsID := nodeByName["Assets:1:Assets"]
	expensesID := nodeByName["Expenses:1:Expenses"]
	foodID := nodeByName["Expenses:2:Food"]
	restaurantID := nodeByName["Expenses:3:Restaurant"]
	for _, l := range graph.Links {
		if l.Source == assetsID && l.Target == expensesID {
			assert.Equal(t, "500", l.Value.String(), "net Assets→Expenses after refund")
		}
		// Net within-hierarchy: Expenses→Food = 1000 − 500 = 500
		if l.Source == expensesID && l.Target == foodID {
			assert.Equal(t, "500", l.Value.String(), "net Expenses→Food after refund")
		}
		if l.Source == foodID && l.Target == restaurantID {
			assert.Equal(t, "500", l.Value.String(), "net Food→Restaurant after refund")
		}
		// No backward hierarchy links should survive
		if l.Source == foodID && l.Target == expensesID {
			t.Error("backward Food→Expenses link should have been removed")
		}
		if l.Source == restaurantID && l.Target == foodID {
			t.Error("backward Restaurant→Food link should have been removed")
		}
	}
}

// TestComputeSegmentedGraph_RefundExceedsPurchase verifies that when a refund
// exceeds the purchase in the same period, backward hierarchy links are removed
// and only the cross-root effect (reduced forward flow) remains.
func TestComputeSegmentedGraph_RefundExceedsPurchase(t *testing.T) {
	postings := []posting.Posting{
		// Small purchase 300, large refund 500 → net refund 200
		txPosting("tx-buy", "Expenses:Flight:Hyd", "300"),
		txPosting("tx-buy", "Assets:Checking", "-300"),
		txPosting("tx-refund", "Expenses:Flight:Hyd", "-500"),
		txPosting("tx-refund", "Assets:Checking", "500"),
	}

	graph := computeSegmentedGraph(postings)

	nodeByName := make(map[string]uint)
	for _, n := range graph.Nodes {
		nodeByName[n.Name] = n.ID
	}

	expID := nodeByName["Expenses:1:Expenses"]
	flightID := nodeByName["Expenses:2:Flight"]
	hydID := nodeByName["Expenses:3:Hyd"]

	for _, l := range graph.Links {
		// No backward within-hierarchy links
		if l.Source == flightID && l.Target == expID {
			t.Error("backward Flight→Expenses link should be removed")
		}
		if l.Source == hydID && l.Target == flightID {
			t.Error("backward Hyd→Flight link should be removed")
		}
		// No forward within-hierarchy links either (net is negative, cross-root handles it)
		if l.Source == expID && l.Target == flightID {
			t.Error("forward Expenses→Flight link should be zero-netted away")
		}
	}

	// Net cross-root: Assets→Expenses should be negative-netted → Expenses→Assets = 200
	assetsID := nodeByName["Assets:1:Assets"]
	var expToAssets decimal.Decimal
	for _, l := range graph.Links {
		if l.Source == expID && l.Target == assetsID {
			expToAssets = l.Value
		}
	}
	assert.Equal(t, "200", expToAssets.String(), "net Expenses→Assets when refund exceeds purchase")
}

// TestComputeSegmentedGraph_RefundToCC verifies a CC refund (Expenses → Liabilities)
// is netted against the forward Liabilities→Expenses flow so Liabilities stays
// left of Expenses in the layout (no bidirectional cycle).
func TestComputeSegmentedGraph_RefundToCC(t *testing.T) {
	postings := []posting.Posting{
		// CC purchase: ₹1000 shopping charged to credit card
		txPosting("tx-buy", "Expenses:Shopping", "1000"),
		txPosting("tx-buy", "Liabilities:CreditCard", "-1000"),
		// CC refund: ₹400 refunded back to credit card
		txPosting("tx-refund", "Expenses:Shopping", "-400"),
		txPosting("tx-refund", "Liabilities:CreditCard", "400"),
	}

	graph := computeSegmentedGraph(postings)

	nodeByName := make(map[string]uint)
	for _, n := range graph.Nodes {
		nodeByName[n.Name] = n.ID
	}

	liabID := nodeByName["Liabilities:1:Liabilities"]
	expID := nodeByName["Expenses:1:Expenses"]

	// After netting: only Liabilities→Expenses (1000−400=600) should exist,
	// no Expenses→Liabilities link (which would create a cycle)
	var liabToExp, expToLiab decimal.Decimal
	for _, l := range graph.Links {
		if l.Source == liabID && l.Target == expID {
			liabToExp = l.Value
		}
		if l.Source == expID && l.Target == liabID {
			expToLiab = l.Value
		}
	}
	assert.Equal(t, "600", liabToExp.String(), "net Liabilities→Expenses after CC refund")
	assert.True(t, expToLiab.IsZero(), "Expenses→Liabilities should be zero after netting")
}

// TestComputeSegmentedGraph_NormalExpense verifies that a normal spend
// (Income → Expenses) still works correctly after the fix.
func TestComputeSegmentedGraph_NormalExpense(t *testing.T) {
	postings := []posting.Posting{
		txPosting("tx-salary", "Income:Salary", "-10000"),
		txPosting("tx-salary", "Assets:Checking", "10000"),
		txPosting("tx-food", "Assets:Checking", "-800"),
		txPosting("tx-food", "Expenses:Food:Restaurant", "800"),
	}

	graph := computeSegmentedGraph(postings)

	nodeNames := make(map[string]bool)
	for _, n := range graph.Nodes {
		nodeNames[n.Name] = true
	}

	assert.True(t, nodeNames["Income:1:Income"], "Income root node should exist")
	assert.True(t, nodeNames["Income:2:Salary"], "Income:Salary node should exist")
	assert.True(t, nodeNames["Expenses:1:Expenses"], "Expenses root node should exist")
	assert.True(t, nodeNames["Expenses:2:Food"], "Expenses:Food node should exist")
	assert.True(t, nodeNames["Expenses:3:Restaurant"], "Expenses:Food:Restaurant node should exist")
	assert.True(t, nodeNames["Assets:1:Assets"], "Assets root node should exist")
}

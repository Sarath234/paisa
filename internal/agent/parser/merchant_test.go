// internal/agent/parser/merchant_test.go
package parser_test

import (
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

var testMerchantRules = []config.MerchantRule{
	{Keyword: "swiggy", Account: "Expenses:Food:Hyd", Description: "Food Swiggy"},
	{Keyword: "zomato", Account: "Expenses:Food:Hyd", Description: "Food Zomato"},
	{Keyword: "blink commerce", Account: "Expenses:Groceries:Hyd", Description: "Groceries Blink"},
	{Keyword: "zepto", Account: "Expenses:Groceries:Hyd", Description: "Groceries ZEPTO"},
	{Keyword: "ing*flipkar", Account: "Expenses:Utils:Hyd", Description: "Utils: Flipkart"},
	{Keyword: "flipkart", Account: "Expenses:Utils:Hyd", Description: "Utils: Flipkart"},
	{Keyword: "district", Account: "Expenses:Entertainment:Hyd", Description: "Entertainment: DISTRICT"},
	{Keyword: "irctc", Account: "Expenses:Travel:Hyd", Description: "Travel"},
	{Keyword: "rail web", Account: "Expenses:Travel:Hyd", Description: "Travel"},
	{Keyword: "amazon", Account: "Expenses:Utils:Hyd", Description: "Utils: Amazon Pay"},
	{Keyword: "monthly interest", Account: "Income:Interest:IDFC6977", Description: "Bank Interest"},
}

func TestRouteMerchant(t *testing.T) {
	cases := []struct {
		merchant    string
		wantAccount string
		wantDesc    string
	}{
		{"RAZ*Swiggy/Bangalore", "Expenses:Food:Hyd", "Food Swiggy"},
		{"ZOMATO/110018759//IN", "Expenses:Food:Hyd", "Food Zomato"},
		{"BLINK COMMERCE PVT L", "Expenses:Groceries:Hyd", "Groceries Blink"},
		{"ZEPTO MARKETPLACE PRIV", "Expenses:Groceries:Hyd", "Groceries ZEPTO"},
		{"FLIPKART", "Expenses:Utils:Hyd", "Utils: Flipkart"},
		{"Ing*Flipkar", "Expenses:Utils:Hyd", "Utils: Flipkart"},
		{"DISTRICT MO", "Expenses:Entertainment:Hyd", "Entertainment: DISTRICT"},
		{"IRCTC Rail Web", "Expenses:Travel:Hyd", "Travel"},
		{"AMAZON PAY IN G", "Expenses:Utils:Hyd", "Utils: Amazon Pay"},
		{"Monthly interest", "Income:Interest:IDFC6977", "Bank Interest"},
	}
	for _, c := range cases {
		acct, desc := parser.RouteMerchant(c.merchant, testMerchantRules)
		assert.Equal(t, c.wantAccount, acct, "merchant=%q", c.merchant)
		assert.Equal(t, c.wantDesc, desc, "merchant=%q", c.merchant)
	}
}

func TestRouteMerchant_NoMatch(t *testing.T) {
	acct, desc := parser.RouteMerchant("UNKNOWN VENDOR", testMerchantRules)
	assert.Equal(t, "", acct)
	assert.Equal(t, "", desc)
}

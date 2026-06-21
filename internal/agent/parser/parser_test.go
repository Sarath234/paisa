// internal/agent/parser/parser_test.go
package parser_test

import (
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

var testAccounts = []config.AccountRule{
	// Fixed routes first — each identifier is unique enough on its own (OR match)
	{Bank: "fixed", Identifiers: []string{"****2222"}, Src: "Assets:Checking:AXIS1111", Destinations: "Liabilities:CreditCard:FK2222", Description: "CC Payment"},
	{Bank: "fixed", Identifiers: []string{"****3333"}, Src: "Assets:Checking:AXIS1111", Destinations: "Liabilities:CreditCard:MyZone3333", Description: "CC Payment"},
	{Bank: "fixed", Identifiers: []string{"****4444"}, Src: "Assets:Checking:AXIS1111", Destinations: "Liabilities:CreditCard:SELECT4444", Description: "CC Payment"},
	{Bank: "fixed", Identifiers: []string{"has been received on your ICICI Bank Credit Card XX9001"}, Src: "Assets:Checking:AXIS1111", Destinations: "Liabilities:CreditCard:ICIC9001", Description: "CC Payment"},
	{Bank: "fixed", Identifiers: []string{"TESTPAYER"}, Src: "Assets:Checking:AXIS2000", Destinations: "Assets:Checking:AXIS1111", Description: "Rent from Tenant"},
	// Format routes
	{Bank: "icici_cc", Identifiers: []string{"ICICI Bank Card XX9001"}, Destinations: "Liabilities:CreditCard:ICICI9001"},
	{Bank: "hdfc_debit", Identifiers: []string{"HDFC Bank Card 6666"}, Destinations: "Assets:Checking:FC6666"},
	{Bank: "hdfc_cc", Identifiers: []string{"HDFC Bank Card 7777"}, Destinations: "Liabilities:CreditCard:HDFC7777"},
	{Bank: "axis_checking", Identifiers: []string{"A/c no. XX1111"}, Destinations: "Assets:Checking:AXIS1111"},
	{Bank: "axis_cc", Identifiers: []string{"Card no. XX3333"}, Destinations: "Liabilities:CreditCard:MyZone3333"},
	{Bank: "axis_cc", Identifiers: []string{"Card no. XX4444"}, Destinations: "Liabilities:CreditCard:SELECT4444"},
	{Bank: "axis_cc", Identifiers: []string{"Card no. XX2222"}, Destinations: "Liabilities:CreditCard:FK2222"},
	{Bank: "idfc_checking", Identifiers: []string{"XX5555"}, Destinations: "Assets:Checking:IDFC5555"},
}

var testMerchants = []config.MerchantRule{
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
	{Keyword: "monthly interest", Account: "Income:Interest:IDFC5555", Description: "Bank Interest"},
}

type wantEntry struct{ date, desc, src, amt, dest string }

var parseTests = []struct {
	name string
	sms  string
	want wantEntry
}{
	{
		"icici_cc spend",
		"INR 350.00 spent using ICICI Bank Card XX9001 on 03-Jun-26 on AMAZON PAY IN G. Avl Limit: INR 2,00,000.00. If not you, call 1800 2662/SMS BLOCK 9001 to 9200000000",
		wantEntry{"2026/06/03", "Utils: Amazon Pay", "Liabilities:CreditCard:ICICI9001", "-350.00 INR", "Expenses:Utils:Hyd"},
	},
	{
		"icici_cc payment (fixed)",
		"Payment of Rs 5,000.00 has been received on your ICICI Bank Credit Card XX9001 through Bharat Bill Payment System on 15-MAY-26.",
		wantEntry{"2026/05/15", "CC Payment", "Liabilities:CreditCard:ICIC9001", "5000.00 INR", "Assets:Checking:AXIS1111"},
	},
	{
		"hdfc_debit swiggy",
		"Spent! INR INR 180 On HDFC Bank Card 6666 At RAZ*Swiggy/Bangalore On 03 Jun,2026 01:18 PM IST Bal INR INR 12000.00 To Block, Call 08069858585/SMS BLKGP3 Last4Digits to 5676712",
		wantEntry{"2026/06/03", "Food Swiggy", "Assets:Checking:FC6666", "-180.00 INR", "Expenses:Food:Hyd"},
	},
	{
		"hdfc_debit blink",
		"Spent! INR INR 350 On HDFC Bank Card 6666 At BLINK COMMERCE PVT L On 02 Jun,2026 08:32 PM IST Bal INR INR 12350.00 To Block, Call 08069858585",
		wantEntry{"2026/06/02", "Groceries Blink", "Assets:Checking:FC6666", "-350.00 INR", "Expenses:Groceries:Hyd"},
	},
	{
		"hdfc_debit zomato",
		"Spent! INR INR 250.00 On HDFC Bank Card 6666 At ZOMATO/110018759//IN On 31 May,2026 10:28 PM IST Bal INR INR 11750.00 To Block, Call 08069858585",
		wantEntry{"2026/05/31", "Food Zomato", "Assets:Checking:FC6666", "-250.00 INR", "Expenses:Food:Hyd"},
	},
	{
		"axis_checking irctc",
		"INR 1200.00 debited\nA/c no. XX1111\n03-06-26, 10:21:54\nUPI/P2M/012345678901/IRCTC Rail Web\nNot you? SMS BLOCKUPI Cust ID to 919900000001\nAxis Bank",
		wantEntry{"2026/06/03", "Travel", "Assets:Checking:AXIS1111", "-1200.00 INR", "Expenses:Travel:Hyd"},
	},
	{
		"testpayer fixed",
		"INR 20000.00 credited\nA/c no. XX1111\n01-06-26, 09:36:03 IST\nUPI/P2A/012345678902/TESTPAYER/UTIB/Paym - Axis Bank",
		wantEntry{"2026/06/01", "Rent from Tenant", "Assets:Checking:AXIS1111", "20000.00 INR", "Assets:Checking:AXIS2000"},
	},
	{
		"idfc interest",
		"Monthly interest of INR.200.00 earned on your Savings A/c XX5555 has been credited to your A/C on 31/05/26. New bal: INR.1,00,000.00. IDFC FIRST Bank",
		wantEntry{"2026/05/31", "Bank Interest", "Assets:Checking:IDFC5555", "200.00 INR", "Income:Interest:IDFC5555"},
	},
	{
		"idfc zepto",
		"Spent Rs.400.00 from A/C XX5555 at ZEPTO MARKETPLACE PRIV on 09/04/26. Not you? Call 180010888/SMS BLOCK (last 4 digit of card) to 5676732. IDFC FIRST Bank",
		wantEntry{"2026/04/09", "Groceries ZEPTO", "Assets:Checking:IDFC5555", "-400.00 INR", "Expenses:Groceries:Hyd"},
	},
	{
		"hdfc_cc zepto",
		"Spent Rs.280 On HDFC Bank Card 7777 At ZEPTO MARKETPLACE PRIV On 2026-05-21:07:32:56.Not You? To Block+Reissue Call 18002586161/SMS BLOCK CC 7777 to 7308080808",
		wantEntry{"2026/05/21", "Groceries ZEPTO", "Liabilities:CreditCard:HDFC7777", "-280.00 INR", "Expenses:Groceries:Hyd"},
	},
	{
		"axis_cc district (3333)",
		"Spent INR 199.00\nAxis Bank Card no. XX3333\n08-05-26 18:44:17 IST\nDISTRICT MO\nAvl Limit: INR 500000.00\nNot you? SMS BLOCK 3333 to 919900000001",
		wantEntry{"2026/05/08", "Entertainment: DISTRICT", "Liabilities:CreditCard:MyZone3333", "-199.00 INR", "Expenses:Entertainment:Hyd"},
	},
	{
		"axis_cc flipkart (4444)",
		"Spent INR 9999\nAxis Bank Card no. XX4444\n23-05-26 23:30:19 IST\nFLIPKART\nAvl Limit: INR 495000.00\nNot you? SMS BLOCK 4444 to 919900000001",
		wantEntry{"2026/05/23", "Utils: Flipkart", "Liabilities:CreditCard:SELECT4444", "-9999.00 INR", "Expenses:Utils:Hyd"},
	},
	{
		"axis_cc flipkart (2222)",
		"Spent INR 2999\nAxis Bank Card no. XX2222\n01-06-26 15:22:40 IST\nIng*Flipkar\nAvl Limit: INR 490000.00\nNot you? SMS BLOCK 2222 to 919900000001",
		wantEntry{"2026/06/01", "Utils: Flipkart", "Liabilities:CreditCard:FK2222", "-2999.00 INR", "Expenses:Utils:Hyd"},
	},
	{
		"crd-pmnt fk2222 (fixed)",
		"Debit INR 1200.00\nAxis Bank A/c XX1111\n02-06-26 10:47:13\nCRD-PMNT-100000****2222\nWhatsApp BAL to 917000000000\nNot You? SMS BLOCKALL CustID to 919900000001",
		wantEntry{"2026/06/02", "CC Payment", "Liabilities:CreditCard:FK2222", "1200.00 INR", "Assets:Checking:AXIS1111"},
	},
	{
		"crd-pmnt myzone3333 (fixed)",
		"Debit INR 1200.00\nAxis Bank A/c XX1111\n02-06-26 10:47:13\nCRD-PMNT-100000****3333\nWhatsApp BAL to 917000000000\nNot You? SMS BLOCKALL CustID to 919900000001",
		wantEntry{"2026/06/02", "CC Payment", "Liabilities:CreditCard:MyZone3333", "1200.00 INR", "Assets:Checking:AXIS1111"},
	},
	{
		"crd-pmnt select4444 (fixed)",
		"Debit INR 1200.00\nAxis Bank A/c XX1111\n02-06-26 10:47:13\nCRD-PMNT-100000****4444\nWhatsApp BAL to 917000000000\nNot You? SMS BLOCKALL CustID to 919900000001",
		wantEntry{"2026/06/02", "CC Payment", "Liabilities:CreditCard:SELECT4444", "1200.00 INR", "Assets:Checking:AXIS1111"},
	},
}

func TestParse_AllExamples(t *testing.T) {
	for _, tc := range parseTests {
		t.Run(tc.name, func(t *testing.T) {
			rule, err := parser.Classify(tc.sms, testAccounts)
			assert.NoError(t, err, "classify failed")
			entry, err := parser.Parse(tc.sms, rule, testMerchants)
			assert.NoError(t, err, "parse failed")
			assert.Equal(t, tc.want.date, entry.Date, "date")
			assert.Equal(t, tc.want.desc, entry.Desc, "desc")
			assert.Equal(t, tc.want.src, entry.Src, "src")
			assert.Equal(t, tc.want.amt, entry.Amt, "amt")
			assert.Equal(t, tc.want.dest, entry.Dest, "dest")
		})
	}
}

func TestClassify_NoMatch(t *testing.T) {
	_, err := parser.Classify("unrelated text", testAccounts)
	assert.Error(t, err)
}

func TestClassify_AxisCC_NotFixedRoute(t *testing.T) {
	// Axis CC spend SMS contains the card number (e.g. "4444") in multiple places
	// ("Card no. XX4444" and "SMS BLOCK 4444 to ..."). Must NOT match a fixed route
	// that uses "4444" as one of its identifiers — fixed route requires ALL identifiers.
	sms := "Spent INR 199.00\nAxis Bank Card no. XX4444\n08-05-26 18:44:17 IST\nRAZ*Swiggy\nAvl Limit: INR 500000.00\nNot you? SMS BLOCK 4444 to 919900000001"
	rule, err := parser.Classify(sms, testAccounts)
	assert.NoError(t, err)
	assert.Equal(t, "axis_cc", rule.Bank, "spend SMS must route to axis_cc, not fixed CC payment")
}

func TestClassify_FixedBeforeFormat(t *testing.T) {
	// CRD-PMNT + 2222 both present → fixed route wins over axis_checking (AND match)
	sms := "Debit INR 1200.00\nAxis Bank A/c XX1111\n02-06-26 10:47:13\nCRD-PMNT-100000****2222"
	rule, err := parser.Classify(sms, testAccounts)
	assert.NoError(t, err)
	assert.Equal(t, "fixed", rule.Bank)
	assert.Equal(t, "Liabilities:CreditCard:FK2222", rule.Destinations)
}

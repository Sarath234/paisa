// internal/agent/parser/parser_test.go
package parser_test

import (
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

var testAccounts = []config.AccountRule{
	// Fixed routes first
	{Bank: "fixed", Identifiers: []string{"CRD-PMNT", "8860"}, Src: "Assets:Checking:AXIS6386", Destinations: "Liabilities:CreditCard:FK8860", Description: "CC Payment"},
	{Bank: "fixed", Identifiers: []string{"CRD-PMNT", "1610"}, Src: "Assets:Checking:AXIS6386", Destinations: "Liabilities:CreditCard:MyZone1610", Description: "CC Payment"},
	{Bank: "fixed", Identifiers: []string{"CRD-PMNT", "6792"}, Src: "Assets:Checking:AXIS6386", Destinations: "Liabilities:CreditCard:SELECT6792", Description: "CC Payment"},
	{Bank: "fixed", Identifiers: []string{"has been received on your ICICI Bank Credit Card XX6009"}, Src: "Assets:Checking:AXIS6386", Destinations: "Liabilities:CreditCard:ICIC6009", Description: "CC Payment"},
	{Bank: "fixed", Identifiers: []string{"KONDAVEET"}, Src: "Assets:Checking:AXISHARITHA", Destinations: "Assets:Checking:AXIS6386", Description: "Rent from Haritha"},
	// Format routes
	{Bank: "icici_cc", Identifiers: []string{"ICICI Bank Card XX6009"}, Destinations: "Liabilities:CreditCard:ICICI6009"},
	{Bank: "hdfc_debit", Identifiers: []string{"HDFC Bank Card 2148"}, Destinations: "Assets:Checking:FC2148"},
	{Bank: "hdfc_cc", Identifiers: []string{"HDFC Bank Card 2527"}, Destinations: "Liabilities:CreditCard:HDFC2527"},
	{Bank: "axis_checking", Identifiers: []string{"XX6386"}, Destinations: "Assets:Checking:AXIS6386"},
	{Bank: "axis_cc", Identifiers: []string{"Card no. XX1610"}, Destinations: "Liabilities:CreditCard:MyZone1610"},
	{Bank: "axis_cc", Identifiers: []string{"Card no. XX6792"}, Destinations: "Liabilities:CreditCard:SELECT6792"},
	{Bank: "axis_cc", Identifiers: []string{"Card no. XX8860"}, Destinations: "Liabilities:CreditCard:FK8860"},
	{Bank: "idfc_checking", Identifiers: []string{"XX6977"}, Destinations: "Assets:Checking:IDFC6977"},
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
	{Keyword: "monthly interest", Account: "Income:Interest:IDFC6977", Description: "Bank Interest"},
}

type wantEntry struct{ date, desc, src, amt, dest string }

var parseTests = []struct {
	name string
	sms  string
	want wantEntry
}{
	{
		"icici_cc spend",
		"INR 453.00 spent using ICICI Bank Card XX6009 on 03-Jun-26 on AMAZON PAY IN G. Avl Limit: INR 4,59,509.36. If not you, call 1800 2662/SMS BLOCK 6009 to 9215676766",
		wantEntry{"2026/06/03", "Utils: Amazon Pay", "Liabilities:CreditCard:ICICI6009", "-453.00 INR", "Expenses:Utils:Hyd"},
	},
	{
		"icici_cc payment (fixed)",
		"Payment of Rs 10,468.87 has been received on your ICICI Bank Credit Card XX6009 through Bharat Bill Payment System on 15-MAY-26.",
		wantEntry{"2026/05/15", "CC Payment", "Liabilities:CreditCard:ICIC6009", "10468.87 INR", "Assets:Checking:AXIS6386"},
	},
	{
		"hdfc_debit swiggy",
		"Spent! INR INR 215 On HDFC Bank Card 2148 At RAZ*Swiggy/Bangalore On 03 Jun,2026 01:18 PM IST Bal INR INR 23216.37 To Block, Call 08069858585/SMS BLKGP3 Last4Digits to 5676712",
		wantEntry{"2026/06/03", "Food Swiggy", "Assets:Checking:FC2148", "-215.00 INR", "Expenses:Food:Hyd"},
	},
	{
		"hdfc_debit blink",
		"Spent! INR INR 426 On HDFC Bank Card 2148 At BLINK COMMERCE PVT L On 02 Jun,2026 08:32 PM IST Bal INR INR 23431.37 To Block, Call 08069858585",
		wantEntry{"2026/06/02", "Groceries Blink", "Assets:Checking:FC2148", "-426.00 INR", "Expenses:Groceries:Hyd"},
	},
	{
		"hdfc_debit zomato",
		"Spent! INR INR 327.25 On HDFC Bank Card 2148 At ZOMATO/110018759//IN On 31 May,2026 10:28 PM IST Bal INR INR 9534.37 To Block, Call 08069858585",
		wantEntry{"2026/05/31", "Food Zomato", "Assets:Checking:FC2148", "-327.25 INR", "Expenses:Food:Hyd"},
	},
	{
		"axis_checking irctc",
		"INR 1804.05 debited\nA/c no. XX6386\n03-06-26, 10:21:54\nUPI/P2M/102154212206/IRCTC Rail Web\nNot you? SMS BLOCKUPI Cust ID to 919951860002\nAxis Bank",
		wantEntry{"2026/06/03", "Travel", "Assets:Checking:AXIS6386", "-1804.05 INR", "Expenses:Travel:Hyd"},
	},
	{
		"kondaveet fixed",
		"INR 30000.00 credited\nA/c no. XX6386\n01-06-26, 09:36:03 IST\nUPI/P2A/067278619954/KONDAVEET/UTIB/Paym - Axis Bank",
		wantEntry{"2026/06/01", "Rent from Haritha", "Assets:Checking:AXIS6386", "30000.00 INR", "Assets:Checking:AXISHARITHA"},
	},
	{
		"idfc interest",
		"Monthly interest of INR.318.00 earned on your Savings A/c XX6977 has been credited to your A/C on 31/05/26. New bal: INR.1,50,101.05. IDFC FIRST Bank",
		wantEntry{"2026/05/31", "Bank Interest", "Assets:Checking:IDFC6977", "318.00 INR", "Income:Interest:IDFC6977"},
	},
	{
		"idfc zepto",
		"Spent Rs.473.00 from A/C XX6977 at ZEPTO MARKETPLACE PRIV on 09/04/26. Not you? Call 180010888/SMS BLOCK (last 4 digit of card) to 5676732. IDFC FIRST Bank",
		wantEntry{"2026/04/09", "Groceries ZEPTO", "Assets:Checking:IDFC6977", "-473.00 INR", "Expenses:Groceries:Hyd"},
	},
	{
		"hdfc_cc zepto",
		"Spent Rs.341 On HDFC Bank Card 2527 At ZEPTO MARKETPLACE PRIV On 2026-05-21:07:32:56.Not You? To Block+Reissue Call 18002586161/SMS BLOCK CC 2527 to 7308080808",
		wantEntry{"2026/05/21", "Groceries ZEPTO", "Liabilities:CreditCard:HDFC2527", "-341.00 INR", "Expenses:Groceries:Hyd"},
	},
	{
		"axis_cc district (1610)",
		"Spent INR 210.12\nAxis Bank Card no. XX1610\n08-05-26 18:44:17 IST\nDISTRICT MO\nAvl Limit: INR 1389000.78\nNot you? SMS BLOCK 1610 to 919951860002",
		wantEntry{"2026/05/08", "Entertainment: DISTRICT", "Liabilities:CreditCard:MyZone1610", "-210.12 INR", "Expenses:Entertainment:Hyd"},
	},
	{
		"axis_cc flipkart (6792)",
		"Spent INR 11864\nAxis Bank Card no. XX6792\n23-05-26 23:30:19 IST\nFLIPKART\nAvl Limit: INR 1324182.46\nNot you? SMS BLOCK 6792 to 919951860002",
		wantEntry{"2026/05/23", "Utils: Flipkart", "Liabilities:CreditCard:SELECT6792", "-11864.00 INR", "Expenses:Utils:Hyd"},
	},
	{
		"axis_cc flipkart (8860)",
		"Spent INR 3468\nAxis Bank Card no. XX8860\n01-06-26 15:22:40 IST\nIng*Flipkar\nAvl Limit: INR 1320714.46\nNot you? SMS BLOCK 8860 to 919951860002",
		wantEntry{"2026/06/01", "Utils: Flipkart", "Liabilities:CreditCard:FK8860", "-3468.00 INR", "Expenses:Utils:Hyd"},
	},
	{
		"crd-pmnt fk8860 (fixed)",
		"Debit INR 1417.00\nAxis Bank A/c XX6386\n02-06-26 10:47:13\nCRD-PMNT-533467****8860\nWhatsApp BAL to 917036165000\nNot You? SMS BLOCKALL CustID to 919951860002",
		wantEntry{"2026/06/02", "CC Payment", "Liabilities:CreditCard:FK8860", "1417.00 INR", "Assets:Checking:AXIS6386"},
	},
	{
		"crd-pmnt myzone1610 (fixed)",
		"Debit INR 1417.00\nAxis Bank A/c XX6386\n02-06-26 10:47:13\nCRD-PMNT-530562****1610\nWhatsApp BAL to 917036165000\nNot You? SMS BLOCKALL CustID to 919951860002",
		wantEntry{"2026/06/02", "CC Payment", "Liabilities:CreditCard:MyZone1610", "1417.00 INR", "Assets:Checking:AXIS6386"},
	},
	{
		"crd-pmnt select6792 (fixed)",
		"Debit INR 1417.00\nAxis Bank A/c XX6386\n02-06-26 10:47:13\nCRD-PMNT-411146****6792\nWhatsApp BAL to 917036165000\nNot You? SMS BLOCKALL CustID to 919951860002",
		wantEntry{"2026/06/02", "CC Payment", "Liabilities:CreditCard:SELECT6792", "1417.00 INR", "Assets:Checking:AXIS6386"},
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

func TestClassify_FixedBeforeFormat(t *testing.T) {
	// CRD-PMNT + 8860 matches fixed route, not axis_checking
	sms := "Debit INR 1417.00\nAxis Bank A/c XX6386\n02-06-26 10:47:13\nCRD-PMNT-533467****8860"
	rule, err := parser.Classify(sms, testAccounts)
	assert.NoError(t, err)
	assert.Equal(t, "fixed", rule.Bank)
	assert.Equal(t, "Liabilities:CreditCard:FK8860", rule.Destinations)
}

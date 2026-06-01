package parser

import (
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/stretchr/testify/assert"
)

var testRules = config.ParserRules{
	DayParts: config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20},
	Merchants: []config.MerchantPattern{
		{Keyword: "swiggy", Description: "Food: {day_part}", Account: "Expenses:Food:Hyd:Swiggy"},
		{Keyword: "zomato", Description: "Food: {day_part}", Account: "Expenses:Food:Hyd:Zomato"},
		{Keyword: "zepto", Description: "Groceries", Account: "Expenses:Groceries:Hyd"},
		{Keyword: "", Description: "Misc", Account: "Expenses:Misc:Hyd"},
	},
	Sources: []config.SourceRule{
		{ID: "fi_upi", Contains: []string{"debited via UPI on"}, Account: "Assets:Checking:FI5687"},
		{ID: "axis", Contains: []string{"A/c no. XX6386"}, Account: "Assets:Checking:AXIS6386"},
		{ID: "cc_fk", Contains: []string{"Card no. XX8860"}, Account: "Liabilities:CreditCard:FK8860"},
		{
			ID:          "canara_loan",
			Contains:    []string{"XXXXX21343", "Canara Bank", "Loan Drawdown"},
			Account:     "Assets:Checking:CANA1343",
			DestAccount: "Liabilities:Loan:CANAHL1090",
			Description: "EMI: Home Loan",
		},
	},
}

// --- parseDate ---

func TestParseDate_Pattern1_DMYYYY(t *testing.T) {
	date, ok := parseDate("INR 100 debited via UPI on 14-05-2025 at Swiggy")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_Pattern2_DMYY(t *testing.T) {
	date, ok := parseDate("Card no. XX8860 INR 500.00 on 14-05-25")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_Pattern3_MonNameDY(t *testing.T) {
	date, ok := parseDate("on May 14, 2025 amount INR 100")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_Pattern4_DDMONYY(t *testing.T) {
	date, ok := parseDate("A/c no. XX6386 debited Rs.500 on 14MAY25")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_Pattern5_DMonY(t *testing.T) {
	date, ok := parseDate("on 14 MAY,2025 Rs 100 debited")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_Pattern6_SlashDMYYYY(t *testing.T) {
	date, ok := parseDate("XXXXX21343 Canara Bank Loan Drawdown INR 45000 on 01/05/2025")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-01", date)
}

func TestParseDate_Pattern7_FullMonthDY(t *testing.T) {
	date, ok := parseDate("Debited May 14, 2025 INR 100")
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", date)
}

func TestParseDate_NoMatch(t *testing.T) {
	_, ok := parseDate("no date in this message")
	assert.False(t, ok)
}

// --- parseAmount ---

func TestParseAmount_RsWithComma(t *testing.T) {
	amt, ok := parseAmount("Rs 2,450.00 debited via UPI")
	assert.True(t, ok)
	assert.Equal(t, 2450.0, amt)
}

func TestParseAmount_RsDot(t *testing.T) {
	amt, ok := parseAmount("Rs.500.00 debited")
	assert.True(t, ok)
	assert.Equal(t, 500.0, amt)
}

func TestParseAmount_INRWithComma(t *testing.T) {
	amt, ok := parseAmount("INR 1,200.50 spent on card")
	assert.True(t, ok)
	assert.Equal(t, 1200.50, amt)
}

func TestParseAmount_INRNoDecimal(t *testing.T) {
	amt, ok := parseAmount("INR 45000 on 01/05/2025")
	assert.True(t, ok)
	assert.Equal(t, 45000.0, amt)
}

func TestParseAmount_NoMatch(t *testing.T) {
	_, ok := parseAmount("no amount here")
	assert.False(t, ok)
}

// --- parseDayPart ---

func TestParseDayPart_Breakfast(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Breakfast", parseDayPart("transaction at 10:59:00", dp))
}

func TestParseDayPart_LunchBoundaryLow(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Lunch", parseDayPart("transaction at 11:00", dp))
}

func TestParseDayPart_LunchBoundaryHigh(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Lunch", parseDayPart("transaction at 14:59", dp))
}

func TestParseDayPart_DinnerBoundaryLow(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Dinner", parseDayPart("transaction at 15:00", dp))
}

func TestParseDayPart_DinnerBoundaryHigh(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Dinner", parseDayPart("transaction at 19:59", dp))
}

func TestParseDayPart_EveningSnack(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Evening Snack", parseDayPart("transaction at 20:00", dp))
}

func TestParseDayPart_NoTime(t *testing.T) {
	dp := config.DayPartsConfig{BreakfastEnd: 11, LunchEnd: 15, DinnerEnd: 20}
	assert.Equal(t, "Meal", parseDayPart("no timestamp in message", dp))
}

// --- matchMerchant ---

func TestMatchMerchant_Swiggy_DayPart(t *testing.T) {
	desc, acct := matchMerchant("debited at SWIGGY 13:30", testRules.Merchants, "Lunch")
	assert.Equal(t, "Food: Lunch", desc)
	assert.Equal(t, "Expenses:Food:Hyd:Swiggy", acct)
}

func TestMatchMerchant_CaseInsensitive(t *testing.T) {
	desc, _ := matchMerchant("ZOMATO ORDER", testRules.Merchants, "Dinner")
	assert.Equal(t, "Food: Dinner", desc)
}

func TestMatchMerchant_Fallback(t *testing.T) {
	desc, acct := matchMerchant("debited at Unknown Store 13:00", testRules.Merchants, "Lunch")
	assert.Equal(t, "Misc", desc)
	assert.Equal(t, "Expenses:Misc:Hyd", acct)
}

// --- matchSource ---

func TestMatchSource_SingleContains(t *testing.T) {
	src, ok := matchSource("INR 100 debited via UPI on 14-05-2025", testRules.Sources)
	assert.True(t, ok)
	assert.Equal(t, "fi_upi", src.ID)
}

func TestMatchSource_MultiContainsAllMatch(t *testing.T) {
	src, ok := matchSource("XXXXX21343 Canara Bank Loan Drawdown INR 45000 on 01/05/2025", testRules.Sources)
	assert.True(t, ok)
	assert.Equal(t, "canara_loan", src.ID)
}

func TestMatchSource_MultiContainsPartialMiss(t *testing.T) {
	_, ok := matchSource("XXXXX21343 Canara Bank credit INR 45000", testRules.Sources)
	assert.False(t, ok)
}

func TestMatchSource_NoMatch(t *testing.T) {
	_, ok := matchSource("some unrecognised bank message", testRules.Sources)
	assert.False(t, ok)
}

// --- RegexParse integration ---

func TestRegexParse_FiUPI_Swiggy_Lunch(t *testing.T) {
	msg := "INR 2,450.00 debited via UPI on 14-05-2025 13:30 at SWIGGY Ref 12345"
	tx, ok := RegexParse(msg, testRules)
	assert.True(t, ok)
	assert.Equal(t, "2025-05-14", tx.Date)
	assert.Equal(t, -2450.0, tx.Amount)
	assert.Equal(t, "INR", tx.Currency)
	assert.Equal(t, "Food: Lunch", tx.Merchant)
	assert.Equal(t, "Expenses:Food:Hyd:Swiggy", tx.SuggestedLedgerAccount)
	assert.Equal(t, "Assets:Checking:FI5687", tx.SourceAccount)
	assert.Equal(t, 1.0, tx.Confidence)
	assert.Equal(t, "debit", tx.TxType)
}

func TestRegexParse_CanaraLoan_DestOverride(t *testing.T) {
	msg := "XXXXX21343 Canara Bank Loan Drawdown INR 45,000.00 on 01/05/2025"
	tx, ok := RegexParse(msg, testRules)
	assert.True(t, ok)
	assert.Equal(t, "2025-05-01", tx.Date)
	assert.Equal(t, "EMI: Home Loan", tx.Merchant)
	assert.Equal(t, "Liabilities:Loan:CANAHL1090", tx.SuggestedLedgerAccount)
	assert.Equal(t, "Assets:Checking:CANA1343", tx.SourceAccount)
}

func TestRegexParse_NoSourceMatch(t *testing.T) {
	_, ok := RegexParse("some random text with no bank markers", testRules)
	assert.False(t, ok)
}

func TestRegexParse_SourceMatchButNoDate(t *testing.T) {
	_, ok := RegexParse("INR 100 debited via UPI on no-date-here at Swiggy", testRules)
	assert.False(t, ok)
}

func TestRegexParse_SourceMatchButNoAmount(t *testing.T) {
	_, ok := RegexParse("debited via UPI on 14-05-2025 at Swiggy no amount present", testRules)
	assert.False(t, ok)
}

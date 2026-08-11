// internal/agent/parser/banks_test.go
package parser_test

import (
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

func TestExtractIciciCC(t *testing.T) {
	sms := "INR 453.00 spent using ICICI Bank Card XX6009 on 03-Jun-26 on AMAZON PAY IN G. Avl Limit: INR 4,59,509.36. If not you, call 1800 2662/SMS BLOCK 6009 to 9215676766"
	m, d, a, debit, err := parser.ExtractIciciCC(sms)
	assert.NoError(t, err)
	assert.Equal(t, "AMAZON PAY IN G", m)
	assert.Equal(t, "03-Jun-26", d)
	assert.Equal(t, "453.00", a)
	assert.True(t, debit)
}

func TestExtractHdfcDebit(t *testing.T) {
	cases := []struct {
		sms      string
		merchant string
		date     string
		amt      string
	}{
		{
			"Spent! INR INR 215 On HDFC Bank Card 2148 At RAZ*Swiggy/Bangalore On 03 Jun,2026 01:18 PM IST Bal INR INR 23216.37 To Block, Call 08069858585",
			"RAZ*Swiggy/Bangalore", "03 Jun,2026", "215",
		},
		{
			"Spent! INR INR 426 On HDFC Bank Card 2148 At BLINK COMMERCE PVT L On 02 Jun,2026 08:32 PM IST Bal INR INR 23431.37 To Block, Call 08069858585",
			"BLINK COMMERCE PVT L", "02 Jun,2026", "426",
		},
		{
			"Spent! INR INR 327.25 On HDFC Bank Card 2148 At ZOMATO/110018759//IN On 31 May,2026 10:28 PM IST Bal INR INR 9534.37 To Block, Call 08069858585",
			"ZOMATO/110018759//IN", "31 May,2026", "327.25",
		},
	}
	for _, c := range cases {
		m, d, a, debit, err := parser.ExtractHdfcDebit(c.sms)
		assert.NoError(t, err)
		assert.Equal(t, c.merchant, m)
		assert.Equal(t, c.date, d)
		assert.Equal(t, c.amt, a)
		assert.True(t, debit)
	}
}

func TestExtractHdfcCC(t *testing.T) {
	sms := "Spent Rs.341 On HDFC Bank Card 2527 At ZEPTO MARKETPLACE PRIV On 2026-05-21:07:32:56.Not You? To Block+Reissue Call 18002586161"
	m, d, a, debit, err := parser.ExtractHdfcCC(sms)
	assert.NoError(t, err)
	assert.Equal(t, "ZEPTO MARKETPLACE PRIV", m)
	assert.Equal(t, "2026-05-21", d)
	assert.Equal(t, "341", a)
	assert.True(t, debit)
}

func TestExtractAxisChecking(t *testing.T) {
	t.Run("format_a_upi", func(t *testing.T) {
		sms := "INR 1804.05 debited\nA/c no. XX6386\n03-06-26, 10:21:54\nUPI/P2M/102154212206/IRCTC Rail Web\nNot you? SMS BLOCKUPI Cust ID to 919951860002\nAxis Bank"
		m, d, a, debit, err := parser.ExtractAxisChecking(sms)
		assert.NoError(t, err)
		assert.Equal(t, "IRCTC Rail Web", m)
		assert.Equal(t, "03-06-26", d)
		assert.Equal(t, "1804.05", a)
		assert.True(t, debit)
	})
	t.Run("format_b_ach_dr", func(t *testing.T) {
		sms := "Debit INR 2000.00\nAxis Bank A/c XX1111\n22-06-26 08:41:18\nACH-DR-BD-MF Utilities Lum\nWhatsApp BAL to 917036165000\nNot You? SMS BLOCKALL CustID to 919951860002"
		m, d, a, debit, err := parser.ExtractAxisChecking(sms)
		assert.NoError(t, err)
		assert.Equal(t, "ACH-DR-BD-MF Utilities Lum", m)
		assert.Equal(t, "22-06-26", d)
		assert.Equal(t, "2000.00", a)
		assert.True(t, debit)
	})
	t.Run("format_a_debited_from_on_merchant", func(t *testing.T) {
		sms := "INR 378.78 debited from A/c no. XX116386 on BOOKMYSHOW  24-07-2026 23:14:20 IST. Avl bal: INR 715912.87. Not you? SMS BLOCKCARD XX5851 to +919951860002 - Axis Bank"
		m, d, a, debit, err := parser.ExtractAxisChecking(sms)
		assert.NoError(t, err)
		assert.Equal(t, "BOOKMYSHOW", m)
		assert.Equal(t, "24-07-2026", d)
		assert.Equal(t, "378.78", a)
		assert.True(t, debit)
	})
	// Format C — email notices: verb precedes the amount, counterparty follows "by".
	t.Run("format_c_credited_with_by_party", func(t *testing.T) {
		cases := []struct {
			sms      string
			merchant string
			date     string
			amt      string
		}{
			{
				"We wish to inform you that your A/c no. XX6386 has been credited with INR 10.00 on 10-08-2026 at 14:46:40 IST by ACH-CR-WIPRO LIMITED-NACH-.",
				"ACH-CR-WIPRO LIMITED-NACH-", "10-08-2026", "10.00",
			},
			{
				"We wish to inform you that your A/c no. XX6386 has been credited with INR 1.00 on 10-08-2026 at 14:38:37 IST by ACH-CR-ITCHOTELSLTD-NACH-3.",
				"ACH-CR-ITCHOTELSLTD-NACH-3", "10-08-2026", "1.00",
			},
			{
				"We wish to inform you that your A/c no. XX6386 has been credited with INR 6.00 on 11-08-2026 at 09:03:18 IST by MAX HEALTHCARE /.",
				"MAX HEALTHCARE /", "11-08-2026", "6.00",
			},
		}
		for _, c := range cases {
			m, d, a, debit, err := parser.ExtractAxisChecking(c.sms)
			assert.NoError(t, err)
			assert.Equal(t, c.merchant, m)
			assert.Equal(t, c.date, d)
			assert.Equal(t, c.amt, a)
			assert.False(t, debit)
		}
	})
	t.Run("format_c_debited_with", func(t *testing.T) {
		sms := "We wish to inform you that your A/c no. XX6386 has been debited with INR 1,250.50 on 09-08-2026 at 10:12:00 IST by ACH-DR-BD-MF Utilities."
		m, d, a, debit, err := parser.ExtractAxisChecking(sms)
		assert.NoError(t, err)
		assert.Equal(t, "ACH-DR-BD-MF Utilities", m)
		assert.Equal(t, "09-08-2026", d)
		assert.Equal(t, "1,250.50", a)
		assert.True(t, debit)
	})
	// Format D — labelled email block, values on the line after each label.
	t.Run("format_d_labelled_block", func(t *testing.T) {
		cases := []struct {
			sms      string
			merchant string
			date     string
			amt      string
		}{
			{
				"    \nAmount Debited:\nINR 250.00\n    \nAccount Number:\nXX6386\n    \nDate & Time:\n10-08-26, 08:58:34 IST\n    \nTransaction Info:\nUPI/P2A/085838940420/DHARMAVARAPU GANESH",
				"DHARMAVARAPU GANESH", "10-08-26", "250.00",
			},
			{
				"    \nAmount Debited:\nINR 10.00\n    \nAccount Number:\nXX6386\n    \nDate & Time:\n09-08-26, 19:22:28 IST\n    \nTransaction Info:\nUPI/P2M/622155323152/MANJULA N",
				"MANJULA N", "09-08-26", "10.00",
			},
		}
		for _, c := range cases {
			m, d, a, debit, err := parser.ExtractAxisChecking(c.sms)
			assert.NoError(t, err)
			assert.Equal(t, c.merchant, m)
			assert.Equal(t, c.date, d)
			assert.Equal(t, c.amt, a)
			assert.True(t, debit)
		}
	})
	t.Run("format_d_credited", func(t *testing.T) {
		sms := "Amount Credited:\nINR 500.00\n\nAccount Number:\nXX6386\n\nDate & Time:\n09-08-26, 19:22:28 IST\n\nTransaction Info:\nUPI/P2A/622155323152/SOMEONE ELSE"
		_, _, a, debit, err := parser.ExtractAxisChecking(sms)
		assert.NoError(t, err)
		assert.Equal(t, "500.00", a)
		assert.False(t, debit)
	})
}

func TestExtractAxisUPI(t *testing.T) {
	sms := "Your A/c has been debited towards NETFLIX for INR 649.00 on 22-06-26. 7e93d89fa7ba415588a8175198857341@okaxis - Axis Bank"
	m, d, a, debit, err := parser.ExtractAxisUPI(sms)
	assert.NoError(t, err)
	assert.Equal(t, "NETFLIX", m)
	assert.Equal(t, "22-06-26", d)
	assert.Equal(t, "649.00", a)
	assert.True(t, debit)
}

func TestExtractAxisCC(t *testing.T) {
	cases := []struct {
		sms      string
		merchant string
		date     string
		amt      string
	}{
		{
			"Spent INR 210.12\nAxis Bank Card no. XX1610\n08-05-26 18:44:17 IST\nDISTRICT MO\nAvl Limit: INR 1389000.78\nNot you? SMS BLOCK 1610 to 919951860002",
			"DISTRICT MO", "08-05-26", "210.12",
		},
		{
			"Spent INR 11864\nAxis Bank Card no. XX6792\n23-05-26 23:30:19 IST\nFLIPKART\nAvl Limit: INR 1324182.46\nNot you? SMS BLOCK 6792 to 919951860002",
			"FLIPKART", "23-05-26", "11864",
		},
		{
			"Spent INR 3468\nAxis Bank Card no. XX8860\n01-06-26 15:22:40 IST\nIng*Flipkar\nAvl Limit: INR 1320714.46\nNot you? SMS BLOCK 8860 to 919951860002",
			"Ing*Flipkar", "01-06-26", "3468",
		},
	}
	for _, c := range cases {
		m, d, a, debit, err := parser.ExtractAxisCC(c.sms)
		assert.NoError(t, err)
		assert.Equal(t, c.merchant, m)
		assert.Equal(t, c.date, d)
		assert.Equal(t, c.amt, a)
		assert.True(t, debit)
	}
}

func TestExtractIDFCChecking(t *testing.T) {
	t.Run("spend", func(t *testing.T) {
		sms := "Spent Rs.473.00 from A/C XX6977 at ZEPTO MARKETPLACE PRIV on 09/04/26. Not you? Call 180010888/SMS BLOCK (last 4 digit of card) to 5676732. IDFC FIRST Bank"
		m, d, a, debit, err := parser.ExtractIDFCChecking(sms)
		assert.NoError(t, err)
		assert.Equal(t, "ZEPTO MARKETPLACE PRIV", m)
		assert.Equal(t, "09/04/26", d)
		assert.Equal(t, "473.00", a)
		assert.True(t, debit)
	})
	t.Run("interest", func(t *testing.T) {
		sms := "Monthly interest of INR.318.00 earned on your Savings A/c XX6977 has been credited to your A/C on 31/05/26. New bal: INR.1,50,101.05. IDFC FIRST Bank"
		m, d, a, debit, err := parser.ExtractIDFCChecking(sms)
		assert.NoError(t, err)
		assert.Equal(t, "Monthly interest", m)
		assert.Equal(t, "31/05/26", d)
		assert.Equal(t, "318.00", a)
		assert.False(t, debit)
	})
}

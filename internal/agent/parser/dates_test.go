// internal/agent/parser/dates_test.go
package parser_test

import (
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

func TestNormaliseDate(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"03-Jun-26", "2026/06/03"},   // DD-Mon-YY
		{"15-MAY-26", "2026/05/15"},   // DD-MON-YY uppercase
		{"03 Jun,2026", "2026/06/03"}, // DD Mon,YYYY
		{"31 May,2026", "2026/05/31"}, // DD Mon,YYYY
		{"2026-05-21", "2026/05/21"},  // YYYY-MM-DD
		{"03-06-26", "2026/06/03"},    // DD-MM-YY
		{"24-07-2026", "2026/07/24"},  // DD-MM-YYYY
		{"08/06/2026", "2026/06/08"},  // DD/MM/YYYY
		{"09/04/26", "2026/04/09"},    // DD/MM/YY
		{"31/05/26", "2026/05/31"},    // DD/MM/YY
	}
	for _, c := range cases {
		got, err := parser.NormaliseDate(c.raw)
		assert.NoError(t, err, "input: %q", c.raw)
		assert.Equal(t, c.want, got, "input: %q", c.raw)
	}
}

func TestNormaliseDate_Unknown(t *testing.T) {
	_, err := parser.NormaliseDate("not-a-date")
	assert.Error(t, err)
}

func TestExtractDateFromSMS(t *testing.T) {
	cases := []struct {
		sms  string
		want string
	}{
		{"INR 453.00 spent using ICICI Bank Card XX6009 on 03-Jun-26 on AMAZON PAY IN G.", "03-Jun-26"},
		{"Payment of Rs 10,468.87 has been received ... on 15-MAY-26.", "15-MAY-26"},
		{"Spent! INR INR 215 On HDFC Bank Card 2148 At RAZ*Swiggy On 03 Jun,2026 01:18 PM IST", "03 Jun,2026"},
		{"Spent Rs.341 On HDFC Bank Card 2527 At ZEPTO On 2026-05-21:07:32:56.", "2026-05-21"},
		{"INR 1804.05 debited\nA/c no. XX6386\n03-06-26, 10:21:54\nUPI/P2M/...", "03-06-26"},
		{"Spent Rs.473.00 from A/C XX6977 at ZEPTO on 09/04/26.", "09/04/26"},
		{"An amount of INR 577.00 has been DEBITED to your account XXXXX21343 on 08/06/2026.", "08/06/2026"},
		{"INR 378.78 debited from A/c no. XX116386 on BOOKMYSHOW  24-07-2026 23:14:20 IST.", "24-07-2026"},
	}
	for _, c := range cases {
		got, err := parser.ExtractDateFromSMS(c.sms)
		assert.NoError(t, err, "sms: %q", c.sms)
		assert.Equal(t, c.want, got, "sms snippet: %q", c.sms[:30])
	}
}

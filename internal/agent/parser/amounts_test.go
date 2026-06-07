// internal/agent/parser/amounts_test.go
package parser_test

import (
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

func TestNormaliseAmount(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"453.00", "453.00"},
		{"10,468.87", "10468.87"},
		{"215", "215.00"},
		{"11864", "11864.00"},
		{"318.00", "318.00"},
		{"1,50,101.05", "150101.05"},
		{"341", "341.00"},
	}
	for _, c := range cases {
		got, err := parser.NormaliseAmount(c.raw)
		assert.NoError(t, err)
		assert.Equal(t, c.want, got, "raw=%q", c.raw)
	}
}

func TestFormatEntryAmount(t *testing.T) {
	assert.Equal(t, "-215.00 INR", parser.FormatEntryAmount("215.00", true))
	assert.Equal(t, "30000.00 INR", parser.FormatEntryAmount("30000.00", false))
}

func TestExtractAmountFromSMS(t *testing.T) {
	cases := []struct {
		sms     string
		wantAmt string
		wantDeb bool
	}{
		{"Payment of Rs 10,468.87 has been received on your ICICI Bank Credit Card XX6009", "10468.87", false},
		{"Debit INR 1417.00\nAxis Bank A/c XX6386\n02-06-26", "1417.00", true},
		{"INR 30000.00 credited\nA/c no. XX6386", "30000.00", false},
		{"Monthly interest of INR.318.00 earned on your Savings A/c XX6977", "318.00", false},
	}
	for _, c := range cases {
		amt, isDebit, err := parser.ExtractAmountFromSMS(c.sms)
		prefix := c.sms
		if len(prefix) > 40 {
			prefix = prefix[:40]
		}
		assert.NoError(t, err, "sms: %q", prefix)
		assert.Equal(t, c.wantAmt, amt, "sms: %q", prefix)
		assert.Equal(t, c.wantDeb, isDebit, "sms: %q", prefix)
	}
}

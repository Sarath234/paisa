// internal/agent/qa/format_test.go
package qa

import "testing"

func TestFormatINR(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "₹0"},
		{215.5, "₹215.50"},
		{4520, "₹4,520"},
		{123456, "₹1,23,456"},
		{1234567.89, "₹12,34,567.89"},
		{-980, "-₹980"},
		{500000, "₹5,00,000"},
		{1000000, "₹10,00,000"},
	}
	for _, c := range cases {
		if got := FormatINR(c.in); got != c.want {
			t.Errorf("FormatINR(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

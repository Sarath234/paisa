// internal/agent/qa/format.go
package qa

import (
	"strconv"
	"strings"
)

// FormatINR renders an amount with Indian digit grouping (₹12,34,567.89).
// Whole amounts drop the ".00".
func FormatINR(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatFloat(v, 'f', 2, 64)
	parts := strings.SplitN(s, ".", 2)
	intPart, frac := parts[0], parts[1]

	grouped := intPart
	if len(intPart) > 3 {
		last3 := intPart[len(intPart)-3:]
		rest := intPart[:len(intPart)-3]
		var groups []string
		for len(rest) > 2 {
			groups = append([]string{rest[len(rest)-2:]}, groups...)
			rest = rest[:len(rest)-2]
		}
		if rest != "" {
			groups = append([]string{rest}, groups...)
		}
		grouped = strings.Join(append(groups, last3), ",")
	}

	out := "₹" + grouped
	if frac != "00" {
		out += "." + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

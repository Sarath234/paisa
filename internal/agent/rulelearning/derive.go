// internal/agent/rulelearning/derive.go
package rulelearning

import (
	"strings"

	agentledger "github.com/ananthakumaran/paisa/internal/agent/ledger"
)

// Derive computes a merchant rule from a corrected entry.
// Returns ok=false if dest is unchanged or original.Desc is empty.
func Derive(original, corrected agentledger.Entry) (keyword, account, description string, ok bool) {
	if original.Dest == corrected.Dest {
		return "", "", "", false
	}
	keyword = strings.ToLower(strings.TrimSpace(original.Desc))
	if keyword == "" {
		return "", "", "", false
	}
	return keyword, corrected.Dest, corrected.Desc, true
}

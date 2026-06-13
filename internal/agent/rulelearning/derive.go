// internal/agent/rulelearning/derive.go
package rulelearning

import (
	"strings"

	agentledger "github.com/ananthakumaran/paisa/internal/agent/ledger"
)

// Derive computes a merchant rule from a corrected entry.
// Returns ok=false if dest is unchanged or original.Desc is empty.
// keyword is lowercased original.Desc (the parser/LLM-filled journal description),
// which may be a phrase like "food swiggy" rather than a short token like "swiggy".
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

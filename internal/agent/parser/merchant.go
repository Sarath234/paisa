// internal/agent/parser/merchant.go
package parser

import (
	"strings"

	"github.com/ananthakumaran/paisa/internal/agent/config"
)

// RouteMerchant finds the first MerchantRule whose keyword is a case-insensitive
// substring of merchant. Returns the account and description, or empty strings if
// no rule matches (caller should try LLM fallback).
func RouteMerchant(merchant string, rules []config.MerchantRule) (account, description string) {
	lower := strings.ToLower(merchant)
	for _, r := range rules {
		if strings.Contains(lower, strings.ToLower(r.Keyword)) {
			return r.Account, r.Description
		}
	}
	return "", ""
}

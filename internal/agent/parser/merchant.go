// internal/agent/parser/merchant.go
package parser

import (
	"strings"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	log "github.com/sirupsen/logrus"
)

// RouteMerchant finds the first MerchantRule whose keyword is a case-insensitive
// substring of merchant. Returns the account and description, or empty strings if
// no rule matches (caller should try LLM fallback).
func RouteMerchant(merchant string, rules []config.MerchantRule) (account, description string) {
	lower := strings.ToLower(merchant)
	log.Debugf("merchant: routing %q against %d rules", merchant, len(rules))
	for _, r := range rules {
		if strings.Contains(lower, strings.ToLower(r.Keyword)) {
			log.Debugf("merchant: matched keyword=%q → account=%q desc=%q", r.Keyword, r.Account, r.Description)
			return r.Account, r.Description
		}
	}
	log.Debugf("merchant: no rule matched for %q — dest+desc will be empty", merchant)
	return "", ""
}

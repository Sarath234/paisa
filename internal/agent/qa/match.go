// internal/agent/qa/match.go
package qa

import "strings"

// MatchAccounts fuzzy-matches a user phrase against ledger account names.
// Exact matches (full path or leaf segment, case-insensitive) win over
// substring matches; substring matches are returned only when nothing is exact.
// An exact match on the full path suppresses substring-only matches.
func MatchAccounts(query string, accounts []string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var exact, partial []string
	for _, a := range accounts {
		la := strings.ToLower(a)
		segs := strings.Split(la, ":")
		leaf := segs[len(segs)-1]
		switch {
		case la == q || leaf == q:
			exact = append(exact, a)
		case strings.Contains(la, q):
			partial = append(partial, a)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return partial
}

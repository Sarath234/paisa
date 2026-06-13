// internal/agent/rulelearning/edit.go
package rulelearning

import (
	"fmt"
	"strings"
)

// FormatEditTemplate returns the editable key:value block sent after ✏️ Edit rule.
func FormatEditTemplate(r PendingRule) string {
	return fmt.Sprintf("Edit and send back:\n\nkeyword: %s\naccount: %s\ndescription: %s",
		r.Keyword, r.Account, r.Description)
}

// ParseEditReply merges changed key:value lines from a Telegram reply into an existing rule.
// Recognised keys: keyword, account, description. Unknown lines are ignored.
func ParseEditReply(text string, existing PendingRule) PendingRule {
	result := existing
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch strings.ToLower(key) {
		case "keyword":
			result.Keyword = val
		case "account":
			result.Account = val
		case "description":
			result.Description = val
		}
	}
	return result
}

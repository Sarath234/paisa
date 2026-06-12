// internal/agent/telegram/format.go
package telegram

import (
	"fmt"
	"github.com/ananthakumaran/paisa/internal/agent/ledger"
	"strings"
)

// FormatDraft renders an Entry as the Telegram approval draft message.
func FormatDraft(e ledger.Entry) string {
	return fmt.Sprintf("📨 New Transaction\n\ndesc: %s\ndate: %s\nsrc:  %s\namt:  %s\ndest: %s",
		e.Desc, e.Date, e.Src, e.Amt, e.Dest)
}

// FormatEditTemplate renders the editable 5-field block sent after ✏️ Edit is tapped.
func FormatEditTemplate(e ledger.Entry) string {
	return fmt.Sprintf("Edit and send back:\n\ndesc: %s\ndate: %s\nsrc:  %s\namt:  %s\ndest: %s",
		e.Desc, e.Date, e.Src, e.Amt, e.Dest)
}

// ParseEditReply merges changed key:value lines from a Telegram reply into an existing Entry.
// Only lines with a recognised key are applied; unrecognised lines are ignored.
// Existing fields are preserved if not present in the reply.
func ParseEditReply(text string, existing ledger.Entry) ledger.Entry {
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
		case "desc":
			result.Desc = val
		case "date":
			result.Date = val
		case "src":
			result.Src = val
		case "amt":
			result.Amt = val
		case "dest":
			result.Dest = val
		}
	}
	return result
}

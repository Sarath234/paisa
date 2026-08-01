// cmd/paisa-agent/cc_due_callback.go
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
	"github.com/ananthakumaran/paisa/internal/agent/telegram"
	log "github.com/sirupsen/logrus"
)

// parseCCDueCallback parses a ccdue_paid:/ccdue_remind: callback_data value
// (built by monitor.CCDueMonitor's ccDueButtons) into the action
// ("paid"/"remind") and the bill's identity. account may itself contain
// colons (e.g. "Liabilities:CreditCard:ICIC6009"), so dueDate — always
// YYYY-MM-DD, never containing a colon — is split off from the END of the
// string, not by a fixed position from the start. Operates on the raw,
// original-case callback data (never the lowercased copy main.go's switch
// uses for dispatch) since account names are case-sensitive.
func parseCCDueCallback(rawData string) (action, account string, dueDate time.Time, err error) {
	var rest string
	switch {
	case strings.HasPrefix(rawData, "ccdue_paid:"):
		action = "paid"
		rest = strings.TrimPrefix(rawData, "ccdue_paid:")
	case strings.HasPrefix(rawData, "ccdue_remind:"):
		action = "remind"
		rest = strings.TrimPrefix(rawData, "ccdue_remind:")
	default:
		return "", "", time.Time{}, fmt.Errorf("not a ccdue callback: %q", rawData)
	}

	idx := strings.LastIndex(rest, ":")
	if idx < 0 {
		return "", "", time.Time{}, fmt.Errorf("malformed ccdue callback data: %q", rawData)
	}
	account = rest[:idx]
	dueDateStr := rest[idx+1:]
	dueDate, err = time.ParseInLocation("2006-01-02", dueDateStr, time.Local)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("parse due date %q: %w", dueDateStr, err)
	}
	return action, account, dueDate, nil
}

// handleCCDueCallback reacts to a Paid/Remind-later button tap on a
// CCDueMonitor insight. Unlike the draft-approval callbacks, there is no
// pending-message store to go stale: billtruth.Store is disk-backed, so a
// tap after an agent restart works exactly like one before it.
func handleCCDueCallback(cb *telegram.CallbackQuery, bot *telegram.Bot, truthStore *billtruth.Store) {
	action, account, dueDate, err := parseCCDueCallback(cb.Data)
	if err != nil {
		log.Errorf("ccdue callback: %v", err)
		return
	}
	msgID := cb.Message.MessageID
	switch action {
	case "paid":
		if err := truthStore.SetUserPaid(account, dueDate); err != nil {
			log.Warnf("ccdue: set user paid: %v", err)
			if editErr := bot.EditMessage(msgID, "⚠️ Couldn't find that bill (may already be reconciled)\n\n"+cb.Message.Text); editErr != nil {
				log.Errorf("ccdue: edit message: %v", editErr)
			}
			return
		}
		if err := bot.EditMessage(msgID, "✅ Marked paid (self-reported)\n\n"+cb.Message.Text); err != nil {
			log.Errorf("ccdue: edit message: %v", err)
		}
	case "remind":
		if err := bot.EditMessage(msgID, "⏰ OK, will remind again\n\n"+cb.Message.Text); err != nil {
			log.Errorf("ccdue: edit message: %v", err)
		}
	}
}

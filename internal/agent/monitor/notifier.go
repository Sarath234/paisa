package monitor

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
)

// TextSender is the slice of telegram.Bot the notifier needs.
type TextSender interface {
	SendText(text string) error
}

// Notifier routes insights: Immediate → Telegram now; Digest → queued until
// FlushDigest. Dedupe is by Insight.Key via the store.
type Notifier struct {
	bot   TextSender
	store *Store
}

func NewNotifier(bot TextSender, store *Store) *Notifier {
	return &Notifier{bot: bot, store: store}
}

func (n *Notifier) Deliver(monitorName string, insights []Insight) {
	for _, in := range insights {
		if n.store.WasSent(in.Key) {
			continue
		}
		switch in.Urgency {
		case Immediate:
			if err := n.bot.SendText(render(in)); err != nil {
				log.Warnf("monitor %s: send %s: %v", monitorName, in.Key, err)
				continue // not marked sent → retried next run
			}
			n.store.MarkSent(in.Key)
		case Digest:
			n.store.MarkSent(in.Key)
			n.store.EnqueueDigest(monitorName, in)
		}
	}
	if err := n.store.Save(); err != nil {
		log.Warnf("monitor: save state: %v", err)
	}
}

// FlushDigest sends all queued insights as one grouped message and drains
// the queue. Empty queue → nil without sending. Send failure keeps the queue.
func (n *Notifier) FlushDigest() error {
	queue := n.store.DigestQueue()
	if len(queue) == 0 {
		return nil
	}

	// Group by monitor, preserving first-appearance order
	order := []string{}
	groups := make(map[string][]QueuedInsight)
	seen := make(map[string]bool)

	for _, q := range queue {
		if !seen[q.Monitor] {
			seen[q.Monitor] = true
			order = append(order, q.Monitor)
		}
		groups[q.Monitor] = append(groups[q.Monitor], q)
	}

	// Render grouped message
	var sb strings.Builder
	noun := "insights"
	if len(queue) == 1 {
		noun = "insight"
	}
	fmt.Fprintf(&sb, "🌅 Daily digest — %d %s\n", len(queue), noun)
	for _, monitor := range order {
		fmt.Fprintf(&sb, "\n%s:\n", monitor)
		for _, q := range groups[monitor] {
			fmt.Fprintf(&sb, "• %s\n", q.Title)
			if q.Body != "" {
				fmt.Fprintf(&sb, "  %s\n", q.Body)
			}
		}
	}

	if err := n.bot.SendText(sb.String()); err != nil {
		return err
	}
	n.store.ClearDigestQueue()
	if err := n.store.Save(); err != nil {
		log.Warnf("monitor: save state: %v", err)
	}
	return nil
}

func render(in Insight) string {
	if in.Body == "" {
		return in.Title
	}
	return in.Title + "\n" + in.Body
}

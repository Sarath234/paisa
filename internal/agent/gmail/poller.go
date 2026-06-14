// internal/agent/gmail/poller.go
package gmail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
)

const pollInterval = 5 * time.Minute
const processedFile = "processed-emails.json"

// StatementEmail is emitted when a new unread statement email is found.
type StatementEmail struct {
	MessageID     string
	Subject       string
	PDFBytes      []byte
	LedgerAccount string // from config statement_accounts match
}

// Poller periodically checks Gmail for new statement emails.
type Poller struct {
	client         *Client
	subjectMatches []SubjectMatch
	stateDir       string // where to store processed-emails.json
	handler        func(StatementEmail)
}

// SubjectMatch pairs an email subject pattern with a ledger account.
type SubjectMatch struct {
	Pattern       string
	LedgerAccount string
}

// NewPoller creates a Poller. stateDir is where processed-emails.json is written.
func NewPoller(client *Client, matches []SubjectMatch, stateDir string, handler func(StatementEmail)) *Poller {
	return &Poller{
		client:         client,
		subjectMatches: matches,
		stateDir:       stateDir,
		handler:        handler,
	}
}

// Start runs the poll loop in the current goroutine (call via go p.Start()).
func (p *Poller) Start() {
	p.poll() // poll once immediately on startup
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for range ticker.C {
		p.poll()
	}
}

func (p *Poller) poll() {
	if p.client == nil || !p.client.IsAuthorized() {
		return
	}
	processed := p.loadProcessed()
	for _, sm := range p.subjectMatches {
		msgs, err := p.client.Search(sm.Pattern)
		if err != nil {
			log.Warnf("gmail: search %q: %v", sm.Pattern, err)
			continue
		}
		for _, msg := range msgs {
			if processed[msg.ID] {
				continue
			}
			pdfBytes, err := p.client.DownloadPDF(msg.ID)
			if err != nil {
				log.Warnf("gmail: download PDF %s: %v", msg.ID, err)
				continue
			}
			p.handler(StatementEmail{
				MessageID:     msg.ID,
				Subject:       msg.Subject,
				PDFBytes:      pdfBytes,
				LedgerAccount: sm.LedgerAccount,
			})
			if err := p.client.MarkRead(msg.ID); err != nil {
				log.Warnf("gmail: mark read %s: %v", msg.ID, err)
			}
			processed[msg.ID] = true
			p.saveProcessed(processed)
		}
	}
}

func (p *Poller) processedPath() string {
	return filepath.Join(p.stateDir, processedFile)
}

func (p *Poller) loadProcessed() map[string]bool {
	data, err := os.ReadFile(p.processedPath())
	if err != nil {
		return make(map[string]bool)
	}
	var m map[string]bool
	if err := json.Unmarshal(data, &m); err != nil {
		return make(map[string]bool)
	}
	return m
}

func (p *Poller) saveProcessed(m map[string]bool) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(p.stateDir, 0755)
	_ = os.WriteFile(p.processedPath(), data, 0644)
}

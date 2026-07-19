// Package dropfolder polls a directory for statement PDFs and feeds them to
// the same handler the Gmail poller uses — a filesystem fallback for users
// without Gmail integration. Processed files move to processed/, problem
// files to failed/; the folder itself is the state.
package dropfolder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type AccountMatch struct {
	Pattern       string // filepath glob, matched case-insensitively against the base name
	LedgerAccount string
	Kind          string
	Password      string
}

type Statement struct {
	Filename      string
	PDFBytes      []byte
	LedgerAccount string
	Kind          string
	Password      string
}

type Poller struct {
	Now      func() time.Time
	Interval time.Duration
	MinAge   time.Duration

	dir     string
	matches []AccountMatch
	handler func(Statement) error
	notify  func(string)
	kick    chan struct{}
}

func New(dir string, matches []AccountMatch, handler func(Statement) error, notify func(string)) *Poller {
	return &Poller{
		Now:      time.Now,
		Interval: time.Minute,
		MinAge:   30 * time.Second,
		dir:      dir,
		matches:  matches,
		handler:  handler,
		notify:   notify,
		kick:     make(chan struct{}, 1),
	}
}

// Start runs the poll loop in the current goroutine (call via go p.Start()).
func (p *Poller) Start() {
	p.PollOnce()
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.PollOnce()
		case <-p.kick:
			p.PollOnce()
		}
	}
}

// Kick asks the poll loop to scan now instead of waiting for the ticker.
// Non-blocking; concurrent kicks coalesce into one scan.
func (p *Poller) Kick() {
	select {
	case p.kick <- struct{}{}:
	default:
	}
}

// PollOnce scans the drop dir once, handling every settled PDF.
func (p *Poller) PollOnce() {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		log.Warnf("dropfolder: read %s: %v", p.dir, err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			continue
		}
		info, err := e.Info()
		if err != nil || p.Now().Sub(info.ModTime()) < p.MinAge {
			continue // still being copied (or vanished); next poll
		}
		p.process(e.Name())
	}
}

// MatchAccount returns the first AccountMatch whose glob pattern matches
// name case-insensitively, or nil. Bad patterns are logged and skipped.
func MatchAccount(name string, matches []AccountMatch) *AccountMatch {
	for i := range matches {
		ok, err := filepath.Match(strings.ToLower(matches[i].Pattern), strings.ToLower(name))
		if err != nil {
			log.Warnf("dropfolder: bad pattern %q: %v", matches[i].Pattern, err)
			continue
		}
		if ok {
			return &matches[i]
		}
	}
	return nil
}

func (p *Poller) process(name string) {
	path := filepath.Join(p.dir, name)

	matched := MatchAccount(name, p.matches)
	if matched == nil {
		p.notify(fmt.Sprintf("❌ Statement file %q matches no configured account — moved to failed/", name))
		p.moveTo(name, "failed")
		return
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return // already processed by a concurrent scan or removed; not an error
	}
	if err != nil {
		p.notify(fmt.Sprintf("❌ Could not read statement file %q: %v — moved to failed/", name, err))
		p.moveTo(name, "failed")
		return
	}

	stmt := Statement{
		Filename:      name,
		PDFBytes:      data,
		LedgerAccount: matched.LedgerAccount,
		Kind:          matched.Kind,
		Password:      matched.Password,
	}
	if err := p.handler(stmt); err != nil {
		log.Warnf("dropfolder: handle %s: %v", name, err)
		p.moveTo(name, "failed")
		return
	}
	p.moveTo(name, "processed")
}

// moveTo moves <dir>/<name> into <dir>/<subdir>/, adding -1, -2… on collision.
func (p *Poller) moveTo(name, subdir string) {
	dest := filepath.Join(p.dir, subdir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		log.Warnf("dropfolder: mkdir %s: %v", dest, err)
		return
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	target := filepath.Join(dest, name)
	for i := 1; ; i++ {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			break
		}
		target = filepath.Join(dest, fmt.Sprintf("%s-%d%s", base, i, ext))
	}
	if err := os.Rename(filepath.Join(p.dir, name), target); err != nil {
		log.Warnf("dropfolder: move %s: %v", name, err)
	}
}

package dropfolder

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name string, age time.Duration) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("%PDF-fake"), 0644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(-age)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return path
}

func newPoller(dir string, handler func(Statement) error, notify func(string)) *Poller {
	p := New(dir, []AccountMatch{{Pattern: "*axis*", LedgerAccount: "Liabilities:CreditCard:Axis"}}, handler, notify)
	return p
}

func TestPollOnceProcessesMatchedPDF(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Axis-CC-July.pdf", time.Minute)

	var got Statement
	p := newPoller(dir, func(s Statement) error { got = s; return nil }, func(string) {})
	p.PollOnce()

	if got.LedgerAccount != "Liabilities:CreditCard:Axis" {
		t.Fatalf("handler not called correctly: %+v", got)
	}
	if got.Filename != "Axis-CC-July.pdf" || string(got.PDFBytes) != "%PDF-fake" {
		t.Fatalf("statement: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "processed", "Axis-CC-July.pdf")); err != nil {
		t.Errorf("file should move to processed/: %v", err)
	}
}

func TestPollOnceCarriesKindAndPasswordFromMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Axis-CC-July.pdf", time.Minute)

	var got Statement
	p := New(dir, []AccountMatch{{
		Pattern:       "*axis*",
		LedgerAccount: "Liabilities:CreditCard:Axis",
		Kind:          "credit_card",
		Password:      "pw123",
	}}, func(s Statement) error { got = s; return nil }, func(string) {})
	p.PollOnce()

	if got.Kind != "credit_card" || got.Password != "pw123" {
		t.Fatalf("kind/password not passed through: %+v", got)
	}
}

func TestPollOnceDispositions(t *testing.T) {
	t.Run("handler error → failed/", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "axis-bad.pdf", time.Minute)
		p := newPoller(dir, func(Statement) error { return errors.New("parse error") }, func(string) {})
		p.PollOnce()
		if _, err := os.Stat(filepath.Join(dir, "failed", "axis-bad.pdf")); err != nil {
			t.Errorf("want failed/: %v", err)
		}
	})

	t.Run("unmatched filename → failed/ + notify", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "hdfc-statement.pdf", time.Minute)
		var note string
		called := false
		p := newPoller(dir, func(Statement) error { called = true; return nil }, func(msg string) { note = msg })
		p.PollOnce()
		if called {
			t.Error("handler must not run for unmatched file")
		}
		if _, err := os.Stat(filepath.Join(dir, "failed", "hdfc-statement.pdf")); err != nil {
			t.Errorf("want failed/: %v", err)
		}
		if note == "" {
			t.Error("want notify for unmatched file")
		}
	})

	t.Run("too-recent mtime skipped", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "axis-fresh.pdf", 5*time.Second)
		called := false
		p := newPoller(dir, func(Statement) error { called = true; return nil }, func(string) {})
		p.PollOnce()
		if called {
			t.Error("file younger than MinAge must be skipped")
		}
		if _, err := os.Stat(filepath.Join(dir, "axis-fresh.pdf")); err != nil {
			t.Error("skipped file must stay in place")
		}
	})

	t.Run("non-pdf ignored", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "axis-notes.txt", time.Minute)
		called := false
		p := newPoller(dir, func(Statement) error { called = true; return nil }, func(string) {})
		p.PollOnce()
		if called {
			t.Error("non-pdf must be ignored")
		}
	})

	t.Run("name collision gets numeric suffix", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "processed"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "processed", "axis.pdf"), []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
		writeFile(t, dir, "axis.pdf", time.Minute)
		p := newPoller(dir, func(Statement) error { return nil }, func(string) {})
		p.PollOnce()
		if _, err := os.Stat(filepath.Join(dir, "processed", "axis-1.pdf")); err != nil {
			t.Errorf("want collision suffix axis-1.pdf: %v", err)
		}
	})
}

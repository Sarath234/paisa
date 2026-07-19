package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.WasSent("k1") {
		t.Fatal("fresh store: k1 should be unsent")
	}
	s.MarkSent("k1")
	s.SetLastRun("cc_due", at("08:15"))
	s.SetLastDigest(at("08:15"))
	s.EnqueueDigest("cc_utilization", Insight{Key: "k2", Title: "T", Body: "B"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	s2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.WasSent("k1") {
		t.Error("k1 should persist as sent")
	}
	if !s2.LastRun("cc_due").Equal(at("08:15")) {
		t.Errorf("lastRun: %v", s2.LastRun("cc_due"))
	}
	if !s2.LastRun("never").IsZero() {
		t.Error("unknown monitor lastRun should be zero")
	}
	if !s2.LastDigest().Equal(at("08:15")) {
		t.Errorf("lastDigest: %v", s2.LastDigest())
	}
	q := s2.DigestQueue()
	if len(q) != 1 || q[0].Monitor != "cc_utilization" || q[0].Key != "k2" {
		t.Fatalf("queue: %+v", q)
	}
	s2.ClearDigestQueue()
	if len(s2.DigestQueue()) != 0 {
		t.Error("queue should be empty after clear")
	}
}

func TestStoreCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "monitor-state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.WasSent("anything") {
		t.Error("corrupt store should start fresh")
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("corrupt file should be renamed to .bak: %v", err)
	}
}

func TestStoreWasSentPrefix(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.WasSentPrefix("cc-stmt/Liabilities:CreditCard:ICIC6009/2026-07-10/") {
		t.Fatal("fresh store: prefix should be unmatched")
	}
	s.MarkSent("cc-stmt/Liabilities:CreditCard:ICIC6009/2026-07-10/23451")
	if !s.WasSentPrefix("cc-stmt/Liabilities:CreditCard:ICIC6009/2026-07-10/") {
		t.Error("want prefix match on exact-prefix key")
	}
	if s.WasSentPrefix("cc-stmt/Liabilities:CreditCard:ICIC6009/2026-08-10/") {
		t.Error("different period prefix must not match")
	}
	if s.WasSentPrefix("cc-stmt/Liabilities:CreditCard:HDFC/") {
		t.Error("different account prefix must not match")
	}
}

func TestStorePrunesOldSentKeys(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	old := at("08:00").Add(-91 * 24 * time.Hour)
	s.Now = func() time.Time { return old }
	s.MarkSent("ancient")
	s.Now = func() time.Time { return at("08:00") }
	s.MarkSent("recent")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s2.WasSent("ancient") {
		t.Error("ancient key should be pruned")
	}
	if !s2.WasSent("recent") {
		t.Error("recent key should survive")
	}
}

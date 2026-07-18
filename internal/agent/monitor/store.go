package monitor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const stateFile = "monitor-state.json"
const sentRetention = 90 * 24 * time.Hour

type QueuedInsight struct {
	Monitor string `json:"monitor"`
	Key     string `json:"key"`
	Title   string `json:"title"`
	Body    string `json:"body"`
}

type storeState struct {
	LastRun     map[string]time.Time `json:"lastRun"`
	Sent        map[string]time.Time `json:"sent"`
	DigestQueue []QueuedInsight      `json:"digestQueue"`
	LastDigest  time.Time            `json:"lastDigest"`
}

// Store persists monitor scheduling and dedupe state as JSON.
// Not safe for concurrent use; the scheduler is the only writer.
type Store struct {
	Now   func() time.Time
	path  string
	state storeState
}

// OpenStore loads <stateDir>/monitor-state.json. A corrupt file is renamed
// aside to .bak and a fresh state is returned.
func OpenStore(stateDir string) (*Store, error) {
	s := &Store{
		Now:  time.Now,
		path: filepath.Join(stateDir, stateFile),
		state: storeState{
			LastRun: map[string]time.Time{},
			Sent:    map[string]time.Time{},
		},
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var loaded storeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		log.Warnf("monitor: corrupt state file, starting fresh: %v", err)
		_ = os.Rename(s.path, s.path+".bak")
		return s, nil
	}
	if loaded.LastRun == nil {
		loaded.LastRun = map[string]time.Time{}
	}
	if loaded.Sent == nil {
		loaded.Sent = map[string]time.Time{}
	}
	s.state = loaded
	return s, nil
}

// Save writes the state atomically, pruning sent keys older than 90 days.
func (s *Store) Save() error {
	cutoff := s.Now().Add(-sentRetention)
	for k, at := range s.state.Sent {
		if at.Before(cutoff) {
			delete(s.state.Sent, k)
		}
	}
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func (s *Store) WasSent(key string) bool { _, ok := s.state.Sent[key]; return ok }

// WasSentPrefix reports whether any sent key starts with prefix. Linear scan
// over the sent-key map (bounded to a few thousand keys by 90-day pruning).
func (s *Store) WasSentPrefix(prefix string) bool {
	for k := range s.state.Sent {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

func (s *Store) MarkSent(key string) { s.state.Sent[key] = s.Now() }

func (s *Store) LastRun(name string) time.Time { return s.state.LastRun[name] }

func (s *Store) SetLastRun(name string, at time.Time) { s.state.LastRun[name] = at }

func (s *Store) LastDigest() time.Time { return s.state.LastDigest }

func (s *Store) SetLastDigest(at time.Time) { s.state.LastDigest = at }

func (s *Store) EnqueueDigest(monitorName string, in Insight) {
	s.state.DigestQueue = append(s.state.DigestQueue, QueuedInsight{
		Monitor: monitorName,
		Key:     in.Key,
		Title:   in.Title,
		Body:    in.Body,
	})
}

func (s *Store) DigestQueue() []QueuedInsight { return s.state.DigestQueue }

func (s *Store) ClearDigestQueue() { s.state.DigestQueue = nil }

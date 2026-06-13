// internal/agent/rulelearning/store.go
package rulelearning

import "sync"

type PendingRule struct {
	MessageID   int
	ChatID      int64
	Keyword     string
	Account     string
	Description string
}

type Store struct {
	mu    sync.Mutex
	items map[int]*PendingRule
}

func NewStore() *Store {
	return &Store{items: make(map[int]*PendingRule)}
}

func (s *Store) Set(r *PendingRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[r.MessageID] = r
}

func (s *Store) Get(messageID int) *PendingRule {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items[messageID]
}

func (s *Store) Delete(messageID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, messageID)
}

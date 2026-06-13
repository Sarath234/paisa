// internal/agent/rulelearning/store.go
package rulelearning

import "sync"

type RuleStatus string

const (
	RuleStatusPending RuleStatus = "pending"
	RuleStatusEditing RuleStatus = "editing"
)

type PendingRule struct {
	MessageID   int
	ChatID      int64
	Keyword     string
	Account     string
	Description string
	Status      RuleStatus
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

func (s *Store) SetEditing(messageID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.items[messageID]; ok {
		r.Status = RuleStatusEditing
	}
}

func (s *Store) GetEditingByChatID(chatID int64) *PendingRule {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.items {
		if r.ChatID == chatID && r.Status == RuleStatusEditing {
			return r
		}
	}
	return nil
}

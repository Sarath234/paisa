// internal/agent/approval/state.go
package approval

import (
	"sync"
	"github.com/ananthakumaran/paisa/internal/agent/ledger"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusEditing Status = "editing"
)

type Pending struct {
	Entry     ledger.Entry
	ChatID    int64
	MessageID int
	Status    Status
}

type Store struct {
	mu    sync.Mutex
	items map[int]*Pending
}

func NewStore() *Store {
	return &Store{items: make(map[int]*Pending)}
}

func (s *Store) Set(p *Pending) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[p.MessageID] = p
}

func (s *Store) Get(messageID int) *Pending {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items[messageID]
}

func (s *Store) SetEditing(messageID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.items[messageID]; ok {
		p.Status = StatusEditing
	}
}

// GetEditingByChatID finds the pending entry for a chat that is in editing state.
// Used to route the user's text reply to the correct pending entry.
func (s *Store) GetEditingByChatID(chatID int64) *Pending {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.items {
		if p.ChatID == chatID && p.Status == StatusEditing {
			return p
		}
	}
	return nil
}

func (s *Store) Delete(messageID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, messageID)
}

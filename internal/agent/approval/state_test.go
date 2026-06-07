// internal/agent/approval/state_test.go
package approval_test

import (
	"sync"
	"testing"
	"github.com/ananthakumaran/paisa/internal/agent/approval"
	"github.com/ananthakumaran/paisa/internal/agent/ledger"
	"github.com/stretchr/testify/assert"
)

func TestStore_SetAndGet(t *testing.T) {
	s := approval.NewStore()
	p := &approval.Pending{
		Entry:     ledger.Entry{Desc: "Food Swiggy"},
		ChatID:    42,
		MessageID: 100,
		Status:    approval.StatusPending,
	}
	s.Set(p)
	got := s.Get(100)
	assert.NotNil(t, got)
	assert.Equal(t, "Food Swiggy", got.Entry.Desc)
}

func TestStore_GetMissing(t *testing.T) {
	s := approval.NewStore()
	assert.Nil(t, s.Get(999))
}

func TestStore_SetEditing(t *testing.T) {
	s := approval.NewStore()
	s.Set(&approval.Pending{MessageID: 1, ChatID: 42, Status: approval.StatusPending})
	s.SetEditing(1)
	assert.Equal(t, approval.StatusEditing, s.Get(1).Status)
}

func TestStore_GetEditingByChatID(t *testing.T) {
	s := approval.NewStore()
	s.Set(&approval.Pending{MessageID: 1, ChatID: 42, Status: approval.StatusPending})
	s.SetEditing(1)
	p := s.GetEditingByChatID(42)
	assert.NotNil(t, p)
	assert.Equal(t, 1, p.MessageID)
	// Different chatID returns nil
	assert.Nil(t, s.GetEditingByChatID(99))
}

func TestStore_Delete(t *testing.T) {
	s := approval.NewStore()
	s.Set(&approval.Pending{MessageID: 1})
	s.Delete(1)
	assert.Nil(t, s.Get(1))
}

func TestStore_ConcurrentAccess(t *testing.T) {
	s := approval.NewStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			s.Set(&approval.Pending{MessageID: id})
			s.Get(id)
			s.Delete(id)
		}(i)
	}
	wg.Wait()
}

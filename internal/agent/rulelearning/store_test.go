// internal/agent/rulelearning/store_test.go
package rulelearning

import "testing"

func TestStore_SetEditing(t *testing.T) {
	s := NewStore()
	s.Set(&PendingRule{MessageID: 1, ChatID: 42, Keyword: "swiggy", Status: RuleStatusPending})
	s.SetEditing(1)
	r := s.Get(1)
	if r == nil {
		t.Fatal("rule not found after SetEditing")
	}
	if r.Status != RuleStatusEditing {
		t.Errorf("Status=%q want %q", r.Status, RuleStatusEditing)
	}
}

func TestStore_SetEditing_UnknownID(t *testing.T) {
	s := NewStore()
	s.SetEditing(999) // must not panic
}

func TestStore_GetEditingByChatID_found(t *testing.T) {
	s := NewStore()
	s.Set(&PendingRule{MessageID: 1, ChatID: 42, Keyword: "swiggy", Status: RuleStatusPending})
	s.Set(&PendingRule{MessageID: 2, ChatID: 42, Keyword: "zomato", Status: RuleStatusPending})
	s.SetEditing(1)

	r := s.GetEditingByChatID(42)
	if r == nil {
		t.Fatal("expected editing rule, got nil")
	}
	if r.MessageID != 1 {
		t.Errorf("MessageID=%d want 1", r.MessageID)
	}
}

func TestStore_GetEditingByChatID_none(t *testing.T) {
	s := NewStore()
	s.Set(&PendingRule{MessageID: 1, ChatID: 42, Keyword: "swiggy", Status: RuleStatusPending})
	if r := s.GetEditingByChatID(42); r != nil {
		t.Errorf("expected nil when no rule is editing, got %+v", r)
	}
}

package billtruth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const stateFile = "bill-truth.json"
const keepCycles = 12

// Store persists bills as JSON. Safe for concurrent use: the Telegram
// handler, drop-folder poller, and scheduler all write.
type Store struct {
	Now   func() time.Time
	mu    sync.Mutex
	path  string
	bills map[string][]Bill // account → bills
}

func Open(stateDir string) (*Store, error) {
	s := &Store{
		Now:   time.Now,
		path:  filepath.Join(stateDir, stateFile),
		bills: map[string][]Bill{},
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var loaded map[string][]Bill
	if err := json.Unmarshal(data, &loaded); err != nil {
		log.Warnf("billtruth: corrupt state file, starting fresh: %v", err)
		_ = os.Rename(s.path, s.path+".bak")
		return s, nil
	}
	s.bills = loaded
	return s, nil
}

// Save writes atomically, keeping only the newest keepCycles bills per card.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	for acct, bills := range s.bills {
		if len(bills) > keepCycles {
			sort.Slice(bills, func(i, j int) bool {
				return bills[i].PeriodEnd.After(bills[j].PeriodEnd)
			})
			s.bills[acct] = bills[:keepCycles]
		}
	}
	data, err := json.MarshalIndent(s.bills, "", "  ")
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

// BillsFor returns deep copies of the account's bills, newest PeriodEnd
// first. Callers may mutate the result freely without corrupting the store.
func (s *Store) BillsFor(account string) []Bill {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.bills[account]
	out := make([]Bill, len(src))
	for i, b := range src {
		if b.Sources != nil {
			sources := make(map[string]Authority, len(b.Sources))
			for k, v := range b.Sources {
				sources[k] = v
			}
			b.Sources = sources
		}
		if b.PaidDate != nil {
			paid := *b.PaidDate
			b.PaidDate = &paid
		}
		if b.UserPaidDate != nil {
			userPaid := *b.UserPaidDate
			b.UserPaidDate = &userPaid
		}
		out[i] = b
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PeriodEnd.After(out[j].PeriodEnd) })
	return out
}

// Accounts returns every account with at least one bill, sorted.
func (s *Store) Accounts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for a := range s.bills {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// findBillByDateLocked returns a pointer into the store's internal slice for the
// account's bill whose DueDate falls on the same calendar day as dueDate,
// or nil. Unlike BillsFor, this is NOT a deep copy — it exists so
// SetUserPaid can mutate in place under the lock; do not use it to hand a
// bill out past the lock.
func (s *Store) findBillByDateLocked(account string, dueDate time.Time) *Bill {
	bills := s.bills[account]
	for i := range bills {
		by, bm, bd := bills[i].DueDate.Date()
		dy, dm, dd := dueDate.Date()
		if by == dy && bm == dm && bd == dd {
			return &bills[i]
		}
	}
	return nil
}

// FindBill returns a deep-copied snapshot of the account's bill whose
// DueDate falls on the same calendar day as dueDate, or nil. Used by the
// Telegram callback handler, which only has account+dueDate (round-tripped
// through callback_data), not the full Bill.
func (s *Store) FindBill(account string, dueDate time.Time) *Bill {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.findBillByDateLocked(account, dueDate)
	if b == nil {
		return nil
	}
	cp := *b
	if b.UserPaidDate != nil {
		userPaid := *b.UserPaidDate
		cp.UserPaidDate = &userPaid
	}
	if b.PaidDate != nil {
		paid := *b.PaidDate
		cp.PaidDate = &paid
	}
	if b.Sources != nil {
		sources := make(map[string]Authority, len(b.Sources))
		for k, v := range b.Sources {
			sources[k] = v
		}
		cp.Sources = sources
	}
	return &cp
}

// SetUserPaid marks the account's bill (matched by calendar-day DueDate)
// self-reported-paid (UserPaidDate = now) and saves. Returns an error if no
// matching bill exists (e.g. a stale button tapped after the due date
// shifted or bill-truth.json was reset).
func (s *Store) SetUserPaid(account string, dueDate time.Time) error {
	s.mu.Lock()
	b := s.findBillByDateLocked(account, dueDate)
	if b == nil {
		s.mu.Unlock()
		return fmt.Errorf("billtruth: no bill for %s due %s", account, dueDate.Format("2006-01-02"))
	}
	now := s.Now()
	b.UserPaidDate = &now
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}

// putForTest inserts a bill without merge logic. Test helper.
func (s *Store) putForTest(b Bill) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bills[b.Account] = append(s.bills[b.Account], b)
}

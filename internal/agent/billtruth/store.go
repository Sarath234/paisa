package billtruth

import (
	"encoding/json"
	"errors"
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

// putForTest inserts a bill without merge logic. Test helper.
func (s *Store) putForTest(b Bill) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bills[b.Account] = append(s.bills[b.Account], b)
}

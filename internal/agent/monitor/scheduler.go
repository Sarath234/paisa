package monitor

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
)

const defaultTick = 15 * time.Minute

// Scheduler runs registered monitors on their cadence and flushes the daily
// digest. One goroutine; monitors run sequentially with panic isolation.
type Scheduler struct {
	Now  func() time.Time
	Tick time.Duration

	monitors   []Monitor
	notifier   *Notifier
	store      *Store
	digestHour int
}

func NewScheduler(monitors []Monitor, notifier *Notifier, store *Store, digestHour int) *Scheduler {
	return &Scheduler{
		Now:        time.Now,
		Tick:       defaultTick,
		monitors:   monitors,
		notifier:   notifier,
		store:      store,
		digestHour: digestHour,
	}
}

// Start runs the tick loop in the current goroutine (call via go s.Start()).
func (s *Scheduler) Start() {
	s.RunOnce()
	ticker := time.NewTicker(s.Tick)
	defer ticker.Stop()
	for range ticker.C {
		s.RunOnce()
	}
}

// RunOnce executes one scheduler pass: due monitors, then the digest flush.
func (s *Scheduler) RunOnce() {
	now := s.Now()
	for _, m := range s.monitors {
		if !m.Due(now, s.store.LastRun(m.Name())) {
			continue
		}
		insights, err := s.checkSafe(m)
		if err != nil {
			log.Warnf("monitor %s: %v", m.Name(), err)
			continue // lastRun not advanced → retried next tick
		}
		s.notifier.Deliver(m.Name(), insights)
		s.store.SetLastRun(m.Name(), now)
		if err := s.store.Save(); err != nil {
			log.Warnf("monitor: save state: %v", err)
		}
	}

	threshold := time.Date(now.Year(), now.Month(), now.Day(), s.digestHour, 0, 0, 0, now.Location())
	if !now.Before(threshold) && s.store.LastDigest().Before(threshold) {
		if err := s.notifier.FlushDigest(); err != nil {
			log.Warnf("monitor: digest flush: %v", err)
			return // LastDigest not advanced → retried next tick
		}
		s.store.SetLastDigest(now)
		if err := s.store.Save(); err != nil {
			log.Warnf("monitor: save state: %v", err)
		}
	}
}

func (s *Scheduler) checkSafe(m Monitor) (insights []Insight, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("monitor %s: panic: %v", m.Name(), r)
			err = nil
			insights = nil
		}
	}()
	return m.Check(context.Background())
}

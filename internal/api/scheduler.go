package api

import (
	"log"
	"sync"
	"time"

	"github.com/relentlessworks/cronkit/internal/model"
	"github.com/relentlessworks/cronkit/internal/store"
)

// Scheduler runs jobs on their schedule.
type Scheduler struct {
	store   *store.Store
	handlers *Handlers
	mu      sync.Mutex
	stopCh  chan struct{}
	ticker  *time.Ticker
}

// NewScheduler creates a new scheduler.
func NewScheduler(s *store.Store) *Scheduler {
	return &Scheduler{
		store:  s,
		stopCh: make(chan struct{}),
	}
}

// SetHandlers connects the scheduler to the handlers (for job execution).
func (sc *Scheduler) SetHandlers(h *Handlers) {
	sc.handlers = h
}

// NotifyJobChange signals the scheduler to re-evaluate jobs.
func (sc *Scheduler) NotifyJobChange() {
	// The scheduler checks every second, so no immediate action needed.
	// This is a hook for future optimization.
}

// Start begins the scheduler loop.
func (sc *Scheduler) Start() {
	sc.ticker = time.NewTicker(1 * time.Second)
	go sc.loop()
	log.Println("scheduler started")
}

// Stop halts the scheduler.
func (sc *Scheduler) Stop() {
	if sc.ticker != nil {
		sc.ticker.Stop()
	}
	close(sc.stopCh)
	log.Println("scheduler stopped")
}

func (sc *Scheduler) loop() {
	for {
		select {
		case <-sc.stopCh:
			return
		case t := <-sc.ticker.C:
			sc.checkJobs(t)
		}
	}
}

func (sc *Scheduler) checkJobs(now time.Time) {
	jobs := sc.store.ListAllJobs()
	for _, job := range jobs {
		if !job.Enabled {
			continue
		}
		// Check if it's time to run (NextRun is at or before now)
		if !job.NextRun.IsZero() && !now.Before(job.NextRun) {
			go sc.runJob(job)
		}
	}
}

func (sc *Scheduler) runJob(job *model.Job) {
	if sc.handlers == nil {
		return
	}
	sc.handlers.executeJob(job)
}

// CalculateNextRun is a helper that wraps model.CalculateNextRun.
func CalculateNextRun(schedule, scheduleType string, from time.Time) (time.Time, error) {
	return model.CalculateNextRun(schedule, scheduleType, from)
}

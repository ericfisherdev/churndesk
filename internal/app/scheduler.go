// internal/app/scheduler.go
package app

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

// fetcherUpdater is an optional interface that fetchers may implement to receive
// live updates to teammate lists and minimum review counts without being recreated.
type fetcherUpdater interface {
	UpdateTeammates([]domain.Teammate)
	UpdateMinReviewCount(int)
}

// Scheduler manages one poll goroutine per integration.
// Each goroutine calls Worker.RunOnce on a configurable interval.
type Scheduler struct {
	items        port.ItemStore
	integrations port.IntegrationStore
	settings     port.SettingsStore
	fetchers     map[domain.Provider]port.Fetcher

	mu      sync.Mutex
	cancels map[int]context.CancelFunc
	ctx     context.Context // server-lifetime context set by Start; used by Reload
}

// NewScheduler constructs a Scheduler. fetchers maps provider to its Fetcher implementation.
func NewScheduler(items port.ItemStore, integrations port.IntegrationStore, settings port.SettingsStore, fetchers map[domain.Provider]port.Fetcher) *Scheduler {
	return &Scheduler{
		items:        items,
		integrations: integrations,
		settings:     settings,
		fetchers:     fetchers,
		cancels:      make(map[int]context.CancelFunc),
	}
}

// Start launches one goroutine per enabled integration. Blocks until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()

	integrations, err := s.integrations.ListIntegrations(ctx)
	if err != nil {
		return err
	}
	for _, integration := range integrations {
		if !integration.Enabled {
			continue
		}
		s.startWorker(ctx, integration)
	}
	<-ctx.Done()
	return nil
}

func (s *Scheduler) startWorker(parent context.Context, integration domain.Integration) {
	fetcher, ok := s.fetchers[integration.Provider]
	if !ok {
		log.Printf("scheduler: no fetcher for provider %s", integration.Provider)
		return
	}
	workerCtx, cancel := context.WithCancel(parent)

	s.mu.Lock()
	s.cancels[integration.ID] = cancel
	s.mu.Unlock()

	worker := NewWorker(fetcher, s.items, s.integrations)
	interval := time.Duration(integration.PollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}

	go func() {
		spaces, err := s.integrations.ListSpaces(workerCtx, integration.ID)
		if err != nil {
			log.Printf("scheduler: list spaces for integration %d: %v", integration.ID, err)
			return
		}
		// Initial sync immediately on start
		if runErr := worker.RunOnce(workerCtx, integration, spaces); runErr != nil {
			log.Printf("scheduler: initial sync for integration %d: %v", integration.ID, runErr)
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				// Refresh spaces on each cycle in case they changed in settings
				spaces, err = s.integrations.ListSpaces(workerCtx, integration.ID)
				if err != nil {
					log.Printf("scheduler: list spaces: %v", err)
					continue
				}
				if err := worker.RunOnce(workerCtx, integration, spaces); err != nil {
					log.Printf("scheduler: sync error for integration %d: %v", integration.ID, err)
				}
			}
		}
	}()
}

// SyncAll triggers an immediate re-sync by reloading all workers.
// It satisfies the handlers.Syncer interface used by FeedHandler.
func (s *Scheduler) SyncAll(ctx context.Context) error {
	return s.Reload(ctx)
}

// Reload stops all running workers and restarts them from the current integration list.
// Safe to call concurrently from the settings handler after integration or space changes.
// No-op if called before Start.
func (s *Scheduler) Reload(_ context.Context) error {
	s.mu.Lock()
	serverCtx := s.ctx
	for _, cancel := range s.cancels {
		cancel()
	}
	s.cancels = make(map[int]context.CancelFunc)
	s.mu.Unlock()

	if serverCtx == nil {
		return nil // Start has not been called yet
	}

	// Push fresh teammate and setting values into any fetcher that supports live updates.
	if err := s.refreshFetchers(serverCtx); err != nil {
		log.Printf("scheduler: refresh fetchers: %v", err)
		// Non-fatal: workers restart with their current (possibly stale) values.
	}

	integrations, err := s.integrations.ListIntegrations(serverCtx)
	if err != nil {
		return err
	}
	for _, ig := range integrations {
		if !ig.Enabled {
			continue
		}
		s.startWorker(serverCtx, ig)
	}
	return nil
}

// refreshFetchers loads the current teammate list and minimum review count from the
// database and pushes them into any fetcher that implements fetcherUpdater.
func (s *Scheduler) refreshFetchers(ctx context.Context) error {
	integrations, err := s.integrations.ListIntegrations(ctx)
	if err != nil {
		return err
	}
	var teammates []domain.Teammate
	for _, ig := range integrations {
		ts, err := s.integrations.ListTeammates(ctx, ig.ID)
		if err != nil {
			return err
		}
		teammates = append(teammates, ts...)
	}

	minReviewCount := 1
	if val, err := s.settings.Get(ctx, domain.SettingMinReviewCount); err == nil && val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			minReviewCount = n
		}
	}

	for _, f := range s.fetchers {
		if u, ok := f.(fetcherUpdater); ok {
			u.UpdateTeammates(teammates)
			u.UpdateMinReviewCount(minReviewCount)
		}
	}
	return nil
}

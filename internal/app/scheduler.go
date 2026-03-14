// internal/app/scheduler.go
package app

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

// Scheduler manages one poll goroutine per integration.
// Each goroutine calls Worker.RunOnce on a configurable interval.
type Scheduler struct {
	items        port.ItemStore
	integrations port.IntegrationStore
	fetchers     map[domain.Provider]port.Fetcher

	mu      sync.Mutex
	cancels map[int]context.CancelFunc
	ctx     context.Context // server-lifetime context set by Start; used by Reload
}

// NewScheduler constructs a Scheduler. fetchers maps provider to its Fetcher implementation.
func NewScheduler(items port.ItemStore, integrations port.IntegrationStore, fetchers map[domain.Provider]port.Fetcher) *Scheduler {
	return &Scheduler{
		items:        items,
		integrations: integrations,
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

	go func() {
		spaces, err := s.integrations.ListSpaces(workerCtx, integration.ID)
		if err != nil {
			log.Printf("scheduler: list spaces for integration %d: %v", integration.ID, err)
			return
		}
		// Initial sync immediately on start
		if err := worker.RunOnce(workerCtx, integration, spaces); err != nil {
			log.Printf("scheduler: initial sync for integration %d: %v", integration.ID, err)
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

// internal/app/worker.go
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

// Worker executes a single sync cycle for one integration:
// fetch → partition deleted/live → upsert/delete → update last_synced_at.
type Worker struct {
	fetcher      port.Fetcher
	items        port.ItemStore
	integrations port.IntegrationStore
}

// NewWorker constructs a Worker with all dependencies injected via constructor.
func NewWorker(fetcher port.Fetcher, items port.ItemStore, integrations port.IntegrationStore) *Worker {
	return &Worker{fetcher: fetcher, items: items, integrations: integrations}
}

// RunOnce executes one sync cycle synchronously. Called by the scheduler's poll loop
// and by the manual sync handler.
func (w *Worker) RunOnce(ctx context.Context, integration domain.Integration, spaces []domain.Space) error {
	syncStartedAt := time.Now().UTC()

	fetched, err := w.fetcher.Fetch(ctx, integration, spaces)
	if err != nil {
		return fmt.Errorf("fetch for integration %d: %w", integration.ID, err)
	}

	var toUpsert, toDelete []domain.Item
	for _, item := range fetched {
		if item.Deleted {
			toDelete = append(toDelete, item)
		} else {
			toUpsert = append(toUpsert, item)
		}
	}

	for _, item := range toDelete {
		if err := w.items.Delete(ctx, item.ID); err != nil {
			return fmt.Errorf("delete item %s: %w", item.ID, err)
		}
	}
	if len(toUpsert) > 0 {
		if err := w.items.Upsert(ctx, toUpsert); err != nil {
			return fmt.Errorf("upsert items: %w", err)
		}
	}

	if err := w.integrations.UpdateLastSyncedAt(ctx, integration.ID, syncStartedAt); err != nil {
		return fmt.Errorf("update last_synced_at for integration %d: %w", integration.ID, err)
	}
	return nil
}

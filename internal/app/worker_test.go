// internal/app/worker_test.go
package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/churndesk/churndesk/internal/app"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubFetcher returns a fixed slice of items on every call.
type stubFetcher struct{ items []domain.Item }

func (s *stubFetcher) Fetch(_ context.Context, _ domain.Integration, _ []domain.Space) ([]domain.Item, error) {
	return s.items, nil
}

// stubItemStore records calls for inspection.
type stubItemStore struct {
	upserted []domain.Item
	deleted  []string
}

func (s *stubItemStore) Upsert(_ context.Context, items []domain.Item) error {
	s.upserted = append(s.upserted, items...)
	return nil
}
func (s *stubItemStore) Delete(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *stubItemStore) ListRanked(_ context.Context, _ int) ([]domain.Item, error) { return nil, nil }
func (s *stubItemStore) Count(_ context.Context) (int, error)                       { return 0, nil }
func (s *stubItemStore) Dismiss(_ context.Context, _ string) error                  { return nil }
func (s *stubItemStore) MarkSeen(_ context.Context, _ string) error                 { return nil }
func (s *stubItemStore) MarkSeenByPR(_ context.Context, _, _ string, _ int) error   { return nil }
func (s *stubItemStore) MarkSeenByJiraKey(_ context.Context, _ string) error        { return nil }
func (s *stubItemStore) RescoreAll(_ context.Context, _ map[domain.ItemType]int, _ []string, _, _ float64) error {
	return nil
}

// stubIntegrationStore records last_synced_at updates.
type stubIntegrationStore struct{ lastSynced time.Time }

func (s *stubIntegrationStore) UpdateLastSyncedAt(_ context.Context, _ int, t time.Time) error {
	s.lastSynced = t
	return nil
}
func (s *stubIntegrationStore) CreateIntegration(ctx context.Context, i domain.Integration) (int, error) {
	return 0, nil
}
func (s *stubIntegrationStore) GetIntegration(ctx context.Context, id int) (*domain.Integration, error) {
	return nil, nil
}
func (s *stubIntegrationStore) UpdateIntegration(ctx context.Context, i domain.Integration) error {
	return nil
}
func (s *stubIntegrationStore) DeleteIntegration(ctx context.Context, id int) error { return nil }
func (s *stubIntegrationStore) ListIntegrations(ctx context.Context) ([]domain.Integration, error) {
	return nil, nil
}
func (s *stubIntegrationStore) CreateSpace(ctx context.Context, sp domain.Space) (int, error) {
	return 0, nil
}
func (s *stubIntegrationStore) ListSpaces(ctx context.Context, id int) ([]domain.Space, error) {
	return nil, nil
}
func (s *stubIntegrationStore) UpdateSpace(ctx context.Context, sp domain.Space) error { return nil }
func (s *stubIntegrationStore) DeleteSpace(ctx context.Context, id int) error          { return nil }
func (s *stubIntegrationStore) ReplaceSpaces(_ context.Context, _ int, _ []domain.Space) error {
	return nil
}
func (s *stubIntegrationStore) CreateTeammate(ctx context.Context, t domain.Teammate) error {
	return nil
}
func (s *stubIntegrationStore) ListTeammates(ctx context.Context, id int) ([]domain.Teammate, error) {
	return nil, nil
}
func (s *stubIntegrationStore) DeleteTeammate(ctx context.Context, id int) error { return nil }
func (s *stubIntegrationStore) ReplaceTeammates(_ context.Context, _ int, _ []domain.Teammate) error {
	return nil
}
func (s *stubIntegrationStore) CreatePrerequisite(ctx context.Context, p domain.ReviewPrerequisite) error {
	return nil
}
func (s *stubIntegrationStore) ListPrerequisites(ctx context.Context, id int) ([]domain.ReviewPrerequisite, error) {
	return nil, nil
}
func (s *stubIntegrationStore) DeletePrerequisite(ctx context.Context, id int) error { return nil }
func (s *stubIntegrationStore) ReplacePrerequisites(_ context.Context, _ int, _ []domain.ReviewPrerequisite) error {
	return nil
}
func (s *stubIntegrationStore) IsOnboardingComplete(ctx context.Context) (bool, error) {
	return true, nil
}

func TestWorker_RunOnce_UpsertsItems(t *testing.T) {
	fetcher := &stubFetcher{items: []domain.Item{
		{ID: "github:review_needed:1", Type: domain.ItemTypePRReviewNeeded, Source: "github"},
	}}
	itemStore := &stubItemStore{}
	integrationStore := &stubIntegrationStore{}

	integration := domain.Integration{ID: 1, Provider: domain.ProviderGitHub}
	spaces := []domain.Space{{Owner: "myorg", Name: "myrepo", Enabled: true}}

	w := app.NewWorker(fetcher, itemStore, integrationStore)
	err := w.RunOnce(context.Background(), integration, spaces)
	require.NoError(t, err)

	assert.Len(t, itemStore.upserted, 1)
	assert.Equal(t, "github:review_needed:1", itemStore.upserted[0].ID)
	assert.False(t, integrationStore.lastSynced.IsZero(), "last_synced_at must be updated after sync")
}

func TestWorker_RunOnce_DeletesItemsWithDeletedFlag(t *testing.T) {
	fetcher := &stubFetcher{items: []domain.Item{
		{ID: "github:approved:1", Type: domain.ItemTypePRApproved, Source: "github", Deleted: true},
		{ID: "github:review_needed:1", Type: domain.ItemTypePRReviewNeeded, Source: "github", Deleted: false},
	}}
	itemStore := &stubItemStore{}
	integrationStore := &stubIntegrationStore{}

	w := app.NewWorker(fetcher, itemStore, integrationStore)
	err := w.RunOnce(context.Background(), domain.Integration{ID: 1}, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"github:approved:1"}, itemStore.deleted)
	assert.Len(t, itemStore.upserted, 1)
	assert.Equal(t, "github:review_needed:1", itemStore.upserted[0].ID)
}

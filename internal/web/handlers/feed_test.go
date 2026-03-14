package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/web/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubItemStore implements handlers.FeedItemStore for tests.
type stubItemStore struct {
	items      []domain.Item
	dismissErr error
	seenErr    error
}

var _ handlers.FeedItemStore = (*stubItemStore)(nil)

func (s *stubItemStore) ListRanked(_ context.Context, _ int) ([]domain.Item, error) {
	return s.items, nil
}
func (s *stubItemStore) Dismiss(_ context.Context, _ string) error { return s.dismissErr }
func (s *stubItemStore) MarkSeen(_ context.Context, _ string) error { return s.seenErr }

// stubSyncer implements handlers.Syncer for tests.
type stubSyncer struct{ called bool }

func (s *stubSyncer) SyncAll(_ context.Context) error {
	s.called = true
	return nil
}

// stubFeedSettingsStore implements handlers.FeedSettingsStore for tests.
type stubFeedSettingsStore struct {
	values map[domain.SettingKey]string
}

func (s *stubFeedSettingsStore) Get(_ context.Context, key domain.SettingKey) (string, error) {
	return s.values[key], nil
}

var _ handlers.FeedSettingsStore = (*stubFeedSettingsStore)(nil)

func defaultTestSettings() *stubFeedSettingsStore {
	return &stubFeedSettingsStore{
		values: map[domain.SettingKey]string{
			domain.SettingFeedColumns:         "1",
			domain.SettingAutoRefreshInterval: "20",
		},
	}
}

func TestFeedFragment_SetsNewItemsHeader(t *testing.T) {
	store := &stubItemStore{items: []domain.Item{{ID: "a"}, {ID: "b"}}}
	h := handlers.NewFeedHandler(store, &stubSyncer{}, defaultTestSettings())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/feed?count=1", nil)
	w := httptest.NewRecorder()
	h.Fragment(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "true", w.Header().Get("X-Has-New-Items"))
}

func TestFeedFragment_NoHeaderWhenCountUnchanged(t *testing.T) {
	store := &stubItemStore{items: []domain.Item{{ID: "a"}, {ID: "b"}}}
	h := handlers.NewFeedHandler(store, &stubSyncer{}, defaultTestSettings())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/feed?count=2", nil)
	w := httptest.NewRecorder()
	h.Fragment(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("X-Has-New-Items"))
}

func TestDismiss_CallsStore(t *testing.T) {
	var dismissed string
	h := handlers.NewFeedHandler(
		&captureDismissStore{items: []domain.Item{}, dismissed: &dismissed},
		&stubSyncer{},
		defaultTestSettings(),
	)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/items/my-item/dismiss", nil)
	req.SetPathValue("id", "my-item")
	w := httptest.NewRecorder()
	h.Dismiss(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "my-item", dismissed)
}

func TestSync_CallsSyncer(t *testing.T) {
	syncer := &stubSyncer{}
	store := &stubItemStore{items: []domain.Item{}}
	h := handlers.NewFeedHandler(store, syncer, defaultTestSettings())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/sync", nil)
	w := httptest.NewRecorder()
	h.Sync(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, syncer.called)
}

func TestFeedFragment_CountParamIgnoredIfNotInt(t *testing.T) {
	store := &stubItemStore{items: []domain.Item{{ID: "a"}}}
	h := handlers.NewFeedHandler(store, &stubSyncer{}, defaultTestSettings())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/feed?count=abc", nil)
	w := httptest.NewRecorder()
	h.Fragment(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	// count=abc parses as 0, 1 item > 0 → has new items
	assert.Equal(t, "true", w.Header().Get("X-Has-New-Items"))
}

// captureDismissStore captures the dismissed item ID for assertion.
type captureDismissStore struct {
	items     []domain.Item
	dismissed *string
}

func (s *captureDismissStore) ListRanked(_ context.Context, _ int) ([]domain.Item, error) {
	return s.items, nil
}
func (s *captureDismissStore) Dismiss(_ context.Context, id string) error {
	*s.dismissed = id
	return nil
}
func (s *captureDismissStore) MarkSeen(_ context.Context, _ string) error { return nil }

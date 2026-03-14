// internal/web/handlers/feed.go
package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/web/templates"
)

// FeedItemStore is the subset of port.ItemStore used by FeedHandler.
type FeedItemStore interface {
	ListRanked(ctx context.Context, limit int) ([]domain.Item, error)
	Dismiss(ctx context.Context, id string) error
	MarkSeen(ctx context.Context, id string) error
}

// Syncer triggers an immediate re-sync across all integrations.
type Syncer interface {
	SyncAll(ctx context.Context) error
}

// FeedSettingsStore is the subset of port.SettingsStore used by FeedHandler.
type FeedSettingsStore interface {
	Get(ctx context.Context, key domain.SettingKey) (string, error)
}

// FeedHandler handles the main feed page and HTMX fragment updates.
type FeedHandler struct {
	items    FeedItemStore
	syncer   Syncer
	settings FeedSettingsStore
}

// NewFeedHandler constructs a FeedHandler.
func NewFeedHandler(items FeedItemStore, syncer Syncer, settings FeedSettingsStore) *FeedHandler {
	return &FeedHandler{items: items, syncer: syncer, settings: settings}
}

func (h *FeedHandler) readColumnsSetting(ctx context.Context) int {
	v, _ := h.settings.Get(ctx, domain.SettingFeedColumns)
	n, _ := strconv.Atoi(v)
	if n < 1 || n > 3 {
		return 1
	}
	return n
}

func (h *FeedHandler) readIntervalSetting(ctx context.Context) int {
	v, _ := h.settings.Get(ctx, domain.SettingAutoRefreshInterval)
	n, _ := strconv.Atoi(v)
	if n < 10 {
		return 20
	}
	return n
}

// Page renders the full feed page (initial load).
func (h *FeedHandler) Page(w http.ResponseWriter, r *http.Request) {
	items, err := h.items.ListRanked(r.Context(), 200)
	if err != nil {
		http.Error(w, "failed to load feed", http.StatusInternalServerError)
		return
	}
	columns := h.readColumnsSetting(r.Context())
	interval := h.readIntervalSetting(r.Context())
	templates.FeedPage(items, columns, interval).Render(r.Context(), w) //nolint:errcheck
}

// Fragment renders just the feed list (HTMX polling update).
// Reads ?count=N from query string to detect new-item count changes.
// Sets X-Has-New-Items: true header when the item count has grown.
func (h *FeedHandler) Fragment(w http.ResponseWriter, r *http.Request) {
	items, err := h.items.ListRanked(r.Context(), 200)
	if err != nil {
		http.Error(w, "failed to load feed", http.StatusInternalServerError)
		return
	}
	prevCount, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if len(items) > prevCount {
		w.Header().Set("X-Has-New-Items", "true")
	}
	columns := h.readColumnsSetting(r.Context())
	interval := h.readIntervalSetting(r.Context())
	templates.FeedFragment(items, columns, interval).Render(r.Context(), w) //nolint:errcheck
}

// Dismiss marks an item as dismissed and returns the updated fragment.
func (h *FeedHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.items.Dismiss(r.Context(), id); err != nil {
		http.Error(w, "dismiss failed", http.StatusInternalServerError)
		return
	}
	items, err := h.items.ListRanked(r.Context(), 200)
	if err != nil {
		http.Error(w, "failed to load feed", http.StatusInternalServerError)
		return
	}
	columns := h.readColumnsSetting(r.Context())
	interval := h.readIntervalSetting(r.Context())
	templates.FeedFragment(items, columns, interval).Render(r.Context(), w) //nolint:errcheck
}

// Seen marks an item as seen. Returns 204 No Content on success.
func (h *FeedHandler) Seen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.items.MarkSeen(r.Context(), id); err != nil {
		http.Error(w, "seen failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Sync triggers an immediate re-sync and returns the updated fragment.
func (h *FeedHandler) Sync(w http.ResponseWriter, r *http.Request) {
	if err := h.syncer.SyncAll(r.Context()); err != nil {
		http.Error(w, "sync failed", http.StatusInternalServerError)
		return
	}
	items, err := h.items.ListRanked(r.Context(), 200)
	if err != nil {
		http.Error(w, "failed to load feed", http.StatusInternalServerError)
		return
	}
	columns := h.readColumnsSetting(r.Context())
	interval := h.readIntervalSetting(r.Context())
	templates.FeedFragment(items, columns, interval).Render(r.Context(), w) //nolint:errcheck
}

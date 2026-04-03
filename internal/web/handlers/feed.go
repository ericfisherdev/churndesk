// internal/web/handlers/feed.go
package handlers

import (
	"context"
	"hash/fnv"
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

// feedFingerprint returns a short hash of item IDs and seen state so the
// polling handler can return 204 when the feed has not changed.
func feedFingerprint(items []domain.Item) string {
	h := fnv.New64a()
	for _, item := range items {
		h.Write([]byte(item.ID))
		if item.Seen == 0 {
			h.Write([]byte{0})
		} else {
			h.Write([]byte{1})
		}
	}
	return strconv.FormatUint(h.Sum64(), 36)
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
	templates.FeedPage(items, columns, interval, feedFingerprint(items)).Render(r.Context(), w) //nolint:errcheck
}

// Fragment renders just the feed list (HTMX polling update).
// Returns 204 No Content when the feed fingerprint matches the client's
// cached value, preventing any DOM swap and thus eliminating flicker.
func (h *FeedHandler) Fragment(w http.ResponseWriter, r *http.Request) {
	items, err := h.items.ListRanked(r.Context(), 200)
	if err != nil {
		http.Error(w, "failed to load feed", http.StatusInternalServerError)
		return
	}
	fp := feedFingerprint(items)
	if r.URL.Query().Get("fp") == fp {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	prevCount, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if len(items) > prevCount {
		w.Header().Set("X-Has-New-Items", "true")
	}
	w.Header().Set("X-Feed-Fingerprint", fp)
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

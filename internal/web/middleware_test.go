// internal/web/middleware_test.go
package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/web"
	"github.com/stretchr/testify/assert"
)

type stubOnboardingStore struct{ complete bool }

func (s *stubOnboardingStore) IsOnboardingComplete(ctx context.Context) (bool, error) {
	return s.complete, nil
}

// All other port.IntegrationStore methods — no-op (only IsOnboardingComplete is used by middleware)
func (s *stubOnboardingStore) CreateIntegration(_ context.Context, _ domain.Integration) (int, error) {
	return 0, nil
}
func (s *stubOnboardingStore) GetIntegration(_ context.Context, _ int) (*domain.Integration, error) {
	return nil, nil
}
func (s *stubOnboardingStore) UpdateIntegration(_ context.Context, _ domain.Integration) error {
	return nil
}
func (s *stubOnboardingStore) DeleteIntegration(_ context.Context, _ int) error { return nil }
func (s *stubOnboardingStore) ListIntegrations(_ context.Context) ([]domain.Integration, error) {
	return nil, nil
}
func (s *stubOnboardingStore) UpdateLastSyncedAt(_ context.Context, _ int, _ time.Time) error {
	return nil
}
func (s *stubOnboardingStore) CreateSpace(_ context.Context, _ domain.Space) (int, error) {
	return 0, nil
}
func (s *stubOnboardingStore) ListSpaces(_ context.Context, _ int) ([]domain.Space, error) {
	return nil, nil
}
func (s *stubOnboardingStore) UpdateSpace(_ context.Context, _ domain.Space) error { return nil }
func (s *stubOnboardingStore) DeleteSpace(_ context.Context, _ int) error          { return nil }
func (s *stubOnboardingStore) CreateTeammate(_ context.Context, _ domain.Teammate) error {
	return nil
}
func (s *stubOnboardingStore) ListTeammates(_ context.Context, _ int) ([]domain.Teammate, error) {
	return nil, nil
}
func (s *stubOnboardingStore) DeleteTeammate(_ context.Context, _ int) error { return nil }
func (s *stubOnboardingStore) CreatePrerequisite(_ context.Context, _ domain.ReviewPrerequisite) error {
	return nil
}
func (s *stubOnboardingStore) ListPrerequisites(_ context.Context, _ int) ([]domain.ReviewPrerequisite, error) {
	return nil, nil
}
func (s *stubOnboardingStore) DeletePrerequisite(_ context.Context, _ int) error { return nil }

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestOnboardingGate_RedirectsWhenIncomplete(t *testing.T) {
	store := &stubOnboardingStore{complete: false}
	mw := web.OnboardingGate(store)(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/settings?setup=1", rec.Header().Get("Location"))
}

func TestOnboardingGate_HTMXRedirectWhenIncomplete(t *testing.T) {
	store := &stubOnboardingStore{complete: false}
	mw := web.OnboardingGate(store)(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/settings?setup=1", rec.Header().Get("HX-Redirect"))
}

func TestOnboardingGate_PassthroughWhenComplete(t *testing.T) {
	store := &stubOnboardingStore{complete: true}
	mw := web.OnboardingGate(store)(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestOnboardingGate_ExemptsPaths(t *testing.T) {
	store := &stubOnboardingStore{complete: false}
	mw := web.OnboardingGate(store)(okHandler)

	exemptPaths := []string{
		"/settings",
		"/settings?setup=1",
		"/static/style.css",
		"/feed",
		"/items/github:approved:1/dismiss",
		"/sync",
	}
	for _, path := range exemptPaths {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "path %s should be exempt", path)
	}
}

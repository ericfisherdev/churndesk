package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/web/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSettingsIntegrationStore struct {
	integrations  []domain.Integration
	spaces        []domain.Space
	teammates     []domain.Teammate
	prerequisites []domain.ReviewPrerequisite
	created       []domain.Integration
}

var _ handlers.SettingsIntegrationStore = (*stubSettingsIntegrationStore)(nil)

func (s *stubSettingsIntegrationStore) CreateIntegration(_ context.Context, i domain.Integration) (int, error) {
	s.created = append(s.created, i)
	return 1, nil
}
func (s *stubSettingsIntegrationStore) GetIntegration(_ context.Context, id int) (*domain.Integration, error) {
	for _, i := range s.integrations {
		if i.ID == id {
			return &i, nil
		}
	}
	return nil, nil
}
func (s *stubSettingsIntegrationStore) UpdateIntegration(_ context.Context, _ domain.Integration) error { return nil }
func (s *stubSettingsIntegrationStore) DeleteIntegration(_ context.Context, _ int) error               { return nil }
func (s *stubSettingsIntegrationStore) ListIntegrations(_ context.Context) ([]domain.Integration, error) {
	return s.integrations, nil
}
func (s *stubSettingsIntegrationStore) UpdateLastSyncedAt(_ context.Context, _ int, _ time.Time) error { return nil }
func (s *stubSettingsIntegrationStore) CreateSpace(_ context.Context, _ domain.Space) (int, error)    { return 0, nil }
func (s *stubSettingsIntegrationStore) ListSpaces(_ context.Context, _ int) ([]domain.Space, error)   { return s.spaces, nil }
func (s *stubSettingsIntegrationStore) UpdateSpace(_ context.Context, _ domain.Space) error           { return nil }
func (s *stubSettingsIntegrationStore) DeleteSpace(_ context.Context, _ int) error                    { return nil }
func (s *stubSettingsIntegrationStore) ReplaceSpaces(_ context.Context, _ int, _ []domain.Space) error { return nil }
func (s *stubSettingsIntegrationStore) CreateTeammate(_ context.Context, _ domain.Teammate) error     { return nil }
func (s *stubSettingsIntegrationStore) ListTeammates(_ context.Context, _ int) ([]domain.Teammate, error) {
	return s.teammates, nil
}
func (s *stubSettingsIntegrationStore) DeleteTeammate(_ context.Context, _ int) error { return nil }
func (s *stubSettingsIntegrationStore) ReplaceTeammates(_ context.Context, _ int, _ []domain.Teammate) error {
	return nil
}
func (s *stubSettingsIntegrationStore) CreatePrerequisite(_ context.Context, _ domain.ReviewPrerequisite) error { return nil }
func (s *stubSettingsIntegrationStore) ListPrerequisites(_ context.Context, _ int) ([]domain.ReviewPrerequisite, error) {
	return s.prerequisites, nil
}
func (s *stubSettingsIntegrationStore) DeletePrerequisite(_ context.Context, _ int) error { return nil }
func (s *stubSettingsIntegrationStore) IsOnboardingComplete(_ context.Context) (bool, error) {
	return len(s.integrations) > 0, nil
}

type stubSettingsHandlerStore struct {
	settings map[domain.SettingKey]string
	weights  []domain.CategoryWeight
}

func (s *stubSettingsHandlerStore) Get(_ context.Context, key domain.SettingKey) (string, error) {
	return s.settings[key], nil
}
func (s *stubSettingsHandlerStore) Set(_ context.Context, key domain.SettingKey, val string) error {
	if s.settings == nil {
		s.settings = make(map[domain.SettingKey]string)
	}
	s.settings[key] = val
	return nil
}
func (s *stubSettingsHandlerStore) GetAll(_ context.Context) (map[domain.SettingKey]string, error) {
	return s.settings, nil
}
func (s *stubSettingsHandlerStore) GetCategoryWeights(_ context.Context) ([]domain.CategoryWeight, error) {
	return s.weights, nil
}
func (s *stubSettingsHandlerStore) SetCategoryWeight(_ context.Context, _ domain.ItemType, _ int) error {
	return nil
}

type stubRescoreStore struct{ rescored bool }

func (s *stubRescoreStore) RescoreAll(_ context.Context, _ map[domain.ItemType]int, _ []string, _, _ float64) error {
	s.rescored = true
	return nil
}

func TestSettingsHandler_Page_Renders(t *testing.T) {
	integrations := &stubSettingsIntegrationStore{}
	settings := &stubSettingsHandlerStore{
		settings: map[domain.SettingKey]string{
			domain.SettingAutoRefreshInterval: "20",
			domain.SettingAgeMultiplier:       "0.5",
			domain.SettingMaxAgeBoost:         "50",
			domain.SettingFeedColumns:         "1",
			domain.SettingMinReviewCount:      "1",
		},
	}
	rescore := &stubRescoreStore{}

	h := handlers.NewSettingsHandler(integrations, settings, rescore, nil)
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/settings", nil)
	rec := httptest.NewRecorder()
	h.Page(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Settings")
}

func TestSettingsHandler_SaveGeneral_ClampsColumns(t *testing.T) {
	integrations := &stubSettingsIntegrationStore{}
	settings := &stubSettingsHandlerStore{settings: make(map[domain.SettingKey]string)}
	rescore := &stubRescoreStore{}

	h := handlers.NewSettingsHandler(integrations, settings, rescore, nil)

	form := url.Values{
		"feed_columns":          {"5"},
		"auto_refresh_interval": {"30"},
		"age_multiplier":        {"0.5"},
		"max_age_boost":         {"50"},
		"min_review_count":      {"2"},
	}
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/settings/general", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.SaveGeneral(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "3", settings.settings[domain.SettingFeedColumns])
}

func TestSettingsHandler_Rescore_CallsStore(t *testing.T) {
	integrations := &stubSettingsIntegrationStore{}
	settings := &stubSettingsHandlerStore{
		settings: map[domain.SettingKey]string{
			domain.SettingAgeMultiplier: "0.5",
			domain.SettingMaxAgeBoost:   "50",
		},
	}
	rescore := &stubRescoreStore{}

	h := handlers.NewSettingsHandler(integrations, settings, rescore, nil)
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/settings/rescore", nil)
	rec := httptest.NewRecorder()
	h.Rescore(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, rescore.rescored)
}

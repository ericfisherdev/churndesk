// internal/app/scorer_test.go
package app_test

import (
	"context"
	"testing"

	"github.com/churndesk/churndesk/internal/app"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSettingsStore implements port.SettingsStore for scorer tests.
type stubSettingsStore struct {
	settings map[domain.SettingKey]string
	weights  []domain.CategoryWeight
}

func (s *stubSettingsStore) Get(_ context.Context, key domain.SettingKey) (string, error) {
	return s.settings[key], nil
}
func (s *stubSettingsStore) Set(_ context.Context, _ domain.SettingKey, _ string) error { return nil }
func (s *stubSettingsStore) GetAll(_ context.Context) (map[domain.SettingKey]string, error) {
	return s.settings, nil
}
func (s *stubSettingsStore) GetCategoryWeights(_ context.Context) ([]domain.CategoryWeight, error) {
	return s.weights, nil
}
func (s *stubSettingsStore) SetCategoryWeight(_ context.Context, _ domain.ItemType, _ int) error {
	return nil
}

// captureRescoreStore records the prerequisiteUsernames passed to RescoreAll.
// Embeds stubItemStore so only RescoreAll needs to be overridden.
type captureRescoreStore struct {
	stubItemStore
	capturedPrereqs []string
}

func (s *captureRescoreStore) RescoreAll(_ context.Context, _ map[domain.ItemType]int, prereqs []string, _, _ float64) error {
	s.capturedPrereqs = prereqs
	return nil
}

func TestScorer_RunOnce_PassesPrerequisites(t *testing.T) {
	settings := &stubSettingsStore{
		settings: map[domain.SettingKey]string{
			domain.SettingAgeMultiplier: "0.5",
			domain.SettingMaxAgeBoost:  "50",
		},
		weights: []domain.CategoryWeight{
			{ItemType: domain.ItemTypePRReviewNeeded, Weight: 60},
		},
	}
	integrations := &stubIntegrationStore{} // reuse from worker_test.go
	capture := &captureRescoreStore{}

	scorer := app.NewScorer(capture, settings, integrations)
	err := scorer.RunOnce(context.Background())
	require.NoError(t, err)
	// no prerequisites configured → empty slice passed to RescoreAll
	assert.Empty(t, capture.capturedPrereqs)
}

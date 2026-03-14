// internal/app/scorer.go
package app

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

// Scorer re-computes prerequisites_met, age_boost, and total_score for all items every 60s.
type Scorer struct {
	items        port.ItemStore
	settings     port.SettingsStore
	integrations port.IntegrationStore
	interval     time.Duration
}

// NewScorer constructs a Scorer. interval defaults to 60s.
func NewScorer(items port.ItemStore, settings port.SettingsStore, integrations port.IntegrationStore) *Scorer {
	return &Scorer{items: items, settings: settings, integrations: integrations, interval: 60 * time.Second}
}

// Start runs the re-scorer goroutine until ctx is cancelled.
// Errors are logged but never cause the goroutine to exit.
func (s *Scorer) Start(ctx context.Context) {
	go func() {
		if err := s.RunOnce(ctx); err != nil {
			log.Printf("scorer: initial run error: %v", err)
		}
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.RunOnce(ctx); err != nil {
					log.Printf("scorer: run error: %v", err)
				}
			}
		}
	}()
}

// RunOnce runs one rescore cycle. Called by Start and by tests.
func (s *Scorer) RunOnce(ctx context.Context) error {
	all, err := s.settings.GetAll(ctx)
	if err != nil {
		return err
	}
	ageMultiplier, _ := strconv.ParseFloat(all[domain.SettingAgeMultiplier], 64)
	maxAgeBoost, _ := strconv.ParseFloat(all[domain.SettingMaxAgeBoost], 64)

	categoryWeights, err := s.settings.GetCategoryWeights(ctx)
	if err != nil {
		return err
	}
	weights := make(map[domain.ItemType]int, len(categoryWeights))
	for _, w := range categoryWeights {
		weights[w.ItemType] = w.Weight
	}

	// Collect all prerequisite usernames across all integrations.
	integrations, err := s.integrations.ListIntegrations(ctx)
	if err != nil {
		return err
	}
	prereqSet := map[string]struct{}{}
	for _, integration := range integrations {
		prereqs, err := s.integrations.ListPrerequisites(ctx, integration.ID)
		if err != nil {
			return err
		}
		for _, p := range prereqs {
			prereqSet[p.GitHubUsername] = struct{}{}
		}
	}
	prereqUsernames := make([]string, 0, len(prereqSet))
	for u := range prereqSet {
		prereqUsernames = append(prereqUsernames, u)
	}

	return s.items.RescoreAll(ctx, weights, prereqUsernames, ageMultiplier, maxAgeBoost)
}

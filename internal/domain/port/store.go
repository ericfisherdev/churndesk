// internal/domain/port/store.go
package port

import (
	"context"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
)

// ItemStore is the outbound port for persisting and querying feed items.
type ItemStore interface {
	Upsert(ctx context.Context, items []domain.Item) error
	ListRanked(ctx context.Context, limit int) ([]domain.Item, error)
	Count(ctx context.Context) (int, error)
	Dismiss(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	MarkSeen(ctx context.Context, id string) error
	MarkSeenByPR(ctx context.Context, prOwner, prRepo string, prNumber int) error
	MarkSeenByJiraKey(ctx context.Context, jiraKey string) error
	RescoreAll(ctx context.Context, weights map[domain.ItemType]int, prerequisiteUsernames []string, ageMultiplier, maxAgeBoost float64) error
}

// LinkStore is the outbound port for PR ↔ Jira issue relationships.
type LinkStore interface {
	UpsertPRJiraLinks(ctx context.Context, prOwner, prRepo string, prNumber int, prTitle string, jiraKeys []string) error
	GetJiraKeysForPR(ctx context.Context, prOwner, prRepo string, prNumber int) ([]string, error)
	GetPRsForJiraKey(ctx context.Context, jiraKey string) ([]domain.PRRef, error)
}

// IntegrationStore is the outbound port for integration configuration.
type IntegrationStore interface {
	CreateIntegration(ctx context.Context, i domain.Integration) (int, error)
	GetIntegration(ctx context.Context, id int) (*domain.Integration, error)
	UpdateIntegration(ctx context.Context, i domain.Integration) error
	DeleteIntegration(ctx context.Context, id int) error
	ListIntegrations(ctx context.Context) ([]domain.Integration, error)
	UpdateLastSyncedAt(ctx context.Context, id int, t time.Time) error

	CreateSpace(ctx context.Context, s domain.Space) (int, error)
	ListSpaces(ctx context.Context, integrationID int) ([]domain.Space, error)
	UpdateSpace(ctx context.Context, s domain.Space) error
	DeleteSpace(ctx context.Context, id int) error
	ReplaceSpaces(ctx context.Context, integrationID int, spaces []domain.Space) error

	CreateTeammate(ctx context.Context, t domain.Teammate) error
	ListTeammates(ctx context.Context, integrationID int) ([]domain.Teammate, error)
	DeleteTeammate(ctx context.Context, id int) error
	ReplaceTeammates(ctx context.Context, integrationID int, teammates []domain.Teammate) error

	CreatePrerequisite(ctx context.Context, p domain.ReviewPrerequisite) error
	ListPrerequisites(ctx context.Context, integrationID int) ([]domain.ReviewPrerequisite, error)
	DeletePrerequisite(ctx context.Context, id int) error
	ReplacePrerequisites(ctx context.Context, integrationID int, prereqs []domain.ReviewPrerequisite) error

	IsOnboardingComplete(ctx context.Context) (bool, error)
}

// SettingsStore is the outbound port for app settings and category weights.
type SettingsStore interface {
	Get(ctx context.Context, key domain.SettingKey) (string, error)
	Set(ctx context.Context, key domain.SettingKey, value string) error
	GetAll(ctx context.Context) (map[domain.SettingKey]string, error)
	GetCategoryWeights(ctx context.Context) ([]domain.CategoryWeight, error)
	SetCategoryWeight(ctx context.Context, itemType domain.ItemType, weight int) error
}

// internal/adapter/sqlite/integration_store_test.go
package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/churndesk/churndesk/internal/adapter/sqlite"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationStore_CreateAndGet(t *testing.T) {
	s := sqlite.NewIntegrationStore(openTestDB(t))
	ctx := context.Background()

	id, err := s.CreateIntegration(ctx, domain.Integration{
		Provider: domain.ProviderGitHub, AccessToken: "tok",
		Username: "alice", PollIntervalSeconds: 300, Enabled: true,
	})
	require.NoError(t, err)
	assert.Greater(t, id, 0)

	got, err := s.GetIntegration(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.ProviderGitHub, got.Provider)
	assert.Equal(t, "alice", got.Username)
	assert.Nil(t, got.LastSyncedAt)
}

func TestIntegrationStore_UpdateLastSyncedAt(t *testing.T) {
	s := sqlite.NewIntegrationStore(openTestDB(t))
	ctx := context.Background()

	id, _ := s.CreateIntegration(ctx, domain.Integration{Provider: domain.ProviderGitHub, Enabled: true})
	now := time.Now().Truncate(time.Second)
	require.NoError(t, s.UpdateLastSyncedAt(ctx, id, now))

	got, err := s.GetIntegration(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got.LastSyncedAt)
	assert.WithinDuration(t, now, *got.LastSyncedAt, time.Second)
}

func TestIntegrationStore_SpaceCRUD(t *testing.T) {
	s := sqlite.NewIntegrationStore(openTestDB(t))
	ctx := context.Background()

	intID, _ := s.CreateIntegration(ctx, domain.Integration{Provider: domain.ProviderGitHub, Enabled: true})
	spaceID, err := s.CreateSpace(ctx, domain.Space{
		IntegrationID: intID, Provider: domain.ProviderGitHub,
		Owner: "myorg", Name: "myrepo", Enabled: true,
	})
	require.NoError(t, err)
	assert.Greater(t, spaceID, 0)

	spaces, err := s.ListSpaces(ctx, intID)
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Equal(t, "myrepo", spaces[0].Name)
	assert.Empty(t, spaces[0].BoardType)

	require.NoError(t, s.DeleteSpace(ctx, spaceID))
	spaces, _ = s.ListSpaces(ctx, intID)
	assert.Empty(t, spaces)
}

func TestIntegrationStore_IsOnboardingComplete(t *testing.T) {
	s := sqlite.NewIntegrationStore(openTestDB(t))
	ctx := context.Background()

	ok, err := s.IsOnboardingComplete(ctx)
	require.NoError(t, err)
	assert.False(t, ok)

	intID, _ := s.CreateIntegration(ctx, domain.Integration{Provider: domain.ProviderGitHub, Enabled: true})
	ok, _ = s.IsOnboardingComplete(ctx)
	assert.False(t, ok)

	_, _ = s.CreateSpace(ctx, domain.Space{IntegrationID: intID, Provider: domain.ProviderGitHub, Name: "repo", Enabled: true})
	ok, err = s.IsOnboardingComplete(ctx)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIntegrationStore_TeammateAndPrerequisiteCRUD(t *testing.T) {
	s := sqlite.NewIntegrationStore(openTestDB(t))
	ctx := context.Background()

	intID, _ := s.CreateIntegration(ctx, domain.Integration{Provider: domain.ProviderGitHub, Enabled: true})

	require.NoError(t, s.CreateTeammate(ctx, domain.Teammate{IntegrationID: intID, GitHubUsername: "bob"}))
	teammates, err := s.ListTeammates(ctx, intID)
	require.NoError(t, err)
	require.Len(t, teammates, 1)
	assert.Equal(t, "bob", teammates[0].GitHubUsername)
	require.NoError(t, s.DeleteTeammate(ctx, teammates[0].ID))

	require.NoError(t, s.CreatePrerequisite(ctx, domain.ReviewPrerequisite{
		IntegrationID: intID, GitHubUsername: "copilot[bot]", DisplayName: "Copilot",
	}))
	prereqs, err := s.ListPrerequisites(ctx, intID)
	require.NoError(t, err)
	require.Len(t, prereqs, 1)
	assert.Equal(t, "copilot[bot]", prereqs[0].GitHubUsername)
}

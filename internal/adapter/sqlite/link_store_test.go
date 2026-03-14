// internal/adapter/sqlite/link_store_test.go
package sqlite_test

import (
	"context"
	"testing"

	"github.com/churndesk/churndesk/internal/adapter/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkStore_UpsertAndGetJiraKeys(t *testing.T) {
	s := sqlite.NewLinkStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 42, "Fix login", []string{"FRONT-1", "BACK-2"}))

	keys, err := s.GetJiraKeysForPR(ctx, "myorg", "myrepo", 42)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"FRONT-1", "BACK-2"}, keys)
}

func TestLinkStore_UpsertUpdatesPRTitle(t *testing.T) {
	s := sqlite.NewLinkStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 42, "Old title", []string{"FRONT-1"}))
	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 42, "New title", []string{"FRONT-1"}))

	prs, err := s.GetPRsForJiraKey(ctx, "FRONT-1")
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, "New title", prs[0].Title)
}

func TestLinkStore_GetPRsForJiraKey(t *testing.T) {
	s := sqlite.NewLinkStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 42, "Fix login", []string{"FRONT-1"}))
	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 99, "Fix timeout", []string{"FRONT-1"}))

	prs, err := s.GetPRsForJiraKey(ctx, "FRONT-1")
	require.NoError(t, err)
	assert.Len(t, prs, 2)
	numbers := []int{prs[0].Number, prs[1].Number}
	assert.ElementsMatch(t, []int{42, 99}, numbers)
}

func TestLinkStore_EmptyJiraKeys_IsNoOp(t *testing.T) {
	s := sqlite.NewLinkStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 42, "PR", []string{}))
	keys, err := s.GetJiraKeysForPR(ctx, "myorg", "myrepo", 42)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

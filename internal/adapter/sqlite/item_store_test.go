// internal/adapter/sqlite/item_store_test.go
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

func makeItem(id string, score float64, itemType domain.ItemType) domain.Item {
	return domain.Item{
		ID: id, Source: "github", Type: itemType,
		ExternalID: "42", Title: "Test PR",
		BaseScore: int(score), TotalScore: score, PrerequisitesMet: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func TestItemStore_UpsertAndListRanked(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, []domain.Item{
		makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded),
		makeItem("github:ci_failing:2", 90, domain.ItemTypePRCIFailing),
	}))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	require.Len(t, ranked, 2)
	assert.Equal(t, "github:ci_failing:2", ranked[0].ID)
	assert.Equal(t, "github:review_needed:1", ranked[1].ID)
}

func TestItemStore_DismissExcludesFromFeed(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, []domain.Item{makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded)}))
	require.NoError(t, s.Dismiss(ctx, "github:review_needed:1"))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, ranked)
}

func TestItemStore_Delete(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, []domain.Item{makeItem("github:approved:1", 30, domain.ItemTypePRApproved)}))
	require.NoError(t, s.Delete(ctx, "github:approved:1"))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, ranked)
}

func TestItemStore_Count(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, []domain.Item{
		makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded),
		makeItem("github:review_needed:2", 60, domain.ItemTypePRReviewNeeded),
	}))
	require.NoError(t, s.Dismiss(ctx, "github:review_needed:1"))

	count, err := s.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestItemStore_MarkSeenByPR(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	items := []domain.Item{
		{ID: "github:review_needed:42", Source: "github", Type: domain.ItemTypePRReviewNeeded,
			ExternalID: "42", PROwner: "myorg", PRRepo: "myrepo", Title: "PR",
			TotalScore: 60, PrerequisitesMet: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "github:comment:42", Source: "github", Type: domain.ItemTypePRNewComment,
			ExternalID: "42", PROwner: "myorg", PRRepo: "myrepo", Title: "PR comment",
			TotalScore: 50, PrerequisitesMet: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "github:review_needed:99", Source: "github", Type: domain.ItemTypePRReviewNeeded,
			ExternalID: "99", PROwner: "myorg", PRRepo: "myrepo", Title: "Other PR",
			TotalScore: 60, PrerequisitesMet: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	require.NoError(t, s.Upsert(ctx, items))
	require.NoError(t, s.MarkSeenByPR(ctx, "myorg", "myrepo", 42))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	seenMap := map[string]int{}
	for _, it := range ranked {
		seenMap[it.ID] = it.Seen
	}
	assert.Equal(t, 1, seenMap["github:review_needed:42"])
	assert.Equal(t, 1, seenMap["github:comment:42"])
	assert.Equal(t, 0, seenMap["github:review_needed:99"])
}

func TestItemStore_MarkSeenByJiraKey(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	items := []domain.Item{
		{ID: "jira:status_change:FRONT-441", Source: "jira", Type: domain.ItemTypeJiraStatusChange,
			ExternalID: "FRONT-441", Title: "Status", TotalScore: 40, PrerequisitesMet: 1,
			CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "jira:comment:FRONT-441", Source: "jira", Type: domain.ItemTypeJiraComment,
			ExternalID: "FRONT-441", Title: "Comment", TotalScore: 40, PrerequisitesMet: 1,
			CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "jira:new_bug:FRONT-999", Source: "jira", Type: domain.ItemTypeJiraNewBug,
			ExternalID: "FRONT-999", Title: "Bug", TotalScore: 70, PrerequisitesMet: 1,
			CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	require.NoError(t, s.Upsert(ctx, items))
	require.NoError(t, s.MarkSeenByJiraKey(ctx, "FRONT-441"))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	seenMap := map[string]int{}
	for _, it := range ranked {
		seenMap[it.ID] = it.Seen
	}
	assert.Equal(t, 1, seenMap["jira:status_change:FRONT-441"])
	assert.Equal(t, 1, seenMap["jira:comment:FRONT-441"])
	assert.Equal(t, 0, seenMap["jira:new_bug:FRONT-999"])
}

func TestItemStore_RescoreAll_NoPrerequisites(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	old := makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded)
	old.CreatedAt = time.Now().Add(-2 * time.Hour)
	old.UpdatedAt = old.CreatedAt
	old.Metadata = `{"reviews":[]}`
	require.NoError(t, s.Upsert(ctx, []domain.Item{old}))

	weights := map[domain.ItemType]int{domain.ItemTypePRReviewNeeded: 60}
	require.NoError(t, s.RescoreAll(ctx, weights, []string{}, 0.5, 50))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	require.Len(t, ranked, 1)
	assert.Equal(t, 1, ranked[0].PrerequisitesMet)
	assert.Equal(t, 60, ranked[0].BaseScore)
	assert.InDelta(t, 1.0, ranked[0].AgeBoost, 0.1)
	assert.InDelta(t, 61.0, ranked[0].TotalScore, 0.1)
}

func TestItemStore_RescoreAll_PrerequisitesNotApproved(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	old := makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded)
	old.CreatedAt = time.Now().Add(-10 * time.Hour)
	old.UpdatedAt = old.CreatedAt
	old.Metadata = `{"reviews":[{"login":"copilot[bot]","state":"CHANGES_REQUESTED"}]}`
	require.NoError(t, s.Upsert(ctx, []domain.Item{old}))

	weights := map[domain.ItemType]int{domain.ItemTypePRReviewNeeded: 60}
	require.NoError(t, s.RescoreAll(ctx, weights, []string{"copilot[bot]"}, 0.5, 50))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	require.Len(t, ranked, 1)
	assert.Equal(t, 0, ranked[0].PrerequisitesMet)
	assert.InDelta(t, 0.0, ranked[0].AgeBoost, 0.001)
	assert.InDelta(t, 60.0, ranked[0].TotalScore, 0.001)
}

func TestItemStore_RescoreAll_PrerequisitesMet(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	old := makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded)
	old.CreatedAt = time.Now().Add(-4 * time.Hour)
	old.UpdatedAt = old.CreatedAt
	old.Metadata = `{"reviews":[{"login":"copilot[bot]","state":"APPROVED"}]}`
	require.NoError(t, s.Upsert(ctx, []domain.Item{old}))

	weights := map[domain.ItemType]int{domain.ItemTypePRReviewNeeded: 60}
	require.NoError(t, s.RescoreAll(ctx, weights, []string{"copilot[bot]"}, 0.5, 50))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	require.Len(t, ranked, 1)
	assert.Equal(t, 1, ranked[0].PrerequisitesMet)
	assert.InDelta(t, 2.0, ranked[0].AgeBoost, 0.1)
	assert.InDelta(t, 62.0, ranked[0].TotalScore, 0.1)
}

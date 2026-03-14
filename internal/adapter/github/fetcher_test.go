// internal/adapter/github/fetcher_test.go
package github_test

import (
	"context"
	"testing"
	"time"

	ghadapter "github.com/churndesk/churndesk/internal/adapter/github"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGitHubClient is a minimal mock of port.GitHubClient for fetcher tests.
type mockGitHubClient struct {
	prs      []*domain.PRDetail
	reviews  []domain.Review
	checks   []domain.CheckRun
	comments []domain.Comment
}

func (m *mockGitHubClient) ListPRsForRepo(ctx context.Context, owner, repo string) ([]*domain.PRDetail, error) {
	return m.prs, nil
}
func (m *mockGitHubClient) ListPRReviews(ctx context.Context, owner, repo string, number int) ([]domain.Review, error) {
	return m.reviews, nil
}
func (m *mockGitHubClient) ListCheckRuns(ctx context.Context, owner, repo, headSHA string) ([]domain.CheckRun, error) {
	return m.checks, nil
}
func (m *mockGitHubClient) ListPRComments(ctx context.Context, owner, repo string, number int) ([]domain.Comment, error) {
	return m.comments, nil
}
func (m *mockGitHubClient) GetPR(ctx context.Context, owner, repo string, number int) (*domain.PRDetail, error) {
	return nil, nil
}
func (m *mockGitHubClient) PostPRComment(ctx context.Context, owner, repo string, number int, body string) error {
	return nil
}
func (m *mockGitHubClient) SubmitReview(ctx context.Context, owner, repo string, number int, verdict, body string) error {
	return nil
}
func (m *mockGitHubClient) RequestReviewers(ctx context.Context, owner, repo string, number int, logins []string) error {
	return nil
}

var _ port.GitHubClient = (*mockGitHubClient)(nil)

func TestGitHubFetcher_PRReviewNeeded(t *testing.T) {
	// Teammate "bob" has an open PR; authenticated user is "alice" — should surface pr_review_needed
	client := &mockGitHubClient{
		prs: []*domain.PRDetail{
			{Number: 42, Title: "Fix login", Owner: "myorg", Repo: "myrepo",
				Author: "bob", HeadSHA: "abc123", State: "open",
				CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		reviews: []domain.Review{}, // no reviews yet
		checks:  []domain.CheckRun{},
	}
	integration := domain.Integration{Provider: domain.ProviderGitHub, Username: "alice"}
	spaces := []domain.Space{{Owner: "myorg", Name: "myrepo", Enabled: true}}
	teammates := []domain.Teammate{{GitHubUsername: "bob"}}

	fetcher := ghadapter.NewFetcher(client, "alice", teammates, 1)
	items, err := fetcher.Fetch(context.Background(), integration, spaces)
	require.NoError(t, err)

	var reviewNeeded []domain.Item
	for _, it := range items {
		if it.Type == domain.ItemTypePRReviewNeeded {
			reviewNeeded = append(reviewNeeded, it)
		}
	}
	require.Len(t, reviewNeeded, 1)
	assert.Equal(t, "github:review_needed:myorg/myrepo:42", reviewNeeded[0].ID)
	assert.Equal(t, "42", reviewNeeded[0].ExternalID)
	assert.Equal(t, "myorg", reviewNeeded[0].PROwner)
	assert.Equal(t, "myrepo", reviewNeeded[0].PRRepo)
}

func TestGitHubFetcher_OwnPRWithCIFailing(t *testing.T) {
	// Authenticated user "alice" has an open PR with a failing check
	client := &mockGitHubClient{
		prs: []*domain.PRDetail{
			{Number: 99, Title: "My feature", Owner: "myorg", Repo: "myrepo",
				Author: "alice", HeadSHA: "def456", State: "open",
				CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		reviews: []domain.Review{},
		checks: []domain.CheckRun{
			{Name: "CI", Status: "completed", Conclusion: "failure"},
		},
	}
	integration := domain.Integration{Provider: domain.ProviderGitHub, Username: "alice"}
	spaces := []domain.Space{{Owner: "myorg", Name: "myrepo", Enabled: true}}

	fetcher := ghadapter.NewFetcher(client, "alice", nil, 1)
	items, err := fetcher.Fetch(context.Background(), integration, spaces)
	require.NoError(t, err)

	var ciFailing []domain.Item
	for _, it := range items {
		if it.Type == domain.ItemTypePRCIFailing {
			ciFailing = append(ciFailing, it)
		}
	}
	require.Len(t, ciFailing, 1)
	assert.Equal(t, "github:ci_failing:myorg/myrepo:99", ciFailing[0].ID)
}

func TestGitHubFetcher_SkipsOwnPRsForReviewNeeded(t *testing.T) {
	// Alice is both the teammate and PR author — should NOT surface review_needed for own PRs
	client := &mockGitHubClient{
		prs: []*domain.PRDetail{
			{Number: 42, Title: "My PR", Owner: "myorg", Repo: "myrepo",
				Author: "alice", HeadSHA: "abc", State: "open",
				CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		reviews: []domain.Review{},
		checks:  []domain.CheckRun{},
	}
	integration := domain.Integration{Provider: domain.ProviderGitHub, Username: "alice"}
	spaces := []domain.Space{{Owner: "myorg", Name: "myrepo", Enabled: true}}
	teammates := []domain.Teammate{{GitHubUsername: "alice"}} // alice is a teammate

	fetcher := ghadapter.NewFetcher(client, "alice", teammates, 1)
	items, err := fetcher.Fetch(context.Background(), integration, spaces)
	require.NoError(t, err)

	for _, it := range items {
		assert.NotEqual(t, domain.ItemTypePRReviewNeeded, it.Type,
			"own PRs should not surface as review_needed")
	}
}

func TestGitHubFetcher_PRNewComment_ExcludesSelfAuthored(t *testing.T) {
	// Own PR with a self-authored comment and an other-authored comment — only other-authored triggers pr_new_comment
	lastSync := time.Now().Add(-1 * time.Hour)
	client := &mockGitHubClient{
		prs: []*domain.PRDetail{
			{Number: 7, Title: "My feature", Owner: "myorg", Repo: "myrepo",
				Author: "alice", HeadSHA: "sha1", State: "open",
				CreatedAt: time.Now().Add(-2 * time.Hour), UpdatedAt: time.Now()},
		},
		reviews: []domain.Review{},
		checks:  []domain.CheckRun{},
		comments: []domain.Comment{
			{ID: 1, Author: "alice", Body: "I pushed a fix", CreatedAt: time.Now()},  // self-authored — must be excluded
			{ID: 2, Author: "bob", Body: "Looks good to me", CreatedAt: time.Now()},  // other-authored — must trigger
		},
	}
	integration := domain.Integration{Provider: domain.ProviderGitHub, Username: "alice", LastSyncedAt: &lastSync}
	spaces := []domain.Space{{Owner: "myorg", Name: "myrepo", Enabled: true}}

	fetcher := ghadapter.NewFetcher(client, "alice", nil, 1)
	items, err := fetcher.Fetch(context.Background(), integration, spaces)
	require.NoError(t, err)

	var newComments []domain.Item
	for _, it := range items {
		if it.Type == domain.ItemTypePRNewComment {
			newComments = append(newComments, it)
		}
	}
	require.Len(t, newComments, 1, "should have exactly one pr_new_comment item")
	assert.Equal(t, "github:comment:myorg/myrepo:7", newComments[0].ID)
}

func TestGitHubFetcher_PRNewComment_SuppressedOnFirstSync(t *testing.T) {
	// First sync (LastSyncedAt == nil) — pr_new_comment must NOT be generated even with comments
	client := &mockGitHubClient{
		prs: []*domain.PRDetail{
			{Number: 5, Title: "Feature", Owner: "myorg", Repo: "myrepo",
				Author: "alice", HeadSHA: "sha2", State: "open",
				CreatedAt: time.Now().Add(-2 * time.Hour), UpdatedAt: time.Now()},
		},
		reviews: []domain.Review{},
		checks:  []domain.CheckRun{},
		comments: []domain.Comment{
			{ID: 3, Author: "bob", Body: "LGTM", CreatedAt: time.Now()},
		},
	}
	integration := domain.Integration{Provider: domain.ProviderGitHub, Username: "alice", LastSyncedAt: nil}
	spaces := []domain.Space{{Owner: "myorg", Name: "myrepo", Enabled: true}}

	fetcher := ghadapter.NewFetcher(client, "alice", nil, 1)
	items, err := fetcher.Fetch(context.Background(), integration, spaces)
	require.NoError(t, err)

	for _, it := range items {
		assert.NotEqual(t, domain.ItemTypePRNewComment, it.Type,
			"pr_new_comment must be suppressed on first sync")
	}
}

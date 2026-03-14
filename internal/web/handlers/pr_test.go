package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/web/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGitHubClient struct {
	pr       *domain.PRDetail
	reviews  []domain.Review
	checks   []domain.CheckRun
	comments []domain.Comment
}

func (m *mockGitHubClient) GetPR(_ context.Context, _, _ string, _ int) (*domain.PRDetail, error) {
	return m.pr, nil
}
func (m *mockGitHubClient) ListPRComments(_ context.Context, _, _ string, _ int) ([]domain.Comment, error) {
	return m.comments, nil
}
func (m *mockGitHubClient) ListPRReviews(_ context.Context, _, _ string, _ int) ([]domain.Review, error) {
	return m.reviews, nil
}
func (m *mockGitHubClient) ListCheckRuns(_ context.Context, _, _, _ string) ([]domain.CheckRun, error) {
	return m.checks, nil
}
func (m *mockGitHubClient) PostPRComment(_ context.Context, _, _ string, _ int, _ string) error {
	return nil
}
func (m *mockGitHubClient) SubmitReview(_ context.Context, _, _ string, _ int, _, _ string) error {
	return nil
}
func (m *mockGitHubClient) RequestReviewers(_ context.Context, _, _ string, _ int, _ []string) error {
	return nil
}
func (m *mockGitHubClient) ListPRsForRepo(_ context.Context, _, _ string) ([]*domain.PRDetail, error) {
	return nil, nil
}

type stubPRItemStore struct{ seenPRs []string }

func (s *stubPRItemStore) MarkSeenByPR(_ context.Context, owner, repo string, number int) error {
	s.seenPRs = append(s.seenPRs, fmt.Sprintf("%s/%s/%d", owner, repo, number))
	return nil
}

type stubLinkStore struct {
	jiraKeys []string
	prRefs   []domain.PRRef
}

func (s *stubLinkStore) GetJiraKeysForPR(_ context.Context, _, _ string, _ int) ([]string, error) {
	return s.jiraKeys, nil
}
func (s *stubLinkStore) GetPRsForJiraKey(_ context.Context, _ string) ([]domain.PRRef, error) {
	return s.prRefs, nil
}
func (s *stubLinkStore) UpsertPRJiraLinks(_ context.Context, _, _ string, _ int, _ string, _ []string) error {
	return nil
}

type stubPRIntegrationStore struct{ teammates []domain.Teammate }

func (s *stubPRIntegrationStore) ListTeammates(_ context.Context, _ int) ([]domain.Teammate, error) {
	return s.teammates, nil
}
func (s *stubPRIntegrationStore) ListIntegrations(_ context.Context) ([]domain.Integration, error) {
	return []domain.Integration{{ID: 1, Provider: domain.ProviderGitHub, Username: "alice", Enabled: true}}, nil
}

func newTestPRHandler(ghClient *mockGitHubClient, items *stubPRItemStore, links *stubLinkStore, integrations *stubPRIntegrationStore) *handlers.PRHandler {
	return handlers.NewPRHandler(ghClient, items, links, integrations, "alice")
}

func TestPRHandler_Page_Renders(t *testing.T) {
	client := &mockGitHubClient{
		pr: &domain.PRDetail{
			Number: 42, Title: "Fix auth", Owner: "myorg", Repo: "myrepo",
			Author: "bob", HeadSHA: "sha1", State: "open",
			Body: "**Fixes** the login timeout.",
			Branch: "fix/auth", BaseBranch: "main",
			CreatedAt: time.Now().Add(-2 * time.Hour), UpdatedAt: time.Now(),
		},
		reviews:  []domain.Review{},
		checks:   []domain.CheckRun{},
		comments: []domain.Comment{},
	}
	items := &stubPRItemStore{}
	links := &stubLinkStore{}
	integrations := &stubPRIntegrationStore{}

	h := newTestPRHandler(client, items, links, integrations)
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/prs/myorg/myrepo/42", nil)
	req.SetPathValue("owner", "myorg")
	req.SetPathValue("repo", "myrepo")
	req.SetPathValue("number", "42")
	rec := httptest.NewRecorder()
	h.Page(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Fix auth")
}

func TestPRHandler_Page_MarksSeen(t *testing.T) {
	client := &mockGitHubClient{
		pr: &domain.PRDetail{
			Number: 7, Title: "Test", Owner: "myorg", Repo: "myrepo",
			Author: "bob", HeadSHA: "sha", State: "open",
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
		reviews: []domain.Review{}, checks: []domain.CheckRun{}, comments: []domain.Comment{},
	}
	items := &stubPRItemStore{}
	h := newTestPRHandler(client, items, &stubLinkStore{}, &stubPRIntegrationStore{})

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/prs/myorg/myrepo/7", nil)
	req.SetPathValue("owner", "myorg")
	req.SetPathValue("repo", "myrepo")
	req.SetPathValue("number", "7")
	h.Page(httptest.NewRecorder(), req)

	assert.Len(t, items.seenPRs, 1)
}

func TestPRHandler_PostComment_ReturnsPartial(t *testing.T) {
	client := &mockGitHubClient{}
	h := newTestPRHandler(client, &stubPRItemStore{}, &stubLinkStore{}, &stubPRIntegrationStore{})

	req := httptest.NewRequestWithContext(context.Background(), "POST", "/prs/myorg/myrepo/42/comments", nil)
	req.SetPathValue("owner", "myorg")
	req.SetPathValue("repo", "myrepo")
	req.SetPathValue("number", "42")
	req.Form = map[string][]string{"body": {"Hello!"}}
	rec := httptest.NewRecorder()
	h.PostComment(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "<!DOCTYPE html>", "PostComment must return a partial, not a full page")
}

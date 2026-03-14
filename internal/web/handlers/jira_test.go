package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/web/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockJiraClient struct {
	issue    *domain.JiraIssue
	comments []domain.Comment
}

func (m *mockJiraClient) GetIssue(_ context.Context, _ string) (*domain.JiraIssue, error) {
	return m.issue, nil
}
func (m *mockJiraClient) ListIssueComments(_ context.Context, _ string) ([]domain.Comment, error) {
	return m.comments, nil
}
func (m *mockJiraClient) PostComment(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockJiraClient) SearchIssues(_ context.Context, _ string) ([]*domain.JiraIssue, error) {
	return nil, nil
}
func (m *mockJiraClient) ListBoards(_ context.Context, _, _ string) ([]*domain.Board, error) {
	return nil, nil
}
func (m *mockJiraClient) GetActiveSprintIssues(_ context.Context, _ int) ([]*domain.JiraIssue, error) {
	return nil, nil
}

type stubJiraItemStore struct{ seenKeys []string }

func (s *stubJiraItemStore) MarkSeenByJiraKey(_ context.Context, key string) error {
	s.seenKeys = append(s.seenKeys, key)
	return nil
}

type stubJiraLinkStore struct{ prs []domain.PRRef }

func (s *stubJiraLinkStore) GetPRsForJiraKey(_ context.Context, _ string) ([]domain.PRRef, error) {
	return s.prs, nil
}
func (s *stubJiraLinkStore) GetJiraKeysForPR(_ context.Context, _, _ string, _ int) ([]string, error) {
	return nil, nil
}
func (s *stubJiraLinkStore) UpsertPRJiraLinks(_ context.Context, _, _ string, _ int, _ string, _ []string) error {
	return nil
}

func TestJiraHandler_Page_Renders(t *testing.T) {
	client := &mockJiraClient{
		issue: &domain.JiraIssue{
			Key: "FRONT-441", Summary: "Fix login timeout",
			Status: "In Progress", Priority: "High", IssueType: "Bug",
			Assignee: "account-id", Description: "**Timeout** after 30s.",
			CreatedAt: time.Now().Add(-24 * time.Hour), UpdatedAt: time.Now(),
		},
		comments: []domain.Comment{},
	}
	items := &stubJiraItemStore{}
	links := &stubJiraLinkStore{}

	h := handlers.NewJiraHandler(client, items, links)
	req := httptest.NewRequest("GET", "/jira/FRONT-441", nil)
	req.SetPathValue("key", "FRONT-441")
	rec := httptest.NewRecorder()
	h.Page(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "FRONT-441")
}

func TestJiraHandler_Page_MarksSeen(t *testing.T) {
	client := &mockJiraClient{
		issue: &domain.JiraIssue{
			Key: "BACK-12", Summary: "DB crash",
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
	}
	items := &stubJiraItemStore{}

	h := handlers.NewJiraHandler(client, items, &stubJiraLinkStore{})
	req := httptest.NewRequest("GET", "/jira/BACK-12", nil)
	req.SetPathValue("key", "BACK-12")
	h.Page(httptest.NewRecorder(), req)

	assert.Equal(t, []string{"BACK-12"}, items.seenKeys)
}

func TestJiraHandler_PostComment_ReturnsPartial(t *testing.T) {
	client := &mockJiraClient{}
	h := handlers.NewJiraHandler(client, &stubJiraItemStore{}, &stubJiraLinkStore{})

	req := httptest.NewRequest("POST", "/jira/FRONT-441/comments", nil)
	req.SetPathValue("key", "FRONT-441")
	req.Form = map[string][]string{"body": {"Reproduced on staging."}}
	rec := httptest.NewRecorder()
	h.PostComment(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "<!DOCTYPE html>", "PostComment must return a partial")
}

// internal/adapter/jira/fetcher_test.go
package jira_test

import (
	"context"
	"testing"
	"time"

	jiradapter "github.com/churndesk/churndesk/internal/adapter/jira"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockJiraClient struct {
	issues   []*domain.JiraIssue
	boards   []*domain.Board
	comments []domain.Comment
}

func (m *mockJiraClient) GetIssue(ctx context.Context, key string) (*domain.JiraIssue, error) {
	for _, i := range m.issues {
		if i.Key == key {
			return i, nil
		}
	}
	return nil, nil
}
func (m *mockJiraClient) ListIssueComments(ctx context.Context, key string) ([]domain.Comment, error) {
	return m.comments, nil
}
func (m *mockJiraClient) PostComment(ctx context.Context, key, body string) error { return nil }
func (m *mockJiraClient) SearchIssues(ctx context.Context, jql string) ([]*domain.JiraIssue, error) {
	return m.issues, nil
}
func (m *mockJiraClient) ListBoards(ctx context.Context, key, boardType string) ([]*domain.Board, error) {
	return m.boards, nil
}
func (m *mockJiraClient) GetActiveSprintIssues(ctx context.Context, boardID int) ([]*domain.JiraIssue, error) {
	return m.issues, nil
}

var _ port.JiraClient = (*mockJiraClient)(nil)

func TestJiraFetcher_NewBug(t *testing.T) {
	// A new bug created after last sync (integration has synced before)
	lastSync := time.Now().Add(-1 * time.Hour)
	client := &mockJiraClient{
		issues: []*domain.JiraIssue{
			{Key: "FRONT-441", Summary: "Login bug", IssueType: "Bug", Status: "To Do",
				CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		boards: []*domain.Board{{ID: 1, Type: "kanban"}},
	}
	integration := domain.Integration{Provider: domain.ProviderJira, Username: "account-id", LastSyncedAt: &lastSync}
	spaces := []domain.Space{{Owner: "FRONT", Name: "Frontend", Provider: domain.ProviderJira, BoardType: "kanban", JiraBoardID: 1, Enabled: true}}

	fetcher := jiradapter.NewFetcher(client, "account-id")
	items, err := fetcher.Fetch(context.Background(), integration, spaces)
	require.NoError(t, err)

	var bugs []domain.Item
	for _, it := range items {
		if it.Type == domain.ItemTypeJiraNewBug {
			bugs = append(bugs, it)
		}
	}
	require.Len(t, bugs, 1)
	assert.Equal(t, "jira:new_bug:FRONT-441", bugs[0].ID)
	assert.Equal(t, "FRONT-441", bugs[0].ExternalID)
}

func TestJiraFetcher_FirstSync_SuppressesAllItemTypes(t *testing.T) {
	// First sync (LastSyncedAt == nil) — NO Jira items of any type should be generated.
	// Covers: jira_status_change, jira_new_bug, jira_comment (spec §4.2 first-sync suppression).
	client := &mockJiraClient{
		issues: []*domain.JiraIssue{
			{Key: "FRONT-441", Summary: "Login bug", IssueType: "Bug", Status: "In Progress",
				Assignee: "account-id", CreatedAt: time.Now(), UpdatedAt: time.Now(),
				Comments: []domain.Comment{
					{ID: 10, Author: "bob", Body: "Found another case", CreatedAt: time.Now()},
				}},
		},
		boards: []*domain.Board{{ID: 1, Type: "kanban"}},
	}
	integration := domain.Integration{Provider: domain.ProviderJira, Username: "account-id", LastSyncedAt: nil}
	spaces := []domain.Space{{Owner: "FRONT", Name: "Frontend", Provider: domain.ProviderJira, BoardType: "kanban", JiraBoardID: 1, Enabled: true}}

	fetcher := jiradapter.NewFetcher(client, "account-id")
	items, err := fetcher.Fetch(context.Background(), integration, spaces)
	require.NoError(t, err)

	assert.Empty(t, items, "all Jira item types must be suppressed on first sync (lastSyncedAt == nil)")
}

// internal/adapter/jira/fetcher.go
package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

// Fetcher implements port.Fetcher for Jira. It depends on port.JiraClient,
// not the concrete adapter, so it can be tested with a mock.
type Fetcher struct {
	client    port.JiraClient
	accountID string // authenticated user's Jira account ID
}

// NewFetcher constructs a Jira Fetcher.
func NewFetcher(client port.JiraClient, accountID string) *Fetcher {
	return &Fetcher{client: client, accountID: accountID}
}

// Fetch implements port.Fetcher.
// integration.LastSyncedAt == nil on first sync — all item types are suppressed.
func (f *Fetcher) Fetch(ctx context.Context, integration domain.Integration, spaces []domain.Space) ([]domain.Item, error) {
	var items []domain.Item
	for _, space := range spaces {
		if !space.Enabled {
			continue
		}
		issues, err := f.fetchIssues(ctx, space)
		if err != nil {
			return nil, fmt.Errorf("fetch issues for space %s: %w", space.Owner, err)
		}
		for _, issue := range issues {
			fetched := f.processIssue(issue, integration.LastSyncedAt)
			items = append(items, fetched...)
		}
	}
	return items, nil
}

func (f *Fetcher) fetchIssues(ctx context.Context, space domain.Space) ([]*domain.JiraIssue, error) {
	switch space.BoardType {
	case "scrum":
		return f.client.GetActiveSprintIssues(ctx, space.JiraBoardID)
	case "kanban":
		return f.client.SearchIssues(ctx, fmt.Sprintf("project = %s AND statusCategory != Done", space.Owner))
	default:
		return nil, fmt.Errorf("unknown board type %q for space %s — ensure board detection ran", space.BoardType, space.Owner)
	}
}

func (f *Fetcher) processIssue(issue *domain.JiraIssue, lastSyncedAt *time.Time) []domain.Item {
	var items []domain.Item
	metadata := buildJiraMetadata(issue)

	// All item types are suppressed on first sync (lastSyncedAt == nil).
	if lastSyncedAt == nil {
		return items
	}

	// jira_status_change: assigned to me.
	// Note: the item is upserted on every sync while assigned to me. Dismissal handles resolution.
	if issue.Assignee == f.accountID {
		items = append(items, domain.Item{
			ID:         "jira:status_change:" + issue.Key,
			Source:     "jira",
			Type:       domain.ItemTypeJiraStatusChange,
			ExternalID: issue.Key,
			Title:      "Status update: " + issue.Summary,
			Metadata:   metadata,
		})
	}

	// jira_new_bug: Bug issue type created after last sync.
	if issue.IssueType == "Bug" && issue.CreatedAt.After(*lastSyncedAt) {
		items = append(items, domain.Item{
			ID:         "jira:new_bug:" + issue.Key,
			Source:     "jira",
			Type:       domain.ItemTypeJiraNewBug,
			ExternalID: issue.Key,
			Title:      "New bug: " + issue.Summary,
			Metadata:   metadata,
		})
	}

	// jira_comment: new comment on issue where I authored at least one comment.
	if iHaveCommented(issue.Comments, f.accountID) {
		if newest := firstNewCommentFrom(issue.Comments, f.accountID, *lastSyncedAt); newest != nil {
			commentItem := domain.Item{
				ID:         "jira:comment:" + issue.Key,
				Source:     "jira",
				Type:       domain.ItemTypeJiraComment,
				ExternalID: issue.Key,
				Title:      "New comment: " + issue.Summary,
				Metadata:   buildCommentJiraMetadata(metadata, newest),
				CreatedAt:  newest.CreatedAt,
				UpdatedAt:  newest.CreatedAt,
			}
			items = append(items, commentItem)
		}
	}

	// Set URL and timestamps on all items
	for i := range items {
		if items[i].URL == "" {
			items[i].URL = "/jira/" + issue.Key
		}
		if items[i].CreatedAt.IsZero() {
			items[i].CreatedAt = issue.CreatedAt
		}
		if items[i].UpdatedAt.IsZero() {
			items[i].UpdatedAt = issue.UpdatedAt
		}
	}
	return items
}

func buildJiraMetadata(issue *domain.JiraIssue) string {
	type meta struct {
		Key         string  `json:"key"`
		Summary     string  `json:"summary"`
		Status      string  `json:"status"`
		Priority    string  `json:"priority"`
		Assignee    string  `json:"assignee"`
		IssueType   string  `json:"issue_type"`
		Sprint      string  `json:"sprint"`
		StoryPoints float64 `json:"story_points"`
	}
	b, _ := json.Marshal(meta{
		Key: issue.Key, Summary: issue.Summary, Status: issue.Status,
		Priority: issue.Priority, Assignee: issue.Assignee, IssueType: issue.IssueType,
		Sprint: issue.Sprint, StoryPoints: issue.StoryPoints,
	})
	if b == nil {
		return "{}"
	}
	return string(b)
}

// iHaveCommented reports whether accountID has authored any comment in the slice.
func iHaveCommented(comments []domain.Comment, accountID string) bool {
	for _, c := range comments {
		if c.Author == accountID {
			return true
		}
	}
	return false
}

// firstNewCommentFrom returns the first comment not authored by accountID posted after since,
// or nil if none exists.
func firstNewCommentFrom(comments []domain.Comment, accountID string, since time.Time) *domain.Comment {
	for i := range comments {
		if comments[i].Author != accountID && comments[i].CreatedAt.After(since) {
			return &comments[i]
		}
	}
	return nil
}

func buildCommentJiraMetadata(base string, c *domain.Comment) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(base), &m); err != nil {
		return base
	}
	m["latest_comment"] = c.Body
	m["latest_comment_author"] = c.Author
	m["latest_comment_at"] = c.CreatedAt.Format(time.RFC3339)
	b, _ := json.Marshal(m)
	return string(b)
}

// Ensure Fetcher implements port.Fetcher at compile time.
var _ port.Fetcher = (*Fetcher)(nil)

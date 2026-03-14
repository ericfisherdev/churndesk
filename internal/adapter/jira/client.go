// internal/adapter/jira/client.go
package jira

import (
	"context"
	"fmt"
	"net/http"
	"time"

	goatlassian "github.com/ctreminiom/go-atlassian/jira/agile"
	jirav3 "github.com/ctreminiom/go-atlassian/jira/v3"
	atlassianmodels "github.com/ctreminiom/go-atlassian/pkg/infra/models"

	"github.com/churndesk/churndesk/internal/domain"
)

// Client wraps go-atlassian REST v3 and Agile clients and translates their types to domain types.
// It implements port.JiraClient — no caller imports go-atlassian types directly.
type Client struct {
	rest      *jirav3.Client
	agile     *goatlassian.Client
	accountID string
}

// NewClient constructs a Jira adapter using basic auth (email + API token).
// Returns *Client which satisfies port.JiraClient.
func NewClient(baseURL, email, token, accountID string) *Client {
	httpClient := &http.Client{Timeout: 30 * time.Second}

	rest, err := jirav3.New(httpClient, baseURL)
	if err != nil {
		// Construction failure is a programmer error (invalid URL); panic to surface immediately.
		panic(fmt.Sprintf("jira adapter: failed to create REST v3 client: %v", err))
	}
	rest.Auth.SetBasicAuth(email, token)

	agile, err := goatlassian.New(httpClient, baseURL)
	if err != nil {
		panic(fmt.Sprintf("jira adapter: failed to create Agile client: %v", err))
	}
	agile.Auth.SetBasicAuth(email, token)

	return &Client{
		rest:      rest,
		agile:     agile,
		accountID: accountID,
	}
}

// GetIssue fetches a single Jira issue by key and maps it to domain.JiraIssue.
//
// Returns domain.ErrNotFound (wrapped) when the issue does not exist (404).
func (c *Client) GetIssue(ctx context.Context, key string) (*domain.JiraIssue, error) {
	issue, _, err := c.rest.Issue.Get(ctx, key, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get jira issue %s: %w", key, err)
	}
	return issueSchemeToJiraIssue(issue), nil
}

// ListIssueComments retrieves all comments for a Jira issue.
//
// Returns an empty slice when there are no comments.
func (c *Client) ListIssueComments(ctx context.Context, key string) ([]domain.Comment, error) {
	page, _, err := c.rest.Issue.Comment.Gets(ctx, key, "created", nil, 0, 100)
	if err != nil {
		return nil, fmt.Errorf("list comments for jira issue %s: %w", key, err)
	}
	return commentSchemesToDomain(page), nil
}

// PostComment adds a plain-text comment to a Jira issue using the ADF document format.
func (c *Client) PostComment(ctx context.Context, key string, body string) error {
	payload := &atlassianmodels.CommentPayloadScheme{
		Body: plainTextADFNode(body),
	}
	_, _, err := c.rest.Issue.Comment.Add(ctx, key, payload, nil)
	if err != nil {
		return fmt.Errorf("post comment on jira issue %s: %w", key, err)
	}
	return nil
}

// SearchIssues executes a JQL query and returns matching issues.
func (c *Client) SearchIssues(ctx context.Context, jql string) ([]*domain.JiraIssue, error) {
	results, _, err := c.rest.Issue.Search.Get(ctx, jql, nil, nil, 0, 100, "")
	if err != nil {
		return nil, fmt.Errorf("search jira issues (jql=%q): %w", jql, err)
	}
	out := make([]*domain.JiraIssue, 0, len(results.Issues))
	for _, issue := range results.Issues {
		out = append(out, issueSchemeToJiraIssue(issue))
	}
	return out, nil
}

// ListBoards retrieves boards by project key and optional board type (e.g. "scrum", "kanban").
func (c *Client) ListBoards(ctx context.Context, projectKey, boardType string) ([]*domain.Board, error) {
	opts := &atlassianmodels.GetBoardsOptions{
		ProjectKeyOrID: projectKey,
		BoardType:      boardType,
	}
	page, _, err := c.agile.Board.Gets(ctx, opts, 0, 50)
	if err != nil {
		return nil, fmt.Errorf("list boards for project %s: %w", projectKey, err)
	}
	out := make([]*domain.Board, 0, len(page.Values))
	for _, b := range page.Values {
		out = append(out, &domain.Board{
			ID:   b.ID,
			Name: b.Name,
			Type: b.Type,
		})
	}
	return out, nil
}

// GetActiveSprintIssues fetches all issues belonging to the active sprint of the given board.
// Returns an empty slice when no active sprint exists.
func (c *Client) GetActiveSprintIssues(ctx context.Context, boardID int) ([]*domain.JiraIssue, error) {
	sprintPage, _, err := c.agile.Board.Sprints(ctx, boardID, 0, 50, []string{"active"})
	if err != nil {
		return nil, fmt.Errorf("list sprints for board %d: %w", boardID, err)
	}
	if len(sprintPage.Values) == 0 {
		return nil, nil
	}

	activeSprint := sprintPage.Values[0]
	issuesPage, _, err := c.agile.Sprint.Issues(ctx, activeSprint.ID, nil, 0, 100)
	if err != nil {
		return nil, fmt.Errorf("get issues for sprint %d (board %d): %w", activeSprint.ID, boardID, err)
	}

	// Sprint issues from the agile API return only key+id; fetch each full issue via REST.
	out := make([]*domain.JiraIssue, 0, len(issuesPage.Issues))
	for _, sprintIssue := range issuesPage.Issues {
		issue, err := c.GetIssue(ctx, sprintIssue.Key)
		if err != nil {
			return nil, fmt.Errorf("get sprint issue %s: %w", sprintIssue.Key, err)
		}
		out = append(out, issue)
	}
	return out, nil
}

// issueSchemeToJiraIssue maps a go-atlassian IssueScheme (v3 ADF) to domain.JiraIssue.
// Sprint and StoryPoints are custom fields not present in the standard schema — they are left empty.
func issueSchemeToJiraIssue(s *atlassianmodels.IssueScheme) *domain.JiraIssue {
	if s == nil {
		return nil
	}
	issue := &domain.JiraIssue{
		Key: s.Key,
	}
	if s.Fields == nil {
		return issue
	}
	f := s.Fields
	issue.Summary = f.Summary
	issue.CreatedAt = parseJiraTime(f.Created)
	issue.UpdatedAt = parseJiraTime(f.Updated)

	if f.Status != nil {
		issue.Status = f.Status.Name
	}
	if f.Priority != nil {
		issue.Priority = f.Priority.Name
	}
	if f.IssueType != nil {
		issue.IssueType = f.IssueType.Name
	}
	if f.Assignee != nil {
		issue.Assignee = f.Assignee.DisplayName
	}
	if f.Reporter != nil {
		issue.Reporter = f.Reporter.DisplayName
	}
	if f.Description != nil {
		issue.Description = extractADFText(f.Description)
	}
	if f.Comment != nil {
		issue.Comments = commentSchemesToDomain(f.Comment)
	}
	return issue
}

// commentSchemesToDomain converts a go-atlassian IssueCommentPageScheme to []domain.Comment.
func commentSchemesToDomain(page *atlassianmodels.IssueCommentPageScheme) []domain.Comment {
	if page == nil {
		return nil
	}
	out := make([]domain.Comment, 0, len(page.Comments))
	for _, c := range page.Comments {
		var body string
		if c.Body != nil {
			body = extractADFText(c.Body)
		}
		var author string
		if c.Author != nil {
			author = c.Author.DisplayName
		}
		out = append(out, domain.Comment{
			Author:    author,
			Body:      body,
			CreatedAt: parseJiraTime(c.Created),
		})
	}
	return out
}

// plainTextADFNode builds a minimal ADF document wrapping plain text for use in comment payloads.
func plainTextADFNode(text string) *atlassianmodels.CommentNodeScheme {
	return &atlassianmodels.CommentNodeScheme{
		Version: 1,
		Type:    "doc",
		Content: []*atlassianmodels.CommentNodeScheme{
			{
				Type: "paragraph",
				Content: []*atlassianmodels.CommentNodeScheme{
					{
						Type: "text",
						Text: text,
					},
				},
			},
		},
	}
}

// extractADFText recursively extracts plain text from an ADF CommentNodeScheme tree.
func extractADFText(node *atlassianmodels.CommentNodeScheme) string {
	if node == nil {
		return ""
	}
	if node.Type == "text" {
		return node.Text
	}
	result := ""
	for _, child := range node.Content {
		result += extractADFText(child)
	}
	return result
}

// parseJiraTime parses a Jira ISO 8601 timestamp string.
// Returns zero time on empty input or parse failure.
func parseJiraTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.999-0700",
		"2006-01-02T15:04:05.999Z07:00",
	}
	for _, format := range formats {
		t, err := time.Parse(format, s)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

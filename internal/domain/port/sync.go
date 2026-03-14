// internal/domain/port/sync.go
package port

import (
	"context"

	"github.com/churndesk/churndesk/internal/domain"
)

// Fetcher is the outbound port for fetching items from an external integration.
type Fetcher interface {
	Fetch(ctx context.Context, integration domain.Integration, spaces []domain.Space) ([]domain.Item, error)
}

// GitHubClient is the outbound port for GitHub API operations.
type GitHubClient interface {
	GetPR(ctx context.Context, owner, repo string, number int) (*domain.PRDetail, error)
	ListPRComments(ctx context.Context, owner, repo string, number int) ([]domain.Comment, error)
	ListPRReviews(ctx context.Context, owner, repo string, number int) ([]domain.Review, error)
	ListCheckRuns(ctx context.Context, owner, repo string, headSHA string) ([]domain.CheckRun, error)
	PostPRComment(ctx context.Context, owner, repo string, number int, body string) error
	SubmitReview(ctx context.Context, owner, repo string, number int, verdict, body string) error
	RequestReviewers(ctx context.Context, owner, repo string, number int, logins []string) error
	ListPRsForRepo(ctx context.Context, owner, repo string) ([]*domain.PRDetail, error)
}

// JiraClient is the outbound port for Jira API operations.
type JiraClient interface {
	GetIssue(ctx context.Context, key string) (*domain.JiraIssue, error)
	ListIssueComments(ctx context.Context, key string) ([]domain.Comment, error)
	PostComment(ctx context.Context, key string, body string) error
	SearchIssues(ctx context.Context, jql string) ([]*domain.JiraIssue, error)
	ListBoards(ctx context.Context, projectKey, boardType string) ([]*domain.Board, error)
	GetActiveSprintIssues(ctx context.Context, boardID int) ([]*domain.JiraIssue, error)
}

// internal/adapter/github/client.go
package github

import (
	"context"
	"fmt"

	"github.com/churndesk/churndesk/internal/domain"
	gogithub "github.com/google/go-github/v68/github"
)

// Client wraps *gogithub.Client and translates go-github types to domain types.
// It implements port.GitHubClient — no caller imports go-github directly.
type Client struct {
	gh                *gogithub.Client
	authenticatedUser string // the logged-in GitHub username
}

// NewClient constructs a GitHub adapter from an already-authenticated *gogithub.Client.
// Construct the underlying client in main.go using an OAuth2 token source.
func NewClient(gh *gogithub.Client, authenticatedUser string) *Client {
	return &Client{gh: gh, authenticatedUser: authenticatedUser}
}

func (c *Client) GetPR(ctx context.Context, owner, repo string, number int) (*domain.PRDetail, error) {
	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("get PR %s/%s#%d: %w", owner, repo, number, err)
	}
	return prToDomain(pr, owner, repo), nil
}

func (c *Client) ListPRComments(ctx context.Context, owner, repo string, number int) ([]domain.Comment, error) {
	var out []domain.Comment
	opts := &gogithub.IssueListCommentsOptions{ListOptions: gogithub.ListOptions{PerPage: 100}}
	for {
		comments, resp, err := c.gh.Issues.ListComments(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, fmt.Errorf("list PR comments: %w", err)
		}
		for _, comment := range comments {
			out = append(out, domain.Comment{
				ID:        comment.GetID(),
				Author:    comment.GetUser().GetLogin(),
				Body:      comment.GetBody(),
				CreatedAt: comment.GetCreatedAt().Time,
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, nil
}

func (c *Client) ListPRReviews(ctx context.Context, owner, repo string, number int) ([]domain.Review, error) {
	reviews, _, err := c.gh.PullRequests.ListReviews(ctx, owner, repo, number, nil)
	if err != nil {
		return nil, fmt.Errorf("list PR reviews: %w", err)
	}
	out := make([]domain.Review, 0, len(reviews))
	for _, r := range reviews {
		out = append(out, domain.Review{
			ID:     r.GetID(),
			Author: r.GetUser().GetLogin(),
			State:  r.GetState(),
		})
	}
	return out, nil
}

func (c *Client) ListCheckRuns(ctx context.Context, owner, repo, headSHA string) ([]domain.CheckRun, error) {
	var out []domain.CheckRun
	opts := &gogithub.ListCheckRunsOptions{ListOptions: gogithub.ListOptions{PerPage: 100}}
	for {
		runs, resp, err := c.gh.Checks.ListCheckRunsForRef(ctx, owner, repo, headSHA, opts)
		if err != nil {
			return nil, fmt.Errorf("list check runs for %s/%s@%s: %w", owner, repo, headSHA, err)
		}
		for _, r := range runs.CheckRuns {
			out = append(out, domain.CheckRun{
				Name:       r.GetName(),
				Status:     r.GetStatus(),
				Conclusion: r.GetConclusion(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, nil
}

func (c *Client) PostPRComment(ctx context.Context, owner, repo string, number int, body string) error {
	_, _, err := c.gh.Issues.CreateComment(ctx, owner, repo, number, &gogithub.IssueComment{Body: gogithub.Ptr(body)})
	if err != nil {
		return fmt.Errorf("post PR comment on %s/%s#%d: %w", owner, repo, number, err)
	}
	return nil
}

func (c *Client) SubmitReview(ctx context.Context, owner, repo string, number int, verdict, body string) error {
	_, _, err := c.gh.PullRequests.CreateReview(ctx, owner, repo, number, &gogithub.PullRequestReviewRequest{
		Body:  gogithub.Ptr(body),
		Event: gogithub.Ptr(verdict), // "APPROVE", "REQUEST_CHANGES", "COMMENT"
	})
	if err != nil {
		return fmt.Errorf("submit review on %s/%s#%d: %w", owner, repo, number, err)
	}
	return nil
}

func (c *Client) RequestReviewers(ctx context.Context, owner, repo string, number int, logins []string) error {
	_, _, err := c.gh.PullRequests.RequestReviewers(ctx, owner, repo, number, gogithub.ReviewersRequest{
		Reviewers: logins,
	})
	if err != nil {
		return fmt.Errorf("request reviewers on %s/%s#%d: %w", owner, repo, number, err)
	}
	return nil
}

func (c *Client) ListPRsForRepo(ctx context.Context, owner, repo string) ([]*domain.PRDetail, error) {
	var out []*domain.PRDetail
	opts := &gogithub.PullRequestListOptions{
		State:       "open",
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}
	for {
		prs, resp, err := c.gh.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list PRs for %s/%s: %w", owner, repo, err)
		}
		for _, pr := range prs {
			out = append(out, prToDomain(pr, owner, repo))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, nil
}

func prToDomain(pr *gogithub.PullRequest, owner, repo string) *domain.PRDetail {
	return &domain.PRDetail{
		Number:       pr.GetNumber(),
		Title:        pr.GetTitle(),
		Owner:        owner,
		Repo:         repo,
		Branch:       pr.GetHead().GetRef(),
		BaseBranch:   pr.GetBase().GetRef(),
		Author:       pr.GetUser().GetLogin(),
		HeadSHA:      pr.GetHead().GetSHA(),
		Additions:    pr.GetAdditions(),
		Deletions:    pr.GetDeletions(),
		FilesChanged: pr.GetChangedFiles(),
		State:        pr.GetState(),
		Body:         pr.GetBody(),
		CreatedAt:    pr.GetCreatedAt().Time,
		UpdatedAt:    pr.GetUpdatedAt().Time,
	}
}

// internal/adapter/github/fetcher.go
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

var jiraKeyRegex = regexp.MustCompile(`[A-Z]+-\d+`)

// Ensure Fetcher implements port.Fetcher at compile time.
var _ port.Fetcher = (*Fetcher)(nil)

// Fetcher implements port.Fetcher for GitHub. It depends on port.GitHubClient,
// not the concrete adapter, so it can be tested with a mock.
type Fetcher struct {
	client            port.GitHubClient
	authenticatedUser string
	teammates         []domain.Teammate
	minReviewCount    int
}

// NewFetcher constructs a GitHub Fetcher. teammates and minReviewCount are loaded
// from the database by the app worker before calling Fetch.
func NewFetcher(client port.GitHubClient, authenticatedUser string, teammates []domain.Teammate, minReviewCount int) *Fetcher {
	return &Fetcher{
		client:            client,
		authenticatedUser: authenticatedUser,
		teammates:         teammates,
		minReviewCount:    minReviewCount,
	}
}

// Fetch implements port.Fetcher. integration.LastSyncedAt is nil on first sync;
// fetchers treat nil as "match nothing" for comment-based items.
func (f *Fetcher) Fetch(ctx context.Context, integration domain.Integration, spaces []domain.Space) ([]domain.Item, error) {
	teammateSet := make(map[string]struct{}, len(f.teammates))
	for _, t := range f.teammates {
		teammateSet[t.GitHubUsername] = struct{}{}
	}

	var items []domain.Item
	for _, space := range spaces {
		if !space.Enabled {
			continue
		}
		prs, err := f.client.ListPRsForRepo(ctx, space.Owner, space.Name)
		if err != nil {
			return nil, fmt.Errorf("list PRs for %s/%s: %w", space.Owner, space.Name, err)
		}
		for _, pr := range prs {
			fetched, err := f.processPR(ctx, pr, space, teammateSet, integration.LastSyncedAt)
			if err != nil {
				return nil, err
			}
			items = append(items, fetched...)
		}
	}
	return items, nil
}

func (f *Fetcher) processPR(
	ctx context.Context,
	pr *domain.PRDetail,
	space domain.Space,
	teammateSet map[string]struct{},
	lastSyncedAt *time.Time,
) ([]domain.Item, error) {
	reviews, err := f.client.ListPRReviews(ctx, space.Owner, space.Name, pr.Number)
	if err != nil {
		return nil, fmt.Errorf("list reviews for PR %d: %w", pr.Number, err)
	}
	checks, err := f.client.ListCheckRuns(ctx, space.Owner, space.Name, pr.HeadSHA)
	if err != nil {
		return nil, fmt.Errorf("list checks for PR %d: %w", pr.Number, err)
	}
	comments, err := f.client.ListPRComments(ctx, space.Owner, space.Name, pr.Number)
	if err != nil {
		return nil, fmt.Errorf("list comments for PR %d: %w", pr.Number, err)
	}

	metadata := buildGitHubMetadata(pr, reviews, comments)
	prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", space.Owner, space.Name, pr.Number)
	externalID := strconv.Itoa(pr.Number)

	var items []domain.Item

	isOwnPR := pr.Author == f.authenticatedUser
	_, isTeammate := teammateSet[pr.Author]

	if isTeammate && !isOwnPR {
		userAlreadyReviewed := hasUserReviewed(reviews, f.authenticatedUser)
		approvalCount := countApprovals(reviews, f.authenticatedUser)
		if !userAlreadyReviewed && approvalCount < f.minReviewCount {
			items = append(items, domain.Item{
				ID:         fmt.Sprintf("github:review_needed:%d", pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRReviewNeeded,
				ExternalID: externalID,
				Title:      fmt.Sprintf("Review needed: %s", pr.Title),
				URL:        prURL,
				Metadata:   metadata,
				PROwner:    space.Owner,
				PRRepo:     space.Name,
				CreatedAt:  pr.CreatedAt,
				UpdatedAt:  pr.UpdatedAt,
			})
		}
		if hasDismissedReview(reviews, f.authenticatedUser) {
			items = append(items, domain.Item{
				ID:         fmt.Sprintf("github:stale_review:%d", pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRStaleReview,
				ExternalID: externalID,
				Title:      fmt.Sprintf("Stale review: %s", pr.Title),
				URL:        prURL,
				Metadata:   metadata,
				PROwner:    space.Owner,
				PRRepo:     space.Name,
				CreatedAt:  pr.CreatedAt,
				UpdatedAt:  pr.UpdatedAt,
			})
		}
	}

	if isOwnPR {
		if hasChangesRequested(reviews) {
			items = append(items, domain.Item{
				ID:         fmt.Sprintf("github:changes_requested:%d", pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRChangesRequested,
				ExternalID: externalID,
				Title:      fmt.Sprintf("Changes requested: %s", pr.Title),
				URL:        prURL,
				Metadata:   metadata,
				PROwner:    space.Owner,
				PRRepo:     space.Name,
				CreatedAt:  pr.CreatedAt,
				UpdatedAt:  pr.UpdatedAt,
			})
		}
		if hasCIFailing(checks) {
			items = append(items, domain.Item{
				ID:         fmt.Sprintf("github:ci_failing:%d", pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRCIFailing,
				ExternalID: externalID,
				Title:      fmt.Sprintf("CI failing: %s", pr.Title),
				URL:        prURL,
				Metadata:   metadata,
				PROwner:    space.Owner,
				PRRepo:     space.Name,
				CreatedAt:  pr.CreatedAt,
				UpdatedAt:  pr.UpdatedAt,
			})
		}
		if lastSyncedAt != nil && hasNewCommentsFrom(comments, *lastSyncedAt, f.authenticatedUser) {
			latest := latestComment(comments, f.authenticatedUser)
			items = append(items, domain.Item{
				ID:         fmt.Sprintf("github:comment:%d", pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRNewComment,
				ExternalID: externalID,
				Title:      fmt.Sprintf("New comment: %s", pr.Title),
				URL:        prURL,
				Metadata:   buildCommentMetadata(metadata, latest),
				PROwner:    space.Owner,
				PRRepo:     space.Name,
				CreatedAt:  pr.CreatedAt,
				UpdatedAt:  pr.UpdatedAt,
			})
		}
		if !hasChangesRequested(reviews) && countAllApprovals(reviews) >= f.minReviewCount {
			items = append(items, domain.Item{
				ID:         fmt.Sprintf("github:approved:%d", pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRApproved,
				ExternalID: externalID,
				Title:      fmt.Sprintf("Approved: %s", pr.Title),
				URL:        prURL,
				Metadata:   metadata,
				PROwner:    space.Owner,
				PRRepo:     space.Name,
				CreatedAt:  pr.CreatedAt,
				UpdatedAt:  pr.UpdatedAt,
			})
		}
	}

	return items, nil
}

// buildGitHubMetadata serializes PR metadata as JSON. Silently returns "{}" on marshal error.
func buildGitHubMetadata(pr *domain.PRDetail, reviews []domain.Review, _ []domain.Comment) string {
	type reviewJSON struct {
		Login string `json:"login"`
		State string `json:"state"`
	}
	type meta struct {
		PRNumber     int          `json:"pr_number"`
		PRTitle      string       `json:"pr_title"`
		PROwner      string       `json:"pr_owner"`
		PRRepo       string       `json:"pr_repo"`
		Branch       string       `json:"branch"`
		BaseBranch   string       `json:"base_branch"`
		Author       string       `json:"author"`
		Additions    int          `json:"additions"`
		Deletions    int          `json:"deletions"`
		FilesChanged int          `json:"files_changed"`
		Reviews      []reviewJSON `json:"reviews"`
	}
	reviewsJSON := make([]reviewJSON, 0, len(reviews))
	for _, r := range reviews {
		reviewsJSON = append(reviewsJSON, reviewJSON{Login: r.Author, State: r.State})
	}
	b, _ := json.Marshal(meta{
		PRNumber:     pr.Number,
		PRTitle:      pr.Title,
		PROwner:      pr.Owner,
		PRRepo:       pr.Repo,
		Branch:       pr.Branch,
		BaseBranch:   pr.BaseBranch,
		Author:       pr.Author,
		Additions:    pr.Additions,
		Deletions:    pr.Deletions,
		FilesChanged: pr.FilesChanged,
		Reviews:      reviewsJSON,
	})
	if b == nil {
		return "{}"
	}
	return string(b)
}

func buildCommentMetadata(base string, latest *domain.Comment) string {
	if latest == nil {
		return base
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(base), &m); err != nil {
		return base
	}
	m["latest_comment"] = latest.Body
	m["latest_comment_author"] = latest.Author
	m["latest_comment_at"] = latest.CreatedAt.Format(time.RFC3339)
	b, _ := json.Marshal(m)
	return string(b)
}

// ExtractJiraKeys returns all Jira issue keys found in PR title, body, and branch.
func ExtractJiraKeys(title, body, branch string) []string {
	seen := map[string]struct{}{}
	var keys []string
	for _, s := range []string{title, body, branch} {
		for _, k := range jiraKeyRegex.FindAllString(s, -1) {
			if _, ok := seen[k]; !ok {
				seen[k] = struct{}{}
				keys = append(keys, k)
			}
		}
	}
	return keys
}

func countApprovals(reviews []domain.Review, excludeUser string) int {
	n := 0
	for _, r := range reviews {
		if r.State == "APPROVED" && r.Author != excludeUser {
			n++
		}
	}
	return n
}

func countAllApprovals(reviews []domain.Review) int {
	n := 0
	for _, r := range reviews {
		if r.State == "APPROVED" {
			n++
		}
	}
	return n
}

func hasUserReviewed(reviews []domain.Review, user string) bool {
	for _, r := range reviews {
		if r.Author == user {
			return true
		}
	}
	return false
}

func hasDismissedReview(reviews []domain.Review, user string) bool {
	for _, r := range reviews {
		if r.Author == user && r.State == "DISMISSED" {
			return true
		}
	}
	return false
}

func hasChangesRequested(reviews []domain.Review) bool {
	for _, r := range reviews {
		if r.State == "CHANGES_REQUESTED" {
			return true
		}
	}
	return false
}

func hasCIFailing(checks []domain.CheckRun) bool {
	for _, c := range checks {
		if c.Status == "completed" && (c.Conclusion == "failure" || c.Conclusion == "action_required") {
			return true
		}
	}
	return false
}

func hasNewCommentsFrom(comments []domain.Comment, since time.Time, excludeAuthor string) bool {
	for _, c := range comments {
		if c.Author != excludeAuthor && c.CreatedAt.After(since) {
			return true
		}
	}
	return false
}

func latestComment(comments []domain.Comment, excludeAuthor string) *domain.Comment {
	var latest *domain.Comment
	for i := range comments {
		c := &comments[i]
		if c.Author == excludeAuthor {
			continue
		}
		if latest == nil || c.CreatedAt.After(latest.CreatedAt) {
			latest = c
		}
	}
	return latest
}

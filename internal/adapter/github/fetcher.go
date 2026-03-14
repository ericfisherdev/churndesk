// internal/adapter/github/fetcher.go
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"sync"
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

	mu             sync.RWMutex
	teammates      []domain.Teammate
	minReviewCount int
}

// NewFetcher constructs a GitHub Fetcher with an initial teammate list and minimum
// review count. Both values can be updated live via UpdateTeammates and
// UpdateMinReviewCount without recreating the fetcher.
func NewFetcher(client port.GitHubClient, authenticatedUser string, teammates []domain.Teammate, minReviewCount int) *Fetcher {
	return &Fetcher{
		client:            client,
		authenticatedUser: authenticatedUser,
		teammates:         teammates,
		minReviewCount:    minReviewCount,
	}
}

// UpdateTeammates replaces the teammate list used by Fetch. Safe to call
// concurrently with Fetch.
func (f *Fetcher) UpdateTeammates(teammates []domain.Teammate) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.teammates = teammates
}

// UpdateMinReviewCount replaces the minimum review count used by Fetch. Safe to
// call concurrently with Fetch.
func (f *Fetcher) UpdateMinReviewCount(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.minReviewCount = n
}

// Fetch implements port.Fetcher. integration.LastSyncedAt is nil on first sync;
// fetchers treat nil as "match nothing" for comment-based items.
func (f *Fetcher) Fetch(ctx context.Context, integration domain.Integration, spaces []domain.Space) ([]domain.Item, error) {
	f.mu.RLock()
	teammates := f.teammates
	minReviewCount := f.minReviewCount
	f.mu.RUnlock()

	teammateSet := make(map[string]struct{}, len(teammates))
	for _, t := range teammates {
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
			fetched, err := f.processPR(ctx, pr, space, teammateSet, minReviewCount, integration.LastSyncedAt)
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
	minReviewCount int,
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

	metadata := buildGitHubMetadata(pr, reviews)
	prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", space.Owner, space.Name, pr.Number)
	externalID := strconv.Itoa(pr.Number)

	var items []domain.Item

	isOwnPR := pr.Author == f.authenticatedUser
	_, isTeammate := teammateSet[pr.Author]

	collapsed := collapseReviewsToLatest(reviews)

	if isTeammate && !isOwnPR {
		userAlreadyReviewed := hasUserReviewed(collapsed, f.authenticatedUser)
		approvalCount := countApprovals(collapsed, f.authenticatedUser)
		if !userAlreadyReviewed && approvalCount < minReviewCount {
			items = append(items, domain.Item{
				ID:         fmt.Sprintf("github:review_needed:%s/%s:%d", space.Owner, space.Name, pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRReviewNeeded,
				ExternalID: externalID,
				Title:      "Review needed: " + pr.Title,
				URL:        prURL,
				Metadata:   metadata,
				PROwner:    space.Owner,
				PRRepo:     space.Name,
				CreatedAt:  pr.CreatedAt,
				UpdatedAt:  pr.UpdatedAt,
			})
		}
		if hasDismissedReview(collapsed, f.authenticatedUser) {
			items = append(items, domain.Item{
				ID:         fmt.Sprintf("github:stale_review:%s/%s:%d", space.Owner, space.Name, pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRStaleReview,
				ExternalID: externalID,
				Title:      "Stale review: " + pr.Title,
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
		if hasChangesRequested(collapsed) {
			items = append(items, domain.Item{
				ID:         fmt.Sprintf("github:changes_requested:%s/%s:%d", space.Owner, space.Name, pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRChangesRequested,
				ExternalID: externalID,
				Title:      "Changes requested: " + pr.Title,
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
				ID:         fmt.Sprintf("github:ci_failing:%s/%s:%d", space.Owner, space.Name, pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRCIFailing,
				ExternalID: externalID,
				Title:      "CI failing: " + pr.Title,
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
				ID:         fmt.Sprintf("github:comment:%s/%s:%d", space.Owner, space.Name, pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRNewComment,
				ExternalID: externalID,
				Title:      "New comment: " + pr.Title,
				URL:        prURL,
				Metadata:   buildCommentMetadata(metadata, latest),
				PROwner:    space.Owner,
				PRRepo:     space.Name,
				CreatedAt:  pr.CreatedAt,
				UpdatedAt:  pr.UpdatedAt,
			})
		}
		if !hasChangesRequested(collapsed) && countAllApprovals(collapsed) >= minReviewCount {
			items = append(items, domain.Item{
				ID:         fmt.Sprintf("github:approved:%s/%s:%d", space.Owner, space.Name, pr.Number),
				Source:     "github",
				Type:       domain.ItemTypePRApproved,
				ExternalID: externalID,
				Title:      "Approved: " + pr.Title,
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
func buildGitHubMetadata(pr *domain.PRDetail, reviews []domain.Review) string {
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
	b, err := json.Marshal(m)
	if err != nil {
		return base
	}
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

// collapseReviewsToLatest returns the most recent review state per reviewer.
// It relies on the GitHub API returning domain.Review entries in chronological
// order — the last entry for a given author wins. If that ordering guarantee
// ever becomes unreliable, add a SubmittedAt timestamp to domain.Review and
// sort by it before iterating here.
func collapseReviewsToLatest(reviews []domain.Review) map[string]string {
	latest := make(map[string]string, len(reviews))
	for _, r := range reviews {
		latest[r.Author] = r.State
	}
	return latest
}

func countApprovals(collapsed map[string]string, excludeUser string) int {
	n := 0
	for author, state := range collapsed {
		if state == "APPROVED" && author != excludeUser {
			n++
		}
	}
	return n
}

func countAllApprovals(collapsed map[string]string) int {
	n := 0
	for _, state := range collapsed {
		if state == "APPROVED" {
			n++
		}
	}
	return n
}

func hasUserReviewed(collapsed map[string]string, user string) bool {
	_, ok := collapsed[user]
	return ok
}

func hasDismissedReview(collapsed map[string]string, user string) bool {
	return collapsed[user] == "DISMISSED"
}

func hasChangesRequested(collapsed map[string]string) bool {
	for _, state := range collapsed {
		if state == "CHANGES_REQUESTED" {
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

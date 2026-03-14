## Chunk 3: Sync Adapters + Application Services

The sync layer has two parts:
- **Adapters** (`internal/adapter/github/`, `internal/adapter/jira/`): implement `port.GitHubClient`, `port.JiraClient`, and `port.Fetcher` — all external API calls live here
- **Application services** (`internal/app/`): orchestrate fetching and scoring using injected port interfaces — no external API imports

> **Note on API library docs:** Use context7 to look up exact method signatures for `go-github/v68` and `go-atlassian` when implementing adapters. The interfaces in `port/sync.go` are the contracts; adapter implementations translate between library types and domain types.

---

### Task 10: GitHub Client Adapter

**Files:**
- Create: `internal/adapter/github/client.go`
- Create: `internal/adapter/github/client_test.go`

The `GitHubClient` adapter wraps `go-github/v68`. Handlers and fetchers only import `domain/port` — never `go-github` directly.

- [ ] **Step 1: Write the failing test**

Tests for the real GitHub client require a live token, so we test behavior via the interface. Instead, create a mock and verify the adapter compiles and satisfies the interface:

```go
// internal/adapter/github/client_test.go
package github_test

import (
	"testing"

	"github.com/churndesk/churndesk/internal/adapter/github"
	"github.com/churndesk/churndesk/internal/domain/port"
	gogithub "github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

// TestNewClient_ImplementsInterface verifies the adapter satisfies the port at compile time.
func TestNewClient_ImplementsInterface(t *testing.T) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test"})
	tc := oauth2.NewClient(t.Context(), ts)
	gc := gogithub.NewClient(tc)

	var _ port.GitHubClient = github.NewClient(gc, "testuser")
}
```

Note: `oauth2` package needed — add with `go get golang.org/x/oauth2`.

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/adapter/github/...
```
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Implement GitHub client adapter**

Use context7 to look up `go-github/v68` API if method signatures are unclear: `mcp__context7__resolve-library-id` with "go-github" then `mcp__context7__query-docs`.

```go
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
	gh           *gogithub.Client
	authenticatedUser string // the logged-in GitHub username
}

// NewClient constructs a GitHub adapter from an already-authenticated *gogithub.Client.
// Construct the underlying client in main.go using an OAuth2 token source.
func NewClient(gh *gogithub.Client, authenticatedUser string) *Client {
	return &Client{gh: gh, authenticatedUser: authenticatedUser}
}

func (c *Client) GetPR(ctx context.Context, owner, repo string, number int) (*domain.PRDetail, error) {
	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number, nil)
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
		for _, c := range comments {
			out = append(out, domain.Comment{
				ID:        c.GetID(),
				Author:    c.GetUser().GetLogin(),
				Body:      c.GetBody(),
				CreatedAt: c.GetCreatedAt().Time,
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
	runs, _, err := c.gh.Checks.ListCheckRunsForRef(ctx, owner, repo, headSHA, nil)
	if err != nil {
		return nil, fmt.Errorf("list check runs: %w", err)
	}
	out := make([]domain.CheckRun, 0, len(runs.CheckRuns))
	for _, r := range runs.CheckRuns {
		out = append(out, domain.CheckRun{
			Name:       r.GetName(),
			Status:     r.GetStatus(),
			Conclusion: r.GetConclusion(),
		})
	}
	return out, nil
}

func (c *Client) PostPRComment(ctx context.Context, owner, repo string, number int, body string) error {
	_, _, err := c.gh.Issues.CreateComment(ctx, owner, repo, number, &gogithub.IssueComment{Body: gogithub.Ptr(body)})
	return err
}

func (c *Client) SubmitReview(ctx context.Context, owner, repo string, number int, verdict, body string) error {
	_, _, err := c.gh.PullRequests.CreateReview(ctx, owner, repo, number, &gogithub.PullRequestReviewRequest{
		Body:  gogithub.Ptr(body),
		Event: gogithub.Ptr(verdict), // "APPROVE", "REQUEST_CHANGES", "COMMENT"
	})
	return err
}

func (c *Client) RequestReviewers(ctx context.Context, owner, repo string, number int, logins []string) error {
	_, _, err := c.gh.PullRequests.RequestReviewers(ctx, owner, repo, number, gogithub.ReviewersRequest{
		Reviewers: logins,
	})
	return err
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
			d := prToDomain(pr, owner, repo)
			out = append(out, d)
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
```

- [ ] **Step 4: Add oauth2 dependency and run test**

```bash
go get golang.org/x/oauth2
CGO_ENABLED=1 go test ./internal/adapter/github/...
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/github/client.go internal/adapter/github/client_test.go go.mod go.sum
git commit -m "feat: add GitHub client adapter implementing port.GitHubClient"
```

---

### Task 11: Jira Client Adapter

**Files:**
- Create: `internal/adapter/jira/client.go`
- Create: `internal/adapter/jira/client_test.go`

Uses `go-atlassian`. Use context7 to look up exact method signatures: `mcp__context7__resolve-library-id` with "go-atlassian" then `mcp__context7__query-docs`.

- [ ] **Step 1: Write the failing test**

```go
// internal/adapter/jira/client_test.go
package jira_test

import (
	"testing"

	jiradapter "github.com/churndesk/churndesk/internal/adapter/jira"
	"github.com/churndesk/churndesk/internal/domain/port"
)

// TestNewClient_ImplementsInterface verifies the adapter satisfies the port at compile time.
func TestNewClient_ImplementsInterface(t *testing.T) {
	var _ port.JiraClient = jiradapter.NewClient("https://example.atlassian.net", "user@example.com", "token", "account-id")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/adapter/jira/...
```
Expected: FAIL

- [ ] **Step 3: Implement Jira client adapter**

> **Important:** Look up the exact go-atlassian API using context7 before implementing. The go-atlassian library uses a different client construction pattern than go-jira. Key packages: `github.com/ctreminiom/go-atlassian/jira/v3` for REST API v3, `github.com/ctreminiom/go-atlassian/jira/agile` for board/sprint APIs. All timestamps come as ISO 8601 strings — parse with `time.Parse(time.RFC3339, ...)`.

```go
// internal/adapter/jira/client.go
package jira

import (
	"context"
	"fmt"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	// Look up exact imports via context7 for go-atlassian
	// Example structure (verify with context7):
	// jiracloud "github.com/ctreminiom/go-atlassian/jira/v3"
	// jiraagile "github.com/ctreminiom/go-atlassian/jira/agile"
)

// Client wraps the go-atlassian clients and translates types to domain types.
// It implements port.JiraClient.
type Client struct {
	baseURL       string
	accountID     string
	// rest    *jiracloud.Client  — actual type from go-atlassian
	// agile   *jiraagile.Client  — actual type from go-atlassian
}

// NewClient constructs a Jira adapter. Credentials are used to build go-atlassian clients internally.
// baseURL example: "https://myorg.atlassian.net"
// email: Atlassian account email for basic auth
// token: Atlassian API token
// accountID: Atlassian account ID (stored in integrations.username)
func NewClient(baseURL, email, token, accountID string) *Client {
	// TODO: construct go-atlassian REST and Agile clients using baseURL, email, token
	// Use context7 to look up: go-atlassian client initialization pattern
	return &Client{baseURL: baseURL, accountID: accountID}
}

func (c *Client) GetIssue(ctx context.Context, key string) (*domain.JiraIssue, error) {
	// TODO: implement using go-atlassian REST client
	// Look up: go-atlassian Issue.Get method signature
	return nil, fmt.Errorf("not implemented — check context7 for go-atlassian API")
}

func (c *Client) ListIssueComments(ctx context.Context, key string) ([]domain.Comment, error) {
	// TODO: implement using go-atlassian REST client
	return nil, fmt.Errorf("not implemented")
}

func (c *Client) PostComment(ctx context.Context, key string, body string) error {
	// TODO: implement using go-atlassian REST client
	return fmt.Errorf("not implemented")
}

func (c *Client) SearchIssues(ctx context.Context, jql string) ([]*domain.JiraIssue, error) {
	// TODO: implement using go-atlassian REST client with JQL search
	return nil, fmt.Errorf("not implemented")
}

func (c *Client) ListBoards(ctx context.Context, projectKey, boardType string) ([]*domain.Board, error) {
	// TODO: implement using go-atlassian Agile client
	// GET /rest/agile/1.0/board?projectKeyOrId={key}&type={boardType}
	return nil, fmt.Errorf("not implemented")
}

func (c *Client) GetActiveSprintIssues(ctx context.Context, boardID int) ([]*domain.JiraIssue, error) {
	// TODO: implement using go-atlassian Agile client
	// Step 1: GET /rest/agile/1.0/board/{boardID}/sprint?state=active → get sprint ID
	// Step 2: GET /rest/agile/1.0/sprint/{sprintID}/issue
	return nil, fmt.Errorf("not implemented")
}

// parseJiraTime parses Jira's ISO 8601 timestamp strings (with timezone offsets).
// Always returns UTC. Never compare raw Jira timestamp strings — always parse first.
func parseJiraTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Jira sometimes returns milliseconds: "2026-03-13T09:00:00.000+0530"
		t, _ = time.Parse("2006-01-02T15:04:05.000Z07:00", s)
	}
	return t.UTC()
}
```

> **Implementation note:** The `NewClient` function and all `TODO` methods must be completed using the actual go-atlassian API before this task is marked done. The stub above shows the skeleton and the context7 lookup instruction. The interface compile test ensures the adapter satisfies `port.JiraClient` once implemented.

- [ ] **Step 4: Run test to verify it passes**

```bash
CGO_ENABLED=1 go test ./internal/adapter/jira/...
```
Expected: PASS (compile test passes once all methods are present)

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/jira/client.go internal/adapter/jira/client_test.go
git commit -m "feat: add Jira client adapter implementing port.JiraClient"
```

---

### Task 12: GitHub Fetcher Adapter

**Files:**
- Create: `internal/adapter/github/fetcher.go`
- Create: `internal/adapter/github/fetcher_test.go`

The GitHub fetcher calls `port.GitHubClient` (never the concrete adapter) to stay testable. Tests use a hand-rolled mock of `port.GitHubClient`.

- [ ] **Step 1: Write the failing test**

```go
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
	spaces := []domain.Space{{Owner: "myorg", Name: "myrepo"}}
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
	assert.Equal(t, "github:review_needed:42", reviewNeeded[0].ID)
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
	spaces := []domain.Space{{Owner: "myorg", Name: "myrepo"}}

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
	assert.Equal(t, "github:ci_failing:99", ciFailing[0].ID)
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
	spaces := []domain.Space{{Owner: "myorg", Name: "myrepo"}}
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
			{ID: 1, Author: "alice", Body: "I pushed a fix", CreatedAt: time.Now()},        // self-authored — must be excluded
			{ID: 2, Author: "bob", Body: "Looks good to me", CreatedAt: time.Now()},         // other-authored — must trigger
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
	assert.Equal(t, "github:comment:7", newComments[0].ID)
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/adapter/github/... -run TestGitHubFetcher
```
Expected: FAIL

- [ ] **Step 3: Implement GitHub fetcher**

```go
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

// Fetcher implements port.Fetcher for GitHub. It depends on port.GitHubClient,
// not the concrete adapter, so it can be tested with a mock.
type Fetcher struct {
	client           port.GitHubClient
	authenticatedUser string
	teammates        []domain.Teammate
	minReviewCount   int
}

// NewFetcher constructs a GitHub Fetcher. teammates and minReviewCount are loaded
// from the database by the app worker before calling Fetch.
func NewFetcher(client port.GitHubClient, authenticatedUser string, teammates []domain.Teammate, minReviewCount int) *Fetcher {
	return &Fetcher{
		client:           client,
		authenticatedUser: authenticatedUser,
		teammates:        teammates,
		minReviewCount:   minReviewCount,
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

func (f *Fetcher) processPR(ctx context.Context, pr *domain.PRDetail, space domain.Space, teammateSet map[string]struct{}, lastSyncedAt *time.Time) ([]domain.Item, error) {
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

	var items []domain.Item

	isOwnPR := pr.Author == f.authenticatedUser
	_, isTeammate := teammateSet[pr.Author]

	if isTeammate && !isOwnPR {
		// Teammate's PR — check if authenticated user needs to review
		approvalCount := countApprovals(reviews, f.authenticatedUser)
		userAlreadyReviewed := hasUserReviewed(reviews, f.authenticatedUser)
		if !userAlreadyReviewed && approvalCount < f.minReviewCount {
			items = append(items, domain.Item{
				ID: fmt.Sprintf("github:review_needed:%d", pr.Number),
				Source: "github", Type: domain.ItemTypePRReviewNeeded,
				ExternalID: strconv.Itoa(pr.Number),
				Title: fmt.Sprintf("Review needed: %s", pr.Title),
				URL: fmt.Sprintf("https://github.com/%s/%s/pull/%d", space.Owner, space.Name, pr.Number),
				Metadata: metadata, PROwner: space.Owner, PRRepo: space.Name,
				CreatedAt: pr.CreatedAt, UpdatedAt: pr.UpdatedAt,
			})
		}
		// Stale review: authenticated user had approval that was DISMISSED
		if hasDismissedReview(reviews, f.authenticatedUser) {
			items = append(items, domain.Item{
				ID: fmt.Sprintf("github:stale_review:%d", pr.Number),
				Source: "github", Type: domain.ItemTypePRStaleReview,
				ExternalID: strconv.Itoa(pr.Number),
				Title: fmt.Sprintf("Stale review: %s", pr.Title),
				URL: fmt.Sprintf("https://github.com/%s/%s/pull/%d", space.Owner, space.Name, pr.Number),
				Metadata: metadata, PROwner: space.Owner, PRRepo: space.Name,
				CreatedAt: pr.CreatedAt, UpdatedAt: pr.UpdatedAt,
			})
		}
	}

	if isOwnPR {
		// Own PR checks
		if hasChangesRequested(reviews) {
			items = append(items, domain.Item{
				ID: fmt.Sprintf("github:changes_requested:%d", pr.Number),
				Source: "github", Type: domain.ItemTypePRChangesRequested,
				ExternalID: strconv.Itoa(pr.Number),
				Title: fmt.Sprintf("Changes requested: %s", pr.Title),
				URL: fmt.Sprintf("https://github.com/%s/%s/pull/%d", space.Owner, space.Name, pr.Number),
				Metadata: metadata, PROwner: space.Owner, PRRepo: space.Name,
				CreatedAt: pr.CreatedAt, UpdatedAt: pr.UpdatedAt,
			})
		}
		if hasCIFailing(checks) {
			items = append(items, domain.Item{
				ID: fmt.Sprintf("github:ci_failing:%d", pr.Number),
				Source: "github", Type: domain.ItemTypePRCIFailing,
				ExternalID: strconv.Itoa(pr.Number),
				Title: fmt.Sprintf("CI failing: %s", pr.Title),
				URL: fmt.Sprintf("https://github.com/%s/%s/pull/%d", space.Owner, space.Name, pr.Number),
				Metadata: metadata, PROwner: space.Owner, PRRepo: space.Name,
				CreatedAt: pr.CreatedAt, UpdatedAt: pr.UpdatedAt,
			})
		}
		// New comments since last sync — exclude self-authored (spec §6.1)
		if lastSyncedAt != nil && hasNewCommentsFrom(comments, *lastSyncedAt, f.authenticatedUser) {
			latest := latestComment(comments, f.authenticatedUser)
			items = append(items, domain.Item{
				ID: fmt.Sprintf("github:comment:%d", pr.Number),
				Source: "github", Type: domain.ItemTypePRNewComment,
				ExternalID: strconv.Itoa(pr.Number),
				Title: fmt.Sprintf("New comment: %s", pr.Title),
				URL: fmt.Sprintf("https://github.com/%s/%s/pull/%d", space.Owner, space.Name, pr.Number),
				Metadata: buildCommentMetadata(metadata, latest),
				PROwner: space.Owner, PRRepo: space.Name,
				CreatedAt: pr.CreatedAt, UpdatedAt: pr.UpdatedAt,
			})
		}
		// Approved: meets minReviewCount with no CHANGES_REQUESTED
		if !hasChangesRequested(reviews) && countAllApprovals(reviews) >= f.minReviewCount {
			items = append(items, domain.Item{
				ID: fmt.Sprintf("github:approved:%d", pr.Number),
				Source: "github", Type: domain.ItemTypePRApproved,
				ExternalID: strconv.Itoa(pr.Number),
				Title: fmt.Sprintf("Approved: %s", pr.Title),
				URL: fmt.Sprintf("https://github.com/%s/%s/pull/%d", space.Owner, space.Name, pr.Number),
				Metadata: metadata, PROwner: space.Owner, PRRepo: space.Name,
				CreatedAt: pr.CreatedAt, UpdatedAt: pr.UpdatedAt,
			})
		}
	}
	return items, nil
}

// buildGitHubMetadata serializes PR metadata as JSON. Silently returns "{}" on marshal error.
func buildGitHubMetadata(pr *domain.PRDetail, reviews []domain.Review, comments []domain.Comment) string {
	type reviewJSON struct {
		Login string `json:"login"`
		State string `json:"state"`
	}
	type meta struct {
		PRNumber    int          `json:"pr_number"`
		PRTitle     string       `json:"pr_title"`
		PROwner     string       `json:"pr_owner"`
		PRRepo      string       `json:"pr_repo"`
		Branch      string       `json:"branch"`
		BaseBranch  string       `json:"base_branch"`
		Author      string       `json:"author"`
		Additions   int          `json:"additions"`
		Deletions   int          `json:"deletions"`
		FilesChanged int         `json:"files_changed"`
		Reviews     []reviewJSON `json:"reviews"`
	}
	reviewsJSON := make([]reviewJSON, 0, len(reviews))
	for _, r := range reviews {
		reviewsJSON = append(reviewsJSON, reviewJSON{Login: r.Author, State: r.State})
	}
	b, _ := json.Marshal(meta{
		PRNumber: pr.Number, PRTitle: pr.Title, PROwner: pr.Owner, PRRepo: pr.Repo,
		Branch: pr.Branch, BaseBranch: pr.BaseBranch, Author: pr.Author,
		Additions: pr.Additions, Deletions: pr.Deletions, FilesChanged: pr.FilesChanged,
		Reviews: reviewsJSON,
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
	// Merge latest_comment fields into existing metadata JSON
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
CGO_ENABLED=1 go test ./internal/adapter/github/... -run TestGitHubFetcher
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/github/fetcher.go internal/adapter/github/fetcher_test.go
git commit -m "feat: add GitHub fetcher adapter implementing port.Fetcher"
```

---

### Task 13: Jira Fetcher Adapter

**Files:**
- Create: `internal/adapter/jira/fetcher.go`
- Create: `internal/adapter/jira/fetcher_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/adapter/jira/... -run TestJiraFetcher
```
Expected: FAIL

- [ ] **Step 3: Implement Jira fetcher**

```go
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
// integration.LastSyncedAt == nil on first sync — status changes and comments are suppressed.
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
			fetched := f.processIssue(issue, space, integration.LastSyncedAt)
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

func (f *Fetcher) processIssue(issue *domain.JiraIssue, space domain.Space, lastSyncedAt *time.Time) []domain.Item {
	var items []domain.Item
	metadata := buildJiraMetadata(issue)

	// jira_status_change: assigned to me. Suppressed on first sync (lastSyncedAt == nil) — spec §4.2.
	// Note: fine-grained "did the status actually change?" detection is not done here;
	// the item is upserted on every sync while it remains assigned to me. Dismissal handles resolution.
	if lastSyncedAt != nil && issue.Assignee == f.accountID {
		items = append(items, domain.Item{
			ID:         fmt.Sprintf("jira:status_change:%s", issue.Key),
			Source:     "jira",
			Type:       domain.ItemTypeJiraStatusChange,
			ExternalID: issue.Key,
			Title:      fmt.Sprintf("Status update: %s", issue.Summary),
			Metadata:   metadata,
		})
	}

	// jira_new_bug: Bug issue type created after last sync. Suppressed on first sync.
	if lastSyncedAt != nil && issue.IssueType == "Bug" && issue.CreatedAt.After(*lastSyncedAt) {
		items = append(items, domain.Item{
			ID:         fmt.Sprintf("jira:new_bug:%s", issue.Key),
			Source:     "jira",
			Type:       domain.ItemTypeJiraNewBug,
			ExternalID: issue.Key,
			Title:      fmt.Sprintf("New bug: %s", issue.Summary),
			Metadata:   metadata,
		})
	}

	// jira_comment: new comment on issue where I authored at least one comment. Suppressed on first sync.
	if lastSyncedAt != nil {
		for _, c := range issue.Comments {
			if c.Author == f.accountID {
				// I have commented on this issue — check for new comments from others
				for _, c2 := range issue.Comments {
					if c2.Author != f.accountID && c2.CreatedAt.After(*lastSyncedAt) {
						items = append(items, domain.Item{
							ID:         fmt.Sprintf("jira:comment:%s", issue.Key),
							Source:     "jira",
							Type:       domain.ItemTypeJiraComment,
							ExternalID: issue.Key,
							Title:      fmt.Sprintf("New comment: %s", issue.Summary),
							Metadata:   buildCommentJiraMetadata(metadata, &c2),
						})
						break // one item per issue
					}
				}
				break
			}
		}
	}

	// Set URL on all items
	for i := range items {
		if items[i].URL == "" {
			items[i].URL = fmt.Sprintf("/jira/%s", issue.Key)
		}
		items[i].CreatedAt = issue.CreatedAt
		items[i].UpdatedAt = issue.UpdatedAt
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
CGO_ENABLED=1 go test ./internal/adapter/jira/... -run TestJiraFetcher
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/jira/fetcher.go internal/adapter/jira/fetcher_test.go
git commit -m "feat: add Jira fetcher adapter implementing port.Fetcher"
```

---

### Task 14: Application Worker

**Files:**
- Create: `internal/app/worker.go`
- Create: `internal/app/worker_test.go`

The worker uses only port interfaces — no concrete adapters. It polls the Fetcher, inspects `item.Deleted`, and writes to the ItemStore. On completion, it updates `last_synced_at` via IntegrationStore.

- [ ] **Step 1: Write the failing test**

```go
// internal/app/worker_test.go
package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/churndesk/churndesk/internal/app"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubFetcher returns a fixed slice of items on every call.
type stubFetcher struct{ items []domain.Item }

func (s *stubFetcher) Fetch(_ context.Context, _ domain.Integration, _ []domain.Space) ([]domain.Item, error) {
	return s.items, nil
}

// stubItemStore records calls for inspection.
type stubItemStore struct {
	upserted []domain.Item
	deleted  []string
}

func (s *stubItemStore) Upsert(_ context.Context, items []domain.Item) error {
	s.upserted = append(s.upserted, items...)
	return nil
}
func (s *stubItemStore) Delete(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *stubItemStore) ListRanked(_ context.Context, _ int) ([]domain.Item, error) { return nil, nil }
func (s *stubItemStore) Count(_ context.Context) (int, error)                       { return 0, nil }
func (s *stubItemStore) Dismiss(_ context.Context, _ string) error                  { return nil }
func (s *stubItemStore) MarkSeen(_ context.Context, _ string) error                 { return nil }
func (s *stubItemStore) MarkSeenByPR(_ context.Context, _, _ string, _ int) error   { return nil }
func (s *stubItemStore) MarkSeenByJiraKey(_ context.Context, _ string) error        { return nil }
func (s *stubItemStore) RescoreAll(_ context.Context, _ map[domain.ItemType]int, _ []string, _, _ float64) error {
	return nil
}

// stubIntegrationStore records last_synced_at updates.
type stubIntegrationStore struct{ lastSynced time.Time }

func (s *stubIntegrationStore) UpdateLastSyncedAt(_ context.Context, _ int, t time.Time) error {
	s.lastSynced = t
	return nil
}
func (s *stubIntegrationStore) CreateIntegration(ctx context.Context, i domain.Integration) (int, error) { return 0, nil }
func (s *stubIntegrationStore) GetIntegration(ctx context.Context, id int) (*domain.Integration, error) { return nil, nil }
func (s *stubIntegrationStore) UpdateIntegration(ctx context.Context, i domain.Integration) error { return nil }
func (s *stubIntegrationStore) DeleteIntegration(ctx context.Context, id int) error { return nil }
func (s *stubIntegrationStore) ListIntegrations(ctx context.Context) ([]domain.Integration, error) { return nil, nil }
func (s *stubIntegrationStore) CreateSpace(ctx context.Context, sp domain.Space) (int, error) { return 0, nil }
func (s *stubIntegrationStore) ListSpaces(ctx context.Context, id int) ([]domain.Space, error) { return nil, nil }
func (s *stubIntegrationStore) UpdateSpace(ctx context.Context, sp domain.Space) error { return nil }
func (s *stubIntegrationStore) DeleteSpace(ctx context.Context, id int) error { return nil }
func (s *stubIntegrationStore) CreateTeammate(ctx context.Context, t domain.Teammate) error { return nil }
func (s *stubIntegrationStore) ListTeammates(ctx context.Context, id int) ([]domain.Teammate, error) { return nil, nil }
func (s *stubIntegrationStore) DeleteTeammate(ctx context.Context, id int) error { return nil }
func (s *stubIntegrationStore) CreatePrerequisite(ctx context.Context, p domain.ReviewPrerequisite) error { return nil }
func (s *stubIntegrationStore) ListPrerequisites(ctx context.Context, id int) ([]domain.ReviewPrerequisite, error) { return nil, nil }
func (s *stubIntegrationStore) DeletePrerequisite(ctx context.Context, id int) error { return nil }
func (s *stubIntegrationStore) IsOnboardingComplete(ctx context.Context) (bool, error) { return true, nil }

func TestWorker_RunOnce_UpsertsItems(t *testing.T) {
	fetcher := &stubFetcher{items: []domain.Item{
		{ID: "github:review_needed:1", Type: domain.ItemTypePRReviewNeeded, Source: "github"},
	}}
	itemStore := &stubItemStore{}
	integrationStore := &stubIntegrationStore{}

	integration := domain.Integration{ID: 1, Provider: domain.ProviderGitHub}
	spaces := []domain.Space{{Owner: "myorg", Name: "myrepo", Enabled: true}}

	w := app.NewWorker(fetcher, itemStore, integrationStore)
	err := w.RunOnce(context.Background(), integration, spaces)
	require.NoError(t, err)

	assert.Len(t, itemStore.upserted, 1)
	assert.Equal(t, "github:review_needed:1", itemStore.upserted[0].ID)
	assert.False(t, integrationStore.lastSynced.IsZero(), "last_synced_at must be updated after sync")
}

func TestWorker_RunOnce_DeletesItemsWithDeletedFlag(t *testing.T) {
	fetcher := &stubFetcher{items: []domain.Item{
		{ID: "github:approved:1", Type: domain.ItemTypePRApproved, Source: "github", Deleted: true},
		{ID: "github:review_needed:1", Type: domain.ItemTypePRReviewNeeded, Source: "github", Deleted: false},
	}}
	itemStore := &stubItemStore{}
	integrationStore := &stubIntegrationStore{}

	w := app.NewWorker(fetcher, itemStore, integrationStore)
	err := w.RunOnce(context.Background(), domain.Integration{ID: 1}, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"github:approved:1"}, itemStore.deleted)
	assert.Len(t, itemStore.upserted, 1)
	assert.Equal(t, "github:review_needed:1", itemStore.upserted[0].ID)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/app/... -run TestWorker
```
Expected: FAIL

- [ ] **Step 3: Implement Worker**

```go
// internal/app/worker.go
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

// Worker executes a single sync cycle for one integration: fetch → partition deleted/live → upsert/delete → update last_synced_at.
type Worker struct {
	fetcher          port.Fetcher
	items            port.ItemStore
	integrations     port.IntegrationStore
}

// NewWorker constructs a Worker with all dependencies injected via constructor.
func NewWorker(fetcher port.Fetcher, items port.ItemStore, integrations port.IntegrationStore) *Worker {
	return &Worker{fetcher: fetcher, items: items, integrations: integrations}
}

// RunOnce executes one sync cycle synchronously. Called by the scheduler's poll loop
// and by the manual sync handler.
func (w *Worker) RunOnce(ctx context.Context, integration domain.Integration, spaces []domain.Space) error {
	fetched, err := w.fetcher.Fetch(ctx, integration, spaces)
	if err != nil {
		return fmt.Errorf("fetch for integration %d: %w", integration.ID, err)
	}

	var toUpsert, toDelete []domain.Item
	for _, item := range fetched {
		if item.Deleted {
			toDelete = append(toDelete, item)
		} else {
			toUpsert = append(toUpsert, item)
		}
	}

	for _, item := range toDelete {
		if err := w.items.Delete(ctx, item.ID); err != nil {
			return fmt.Errorf("delete item %s: %w", item.ID, err)
		}
	}
	if len(toUpsert) > 0 {
		if err := w.items.Upsert(ctx, toUpsert); err != nil {
			return fmt.Errorf("upsert items: %w", err)
		}
	}

	if err := w.integrations.UpdateLastSyncedAt(ctx, integration.ID, time.Now()); err != nil {
		return fmt.Errorf("update last_synced_at for integration %d: %w", integration.ID, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
CGO_ENABLED=1 go test ./internal/app/... -run TestWorker
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/worker.go internal/app/worker_test.go
git commit -m "feat: add app Worker: fetch → partition → upsert/delete → update last_synced_at"
```

---

### Task 15: Application Scorer + Scheduler

**Files:**
- Create: `internal/app/scorer.go`
- Create: `internal/app/scorer_test.go`
- Create: `internal/app/scheduler.go`

The scorer is a re-scorer goroutine that runs every 60 seconds. It fetches weights, prerequisites, and settings from stores, then calls `RescoreAll`. The scheduler manages per-integration worker goroutines.

- [ ] **Step 1: Write the failing scorer test**

```go
// internal/app/scorer_test.go
package app_test

import (
	"context"
	"testing"

	"github.com/churndesk/churndesk/internal/app"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSettingsStore implements port.SettingsStore for scorer tests.
type stubSettingsStore struct {
	settings map[domain.SettingKey]string
	weights  []domain.CategoryWeight
}

func (s *stubSettingsStore) Get(_ context.Context, key domain.SettingKey) (string, error) {
	return s.settings[key], nil
}
func (s *stubSettingsStore) Set(_ context.Context, _ domain.SettingKey, _ string) error { return nil }
func (s *stubSettingsStore) GetAll(_ context.Context) (map[domain.SettingKey]string, error) {
	return s.settings, nil
}
func (s *stubSettingsStore) GetCategoryWeights(_ context.Context) ([]domain.CategoryWeight, error) {
	return s.weights, nil
}
func (s *stubSettingsStore) SetCategoryWeight(_ context.Context, _ domain.ItemType, _ int) error {
	return nil
}

// captureRescoreStore records the prerequisiteUsernames passed to RescoreAll.
// Embeds stubItemStore so only RescoreAll needs to be overridden.
type captureRescoreStore struct {
	stubItemStore
	capturedPrereqs []string
}

func (s *captureRescoreStore) RescoreAll(_ context.Context, _ map[domain.ItemType]int, prereqs []string, _, _ float64) error {
	s.capturedPrereqs = prereqs
	return nil
}

func TestScorer_RunOnce_PassesPrerequisites(t *testing.T) {
	settings := &stubSettingsStore{
		settings: map[domain.SettingKey]string{
			domain.SettingAgeMultiplier: "0.5",
			domain.SettingMaxAgeBoost:  "50",
		},
		weights: []domain.CategoryWeight{
			{ItemType: domain.ItemTypePRReviewNeeded, Weight: 60},
		},
	}
	integrations := &stubIntegrationStore{} // reuse from worker_test.go
	capture := &captureRescoreStore{}

	scorer := app.NewScorer(capture, settings, integrations)
	err := scorer.RunOnce(context.Background())
	require.NoError(t, err)
	// no prerequisites configured → empty slice passed to RescoreAll
	assert.Empty(t, capture.capturedPrereqs)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/app/... -run TestScorer
```
Expected: FAIL

- [ ] **Step 3: Implement Scorer**

```go
// internal/app/scorer.go
package app

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

// Scorer re-computes prerequisites_met, age_boost, and total_score for all items every 60s.
// It is the canonical authority for the prerequisites_met column (spec §6.1).
type Scorer struct {
	items        port.ItemStore
	settings     port.SettingsStore
	integrations port.IntegrationStore
	interval     time.Duration
}

// NewScorer constructs a Scorer. interval defaults to 60s.
func NewScorer(items port.ItemStore, settings port.SettingsStore, integrations port.IntegrationStore) *Scorer {
	return &Scorer{items: items, settings: settings, integrations: integrations, interval: 60 * time.Second}
}

// Start runs the re-scorer goroutine until ctx is cancelled.
// Errors are logged but never cause the goroutine to exit.
func (s *Scorer) Start(ctx context.Context) {
	go func() {
		if err := s.RunOnce(ctx); err != nil {
			log.Printf("scorer: initial run error: %v", err)
		}
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.RunOnce(ctx); err != nil {
					log.Printf("scorer: run error: %v", err)
				}
			}
		}
	}()
}

// RunOnce runs one rescore cycle. Called by Start and by tests.
func (s *Scorer) RunOnce(ctx context.Context) error {
	all, err := s.settings.GetAll(ctx)
	if err != nil {
		return err
	}
	ageMultiplier, _ := strconv.ParseFloat(all[domain.SettingAgeMultiplier], 64)
	maxAgeBoost, _ := strconv.ParseFloat(all[domain.SettingMaxAgeBoost], 64)

	categoryWeights, err := s.settings.GetCategoryWeights(ctx)
	if err != nil {
		return err
	}
	weights := make(map[domain.ItemType]int, len(categoryWeights))
	for _, w := range categoryWeights {
		weights[w.ItemType] = w.Weight
	}

	// Collect all prerequisite usernames across all integrations.
	integrations, err := s.integrations.ListIntegrations(ctx)
	if err != nil {
		return err
	}
	prereqSet := map[string]struct{}{}
	for _, integration := range integrations {
		prereqs, err := s.integrations.ListPrerequisites(ctx, integration.ID)
		if err != nil {
			return err
		}
		for _, p := range prereqs {
			prereqSet[p.GitHubUsername] = struct{}{}
		}
	}
	prereqUsernames := make([]string, 0, len(prereqSet))
	for u := range prereqSet {
		prereqUsernames = append(prereqUsernames, u)
	}

	return s.items.RescoreAll(ctx, weights, prereqUsernames, ageMultiplier, maxAgeBoost)
}
```

- [ ] **Step 4: Implement Scheduler**

No test for the scheduler goroutine lifecycle itself (timing-sensitive). Instead, verify it compiles and wires correctly:

```go
// internal/app/scheduler.go
package app

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

// Scheduler manages one poll goroutine per integration.
// Each goroutine calls Worker.RunOnce on a configurable interval.
type Scheduler struct {
	items        port.ItemStore
	integrations port.IntegrationStore
	fetchers     map[domain.Provider]port.Fetcher

	mu      sync.Mutex
	cancels map[int]context.CancelFunc
	ctx     context.Context // server-lifetime context set by Start; used by Reload
}

// NewScheduler constructs a Scheduler. fetchers maps provider to its Fetcher implementation.
func NewScheduler(items port.ItemStore, integrations port.IntegrationStore, fetchers map[domain.Provider]port.Fetcher) *Scheduler {
	return &Scheduler{
		items:        items,
		integrations: integrations,
		fetchers:     fetchers,
		cancels:      make(map[int]context.CancelFunc),
	}
}

// Start launches one goroutine per enabled integration. Blocks until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()

	integrations, err := s.integrations.ListIntegrations(ctx)
	if err != nil {
		return err
	}
	for _, integration := range integrations {
		if !integration.Enabled {
			continue
		}
		s.startWorker(ctx, integration)
	}
	<-ctx.Done()
	return nil
}

func (s *Scheduler) startWorker(parent context.Context, integration domain.Integration) {
	fetcher, ok := s.fetchers[integration.Provider]
	if !ok {
		log.Printf("scheduler: no fetcher for provider %s", integration.Provider)
		return
	}
	workerCtx, cancel := context.WithCancel(parent)

	s.mu.Lock()
	s.cancels[integration.ID] = cancel
	s.mu.Unlock()

	worker := NewWorker(fetcher, s.items, s.integrations)
	interval := time.Duration(integration.PollIntervalSeconds) * time.Second

	go func() {
		spaces, err := s.integrations.ListSpaces(workerCtx, integration.ID)
		if err != nil {
			log.Printf("scheduler: list spaces for integration %d: %v", integration.ID, err)
			return
		}
		// Initial sync immediately on start
		if err := worker.RunOnce(workerCtx, integration, spaces); err != nil {
			log.Printf("scheduler: initial sync for integration %d: %v", integration.ID, err)
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				// Refresh spaces on each cycle in case they changed in settings
				spaces, err = s.integrations.ListSpaces(workerCtx, integration.ID)
				if err != nil {
					log.Printf("scheduler: list spaces: %v", err)
					continue
				}
				if err := worker.RunOnce(workerCtx, integration, spaces); err != nil {
					log.Printf("scheduler: sync error for integration %d: %v", integration.ID, err)
				}
			}
		}
	}()
}
```

- [ ] **Step 4b: Add `Reload` method to Scheduler**

`Reload` is called by the settings handler after integration changes. It stops running workers and restarts them using the stored server-lifetime context from `Start`.

```go
// Reload stops all running workers and restarts them from the current integration list.
// Safe to call concurrently from the settings handler after integration or space changes.
// No-op if called before Start.
func (s *Scheduler) Reload(_ context.Context) error {
	s.mu.Lock()
	serverCtx := s.ctx
	for _, cancel := range s.cancels {
		cancel()
	}
	s.cancels = make(map[int]context.CancelFunc)
	s.mu.Unlock()

	if serverCtx == nil {
		return nil // Start has not been called yet
	}

	integrations, err := s.integrations.ListIntegrations(serverCtx)
	if err != nil {
		return err
	}
	for _, ig := range integrations {
		if !ig.Enabled {
			continue
		}
		s.startWorker(serverCtx, ig)
	}
	return nil
}
```

- [ ] **Step 5: Run all app tests**

```bash
CGO_ENABLED=1 go test ./internal/app/... -v
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/
git commit -m "feat: add app Scorer (re-scores all items every 60s) and Scheduler (per-integration poll loops)"
```

---

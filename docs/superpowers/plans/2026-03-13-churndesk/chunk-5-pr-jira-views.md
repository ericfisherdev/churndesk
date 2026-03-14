## Chunk 5: PR + Jira Views

Full-page detail views for GitHub PRs and Jira issues. Both fetch live data from their respective APIs on page load, render markdown descriptions and comments, and support posting replies via HTMX. On successful load, all related feed items are marked seen.

---

### Task 21: PR Handler

**Files:**
- Create: `internal/web/handlers/pr.go`
- Create: `internal/web/handlers/pr_test.go`

The PR handler depends on `port.GitHubClient` (live API), `port.ItemStore` (MarkSeenByPR), and `port.LinkStore` (GetJiraKeysForPR). The authenticated username is injected at construction time and controls which action panels are shown in the template.

- [ ] **Step 1: Write the failing test**

```go
// internal/web/handlers/pr_test.go
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

// mockGitHubClient for PR handler tests.
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

// stubPRItemStore for handler tests.
type stubPRItemStore struct{ seenPRs []string }

func (s *stubPRItemStore) MarkSeenByPR(_ context.Context, owner, repo string, number int) error {
	s.seenPRs = append(s.seenPRs, fmt.Sprintf("%s/%s/%d", owner, repo, number))
	return nil
}

// stubLinkStore for handler tests.
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

// stubTeammateStore for PR handler — lists teammates to populate Request Reviewers panel.
type stubTeammateStore struct{ teammates []domain.Teammate }

func (s *stubTeammateStore) ListTeammates(_ context.Context, _ int) ([]domain.Teammate, error) {
	return s.teammates, nil
}
func (s *stubTeammateStore) ListIntegrations(_ context.Context) ([]domain.Integration, error) {
	return []domain.Integration{{ID: 1, Provider: domain.ProviderGitHub, Username: "alice", Enabled: true}}, nil
}

func newTestPRHandler(ghClient *mockGitHubClient, items *stubPRItemStore, links *stubLinkStore, integrations *stubTeammateStore) *handlers.PRHandler {
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
	integrations := &stubTeammateStore{}

	h := newTestPRHandler(client, items, links, integrations)
	req := httptest.NewRequest("GET", "/prs/myorg/myrepo/42", nil)
	req.SetPathValue("owner", "myorg")
	req.SetPathValue("repo", "myrepo")
	req.SetPathValue("number", "42")
	rec := httptest.NewRecorder()
	h.Page(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Fix auth")
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
	h := newTestPRHandler(client, items, &stubLinkStore{}, &stubTeammateStore{})

	req := httptest.NewRequest("GET", "/prs/myorg/myrepo/7", nil)
	req.SetPathValue("owner", "myorg")
	req.SetPathValue("repo", "myrepo")
	req.SetPathValue("number", "7")
	h.Page(httptest.NewRecorder(), req)

	// MarkSeenByPR must be called on successful fetch
	assert.Len(t, items.seenPRs, 1)
}

func TestPRHandler_PostComment_ReturnsPartial(t *testing.T) {
	client := &mockGitHubClient{}
	h := newTestPRHandler(client, &stubPRItemStore{}, &stubLinkStore{}, &stubTeammateStore{})

	req := httptest.NewRequest("POST", "/prs/myorg/myrepo/42/comments", nil)
	req.SetPathValue("owner", "myorg")
	req.SetPathValue("repo", "myrepo")
	req.SetPathValue("number", "42")
	req.Form = map[string][]string{"body": {"Hello!"}}
	rec := httptest.NewRecorder()
	h.PostComment(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// Response must be a comment partial (not a full page)
	body := rec.Body.String()
	assert.NotContains(t, body, "<!DOCTYPE html>", "PostComment must return a partial, not a full page")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/web/handlers/... -run TestPRHandler
```
Expected: FAIL

- [ ] **Step 3: Implement `pr.go`**

```go
// internal/web/handlers/pr.go
package handlers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/markdown"
	"github.com/churndesk/churndesk/internal/web/templates"
)

// PRGitHubClient is the subset of port.GitHubClient used by PRHandler.
type PRGitHubClient interface {
	GetPR(ctx context.Context, owner, repo string, number int) (*domain.PRDetail, error)
	ListPRComments(ctx context.Context, owner, repo string, number int) ([]domain.Comment, error)
	ListPRReviews(ctx context.Context, owner, repo string, number int) ([]domain.Review, error)
	ListCheckRuns(ctx context.Context, owner, repo, headSHA string) ([]domain.CheckRun, error)
	PostPRComment(ctx context.Context, owner, repo string, number int, body string) error
	SubmitReview(ctx context.Context, owner, repo string, number int, verdict, body string) error
	RequestReviewers(ctx context.Context, owner, repo string, number int, logins []string) error
}

// PRItemStore is the subset of port.ItemStore used by PRHandler.
type PRItemStore interface {
	MarkSeenByPR(ctx context.Context, prOwner, prRepo string, prNumber int) error
}

// PRLinkStore is the subset of port.LinkStore used by PRHandler.
type PRLinkStore interface {
	GetJiraKeysForPR(ctx context.Context, prOwner, prRepo string, prNumber int) ([]string, error)
}

// PRIntegrationStore is the subset of port.IntegrationStore used by PRHandler
// (to list teammates for the Request Reviewers panel).
type PRIntegrationStore interface {
	ListIntegrations(ctx context.Context) ([]domain.Integration, error)
	ListTeammates(ctx context.Context, integrationID int) ([]domain.Teammate, error)
}

// PRHandler handles the full PR detail view and PR action endpoints.
type PRHandler struct {
	gh                PRGitHubClient
	items             PRItemStore
	links             PRLinkStore
	integrations      PRIntegrationStore
	authenticatedUser string // GitHub username of the logged-in user
}

// NewPRHandler constructs a PRHandler with all dependencies injected.
func NewPRHandler(gh PRGitHubClient, items PRItemStore, links PRLinkStore, integrations PRIntegrationStore, authenticatedUser string) *PRHandler {
	return &PRHandler{gh: gh, items: items, links: links, integrations: integrations, authenticatedUser: authenticatedUser}
}

// Page renders the full PR detail view (GET /prs/:owner/:repo/:number).
// Fetches live data from GitHub. On success, marks all related feed items as seen.
// On API error, renders an error page with HTTP 200 (HTMX pattern).
func (h *PRHandler) Page(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		h.renderError(w, r, "Invalid PR number")
		return
	}

	pr, err := h.gh.GetPR(r.Context(), owner, repo, number)
	if err != nil {
		log.Printf("get PR %s/%s#%d: %v", owner, repo, number, err)
		h.renderError(w, r, "Failed to load PR from GitHub")
		return
	}

	comments, err := h.gh.ListPRComments(r.Context(), owner, repo, number)
	if err != nil {
		log.Printf("list PR comments: %v", err)
		h.renderError(w, r, "Failed to load PR comments")
		return
	}

	reviews, err := h.gh.ListPRReviews(r.Context(), owner, repo, number)
	if err != nil {
		log.Printf("list PR reviews: %v", err)
		h.renderError(w, r, "Failed to load PR reviews")
		return
	}

	checks, err := h.gh.ListCheckRuns(r.Context(), owner, repo, pr.HeadSHA)
	if err != nil {
		log.Printf("list check runs: %v", err)
		h.renderError(w, r, "Failed to load CI checks")
		return
	}

	jiraKeys, _ := h.links.GetJiraKeysForPR(r.Context(), owner, repo, number)
	teammates := h.listTeammates(r.Context())

	// Render markdown for PR body
	bodyHTML := markdown.Render(pr.Body)

	// Mark all related feed items as seen — only on successful API fetch
	if err := h.items.MarkSeenByPR(r.Context(), owner, repo, number); err != nil {
		log.Printf("mark seen by PR: %v", err)
	}

	data := templates.PRPageData{
		PR:                pr,
		BodyHTML:          bodyHTML,
		Comments:          renderComments(comments),
		Reviews:           reviews,
		Checks:            checks,
		JiraKeys:          jiraKeys,
		Teammates:         teammates,
		AuthenticatedUser: h.authenticatedUser,
	}
	if err := templates.PRPage(data).Render(r.Context(), w); err != nil {
		log.Printf("pr page render: %v", err)
	}
}

// PostComment posts a comment and returns a single comment HTML partial (POST /prs/:owner/:repo/:number/comments).
func (h *PRHandler) PostComment(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || r.FormValue("body") == "" {
		templates.ErrorPartial("Invalid request").Render(r.Context(), w)
		return
	}

	if err := h.gh.PostPRComment(r.Context(), owner, repo, number, r.FormValue("body")); err != nil {
		log.Printf("post PR comment: %v", err)
		templates.ErrorPartial("Failed to post comment").Render(r.Context(), w)
		return
	}

	comment := domain.Comment{
		Author:    h.authenticatedUser,
		Body:      r.FormValue("body"),
		CreatedAt: time.Now(),
	}
	rendered := markdown.Render(comment.Body)
	templates.CommentPartial(comment.Author, comment.Body, rendered, comment.CreatedAt).Render(r.Context(), w)
}

// SubmitReview submits an approve/request-changes/comment review (POST /prs/:owner/:repo/:number/reviews).
func (h *PRHandler) SubmitReview(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	number, _ := strconv.Atoi(r.PathValue("number"))

	verdict := r.FormValue("verdict") // "APPROVE", "REQUEST_CHANGES", "COMMENT"
	body := r.FormValue("body")

	if err := h.gh.SubmitReview(r.Context(), owner, repo, number, verdict, body); err != nil {
		log.Printf("submit review: %v", err)
		templates.ErrorPartial("Failed to submit review").Render(r.Context(), w)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// RequestReviewers requests reviewers for a PR (POST /prs/:owner/:repo/:number/reviewers).
func (h *PRHandler) RequestReviewers(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	number, _ := strconv.Atoi(r.PathValue("number"))

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	logins := r.Form["logins"] // multi-value form field

	if err := h.gh.RequestReviewers(r.Context(), owner, repo, number, logins); err != nil {
		log.Printf("request reviewers: %v", err)
		templates.ErrorPartial("Failed to request reviewers").Render(r.Context(), w)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *PRHandler) renderError(w http.ResponseWriter, r *http.Request, msg string) {
	templates.ErrorPage(msg).Render(r.Context(), w)
}

func (h *PRHandler) listTeammates(ctx context.Context) []domain.Teammate {
	integrations, err := h.integrations.ListIntegrations(ctx)
	if err != nil {
		return nil
	}
	var out []domain.Teammate
	for _, ig := range integrations {
		if ig.Provider != domain.ProviderGitHub {
			continue
		}
		ts, _ := h.integrations.ListTeammates(ctx, ig.ID)
		out = append(out, ts...)
	}
	return out
}

// renderComments converts raw comments into rendered HTML for the template.
func renderComments(comments []domain.Comment) []templates.RenderedComment {
	out := make([]templates.RenderedComment, 0, len(comments))
	for _, c := range comments {
		out = append(out, templates.RenderedComment{
			Comment: c,
			BodyHTML: markdown.Render(c.Body),
		})
	}
	return out
}
```

- [ ] **Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/web/handlers/... -run TestPRHandler
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/handlers/pr.go internal/web/handlers/pr_test.go
git commit -m "feat: add PR handler — live GitHub PR view with markdown, reviews, checks, HTMX comment reply"
```

---

### Task 22: PR Template

**Files:**
- Create: `internal/web/templates/pr.templ`

Data types (`PRPageData`, `RenderedComment`) are plain Go structs defined in the same package and used by both the handler and the template. Define them in `pr.templ` (they compile to the `templates` package).

- [ ] **Step 1: Write `pr.templ`**

```go
// internal/web/templates/pr.templ
package templates

import (
	"fmt"
	"html/template"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
)

// RenderedComment is a domain.Comment with its body pre-rendered to HTML.
type RenderedComment struct {
	domain.Comment
	BodyHTML template.HTML
}

// PRPageData carries all data needed to render the PR detail page.
type PRPageData struct {
	PR                *domain.PRDetail
	BodyHTML          template.HTML
	Comments          []RenderedComment
	Reviews           []domain.Review
	Checks            []domain.CheckRun
	JiraKeys          []string
	Teammates         []domain.Teammate
	AuthenticatedUser string
}

// PRPage renders the full PR detail page.
templ PRPage(d PRPageData) {
	@Layout(fmt.Sprintf("PR #%d — %s", d.PR.Number, d.PR.Title)) {
		<div class="pr-detail" style="max-width:1100px;margin:0 auto;padding:20px 24px">
			<!-- Header -->
			<div style="display:flex;align-items:flex-start;gap:12px;margin-bottom:16px">
				<div style="flex:1">
					<a href="/" style="font-size:12px;color:var(--muted);text-decoration:none">← Back to inbox</a>
					<h1 style="font-size:18px;font-weight:600;margin:6px 0 4px">{ d.PR.Title }</h1>
					<div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
						@prStatusBadge(d.PR.State)
						<span class="pill pill-muted">{ d.PR.Branch } → { d.PR.BaseBranch }</span>
						<span style="color:var(--muted);font-size:12px">by { d.PR.Author }</span>
					</div>
				</div>
			</div>
			<!-- Pills row: repo, diff stats, CI -->
			<div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:16px">
				<span class="pill pill-muted">{ d.PR.Owner }/{ d.PR.Repo } #{ fmt.Sprintf("%d", d.PR.Number) }</span>
				<span class="pill pill-green">+{ fmt.Sprintf("%d", d.PR.Additions) }</span>
				<span class="pill pill-red">-{ fmt.Sprintf("%d", d.PR.Deletions) }</span>
				<span class="pill pill-muted">{ fmt.Sprintf("%d", d.PR.FilesChanged) } files</span>
				@ciStatusBadge(d.Checks)
			</div>
			<!-- Linked Jira issues -->
			if len(d.JiraKeys) > 0 {
				<div style="display:flex;gap:6px;align-items:center;margin-bottom:16px;padding:10px 12px;background:var(--card);border:1px solid var(--border);border-radius:8px">
					<span style="font-size:12px;color:var(--muted);margin-right:4px">Jira:</span>
					for _, key := range d.JiraKeys {
						<a href={ templ.SafeURL("/jira/" + key) } class="pill pill-amber">{ key }</a>
					}
				</div>
			}
			<!-- Main content: 2-column layout -->
			<div style="display:grid;grid-template-columns:1fr 320px;gap:20px;align-items:start">
				<!-- Left: description + comments -->
				<div>
					if d.PR.Body != "" {
						<div class="prose" style="background:var(--card);border:1px solid var(--border);border-radius:8px;padding:16px;margin-bottom:16px">
							@templ.Raw(string(d.BodyHTML))
						</div>
					}
					<div id="comment-list">
						for _, c := range d.Comments {
							@prCommentBlock(c)
						}
					</div>
					<!-- Reply form -->
					<div style="margin-top:16px;background:var(--card);border:1px solid var(--border);border-radius:8px;padding:16px">
						<form
							hx-post={ fmt.Sprintf("/prs/%s/%s/%d/comments", d.PR.Owner, d.PR.Repo, d.PR.Number) }
							hx-target="#comment-list"
							hx-swap="beforeend">
							<textarea
								name="body"
								placeholder="Leave a comment…"
								style="width:100%;min-height:80px;background:var(--surface);border:1px solid var(--border-solid);border-radius:6px;color:var(--text);padding:8px;font-size:13px;resize:vertical"></textarea>
							<div style="display:flex;justify-content:flex-end;margin-top:8px">
								<button type="submit" class="btn btn-primary">Comment</button>
							</div>
						</form>
					</div>
					<!-- Review bar (not shown for own PRs) -->
					if d.PR.Author != d.AuthenticatedUser {
						<div style="margin-top:16px;background:var(--card);border:1px solid var(--border);border-radius:8px;padding:16px">
							<h3 style="font-size:13px;font-weight:600;margin-bottom:12px">Submit review</h3>
							<form hx-post={ fmt.Sprintf("/prs/%s/%s/%d/reviews", d.PR.Owner, d.PR.Repo, d.PR.Number) } hx-swap="none">
								<div style="display:flex;gap:8px;margin-bottom:8px">
									<label><input type="radio" name="verdict" value="APPROVE"/> Approve</label>
									<label><input type="radio" name="verdict" value="REQUEST_CHANGES"/> Request changes</label>
									<label><input type="radio" name="verdict" value="COMMENT"/> Comment</label>
								</div>
								<textarea name="body" placeholder="Review comment (optional)" style="width:100%;min-height:60px;background:var(--surface);border:1px solid var(--border-solid);border-radius:6px;color:var(--text);padding:8px;font-size:13px"></textarea>
								<div style="display:flex;justify-content:flex-end;margin-top:8px">
									<button type="submit" class="btn btn-primary">Submit review</button>
								</div>
							</form>
						</div>
					}
					<!-- Request Reviewers panel (own PRs only) -->
					if d.PR.Author == d.AuthenticatedUser && len(d.Teammates) > 0 {
						<div style="margin-top:16px;background:var(--card);border:1px solid var(--border);border-radius:8px;padding:16px">
							<h3 style="font-size:13px;font-weight:600;margin-bottom:12px">Request reviewers</h3>
							<form hx-post={ fmt.Sprintf("/prs/%s/%s/%d/reviewers", d.PR.Owner, d.PR.Repo, d.PR.Number) } hx-swap="none">
								<div style="display:flex;flex-direction:column;gap:6px;margin-bottom:12px">
									for _, t := range d.Teammates {
										<label style="display:flex;align-items:center;gap:8px;font-size:13px">
											<input type="checkbox" name="logins" value={ t.GitHubUsername }/>
											{ t.DisplayName } <span style="color:var(--muted)">({ t.GitHubUsername })</span>
										</label>
									}
								</div>
								<button type="submit" class="btn">Request</button>
							</form>
						</div>
					}
				</div>
				<!-- Right sidebar -->
				<div>
					<!-- Reviewers -->
					<div style="background:var(--card);border:1px solid var(--border);border-radius:8px;padding:14px;margin-bottom:12px">
						<h3 style="font-size:12px;font-weight:600;color:var(--muted);margin-bottom:10px;text-transform:uppercase;letter-spacing:0.5px">Reviewers</h3>
						if len(d.Reviews) == 0 {
							<p style="font-size:12px;color:var(--muted)">No reviews yet</p>
						}
						for _, r := range d.Reviews {
							<div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
								<span style="font-size:13px">{ r.Author }</span>
								@reviewStateBadge(r.State)
							</div>
						}
					</div>
					<!-- CI Checks -->
					<div style="background:var(--card);border:1px solid var(--border);border-radius:8px;padding:14px">
						<h3 style="font-size:12px;font-weight:600;color:var(--muted);margin-bottom:10px;text-transform:uppercase;letter-spacing:0.5px">Checks</h3>
						if len(d.Checks) == 0 {
							<p style="font-size:12px;color:var(--muted)">No checks</p>
						}
						for _, c := range d.Checks {
							<div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
								@checkBadge(c.Conclusion)
								<span style="font-size:12px">{ c.Name }</span>
							</div>
						}
					</div>
				</div>
			</div>
		</div>
	}
}

templ prCommentBlock(c RenderedComment) {
	<div class="comment" style="padding:14px 0;border-top:1px solid var(--border)">
		<div style="display:flex;gap:10px;align-items:center;margin-bottom:8px">
			<span style="font-weight:600;font-size:13px">{ c.Author }</span>
			<span style="color:var(--muted);font-size:12px">{ c.CreatedAt.Format("Jan 2, 2006 15:04") }</span>
		</div>
		<div class="prose">@templ.Raw(string(c.BodyHTML))</div>
	</div>
}

templ prStatusBadge(state string) {
	switch state {
	case "open":
		<span class="pill pill-green">Open</span>
	case "merged":
		<span class="pill pill-accent">Merged</span>
	default:
		<span class="pill pill-muted">Closed</span>
	}
}

templ reviewStateBadge(state string) {
	switch state {
	case "APPROVED":
		<span class="pill pill-green" style="font-size:11px">Approved</span>
	case "CHANGES_REQUESTED":
		<span class="pill pill-amber" style="font-size:11px">Changes requested</span>
	case "DISMISSED":
		<span class="pill pill-muted" style="font-size:11px">Dismissed</span>
	default:
		<span class="pill pill-muted" style="font-size:11px">{ state }</span>
	}
}

templ checkBadge(conclusion string) {
	switch conclusion {
	case "success":
		<span style="color:var(--green)">✓</span>
	case "failure", "action_required":
		<span style="color:var(--red)">✗</span>
	default:
		<span style="color:var(--muted)">○</span>
	}
}

templ ciStatusBadge(checks []domain.CheckRun) {
	if len(checks) == 0 {
		<!-- no badge -->
	} else {
		if anyCheckFailing(checks) {
			<span class="pill pill-red">CI failing</span>
		} else if allChecksPassing(checks) {
			<span class="pill pill-green">CI passing</span>
		} else {
			<span class="pill pill-muted">CI pending</span>
		}
	}
}

// ErrorPage renders a full-page error with layout (used when live API fails).
// HTTP status is always 200 — HTMX does not swap on non-2xx.
templ ErrorPage(message string) {
	@Layout("Error") {
		<div style="max-width:600px;margin:60px auto;padding:0 24px">
			<div style="background:var(--card);border:1px solid var(--red);border-radius:10px;padding:24px">
				<h2 style="color:var(--red);font-size:16px;margin-bottom:8px">Something went wrong</h2>
				<p style="color:var(--muted);font-size:13px;margin-bottom:16px">{ message }</p>
				<a href="/" class="btn">← Back to inbox</a>
			</div>
		</div>
	}
}

// ── Template helpers ──────────────────────────────────────────────────────────

func anyCheckFailing(checks []domain.CheckRun) bool {
	for _, c := range checks {
		if c.Status == "completed" && (c.Conclusion == "failure" || c.Conclusion == "action_required") {
			return true
		}
	}
	return false
}

func allChecksPassing(checks []domain.CheckRun) bool {
	for _, c := range checks {
		if c.Conclusion != "success" {
			return false
		}
	}
	return true
}

// Suppress unused import for time (used in prCommentBlock).
var _ = time.Now
```

- [ ] **Step 2: Compile templates**

```bash
templ generate ./internal/web/templates/...
CGO_ENABLED=1 go build ./...
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/web/templates/pr.templ
git commit -m "feat: add PR detail page template with reviews, CI checks, reply form, and review bar"
```

---

### Task 23: Jira Handler

**Files:**
- Create: `internal/web/handlers/jira.go`
- Create: `internal/web/handlers/jira_test.go`

Mirrors the PR handler pattern: live API fetch, mark seen on success, HTMX comment partial.

- [ ] **Step 1: Write the failing test**

```go
// internal/web/handlers/jira_test.go
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

// mockJiraClient for Jira handler tests.
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

// stubJiraItemStore for Jira handler tests.
type stubJiraItemStore struct{ seenKeys []string }

func (s *stubJiraItemStore) MarkSeenByJiraKey(_ context.Context, key string) error {
	s.seenKeys = append(s.seenKeys, key)
	return nil
}

// stubJiraLinkStore for Jira handler tests.
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/web/handlers/... -run TestJiraHandler
```
Expected: FAIL

- [ ] **Step 3: Implement `jira.go`**

```go
// internal/web/handlers/jira.go
package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/markdown"
	"github.com/churndesk/churndesk/internal/web/templates"
)


// JiraAPIClient is the subset of port.JiraClient used by JiraHandler.
type JiraAPIClient interface {
	GetIssue(ctx context.Context, key string) (*domain.JiraIssue, error)
	ListIssueComments(ctx context.Context, key string) ([]domain.Comment, error)
	PostComment(ctx context.Context, key string, body string) error
}

// JiraItemStore is the subset of port.ItemStore used by JiraHandler.
type JiraItemStore interface {
	MarkSeenByJiraKey(ctx context.Context, jiraKey string) error
}

// JiraLinkStore is the subset of port.LinkStore used by JiraHandler.
type JiraLinkStore interface {
	GetPRsForJiraKey(ctx context.Context, jiraKey string) ([]domain.PRRef, error)
}

// JiraHandler handles the full Jira issue detail view and comment endpoint.
type JiraHandler struct {
	jira  JiraAPIClient
	items JiraItemStore
	links JiraLinkStore
}

// NewJiraHandler constructs a JiraHandler with all dependencies injected.
func NewJiraHandler(jira JiraAPIClient, items JiraItemStore, links JiraLinkStore) *JiraHandler {
	return &JiraHandler{jira: jira, items: items, links: links}
}

// Page renders the full Jira issue view (GET /jira/:key).
// Fetches live data. On success, marks all related feed items as seen.
// On API error, renders an error page with HTTP 200.
func (h *JiraHandler) Page(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	issue, err := h.jira.GetIssue(r.Context(), key)
	if err != nil || issue == nil {
		log.Printf("get Jira issue %s: %v", key, err)
		templates.ErrorPage("Failed to load Jira issue").Render(r.Context(), w)
		return
	}

	comments, err := h.jira.ListIssueComments(r.Context(), key)
	if err != nil {
		log.Printf("list Jira comments for %s: %v", key, err)
		templates.ErrorPage("Failed to load comments").Render(r.Context(), w)
		return
	}

	linkedPRs, _ := h.links.GetPRsForJiraKey(r.Context(), key)

	descHTML := markdown.Render(issue.Description)

	// Mark seen on successful fetch only
	if err := h.items.MarkSeenByJiraKey(r.Context(), key); err != nil {
		log.Printf("mark seen jira %s: %v", key, err)
	}

	data := templates.JiraPageData{
		Issue:      issue,
		DescHTML:   descHTML,
		Comments:   renderComments(comments),
		LinkedPRs:  linkedPRs,
	}
	if err := templates.JiraPage(data).Render(r.Context(), w); err != nil {
		log.Printf("jira page render: %v", err)
	}
}

// PostComment posts a comment and returns a single comment HTML partial (POST /jira/:key/comments).
func (h *JiraHandler) PostComment(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	body := r.FormValue("body")
	if body == "" {
		templates.ErrorPartial("Comment body required").Render(r.Context(), w)
		return
	}

	if err := h.jira.PostComment(r.Context(), key, body); err != nil {
		log.Printf("post Jira comment on %s: %v", key, err)
		templates.ErrorPartial("Failed to post comment").Render(r.Context(), w)
		return
	}

	comment := domain.Comment{
		Author:    "me", // Jira account display name not available here; handler can be extended
		Body:      body,
		CreatedAt: time.Now(),
	}
	rendered := markdown.Render(comment.Body)
	templates.CommentPartial(comment.Author, comment.Body, rendered, comment.CreatedAt).Render(r.Context(), w)
}
```

- [ ] **Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/web/handlers/... -run TestJiraHandler
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/handlers/jira.go internal/web/handlers/jira_test.go
git commit -m "feat: add Jira handler — live issue view with markdown, comments, and HTMX reply"
```

---

### Task 24: Jira Template

**Files:**
- Create: `internal/web/templates/jira.templ`

- [ ] **Step 1: Write `jira.templ`**

```go
// internal/web/templates/jira.templ
package templates

import (
	"fmt"
	"html/template"

	"github.com/churndesk/churndesk/internal/domain"
)

// JiraPageData carries all data needed to render the Jira issue detail page.
type JiraPageData struct {
	Issue     *domain.JiraIssue
	DescHTML  template.HTML
	Comments  []RenderedComment
	LinkedPRs []domain.PRRef
}

// JiraPage renders the full Jira issue detail page.
templ JiraPage(d JiraPageData) {
	@Layout(fmt.Sprintf("%s — %s", d.Issue.Key, d.Issue.Summary)) {
		<div style="max-width:1100px;margin:0 auto;padding:20px 24px">
			<!-- Header -->
			<div style="margin-bottom:16px">
				<a href="/" style="font-size:12px;color:var(--muted);text-decoration:none">← Back to inbox</a>
				<div style="display:flex;align-items:center;gap:10px;margin-top:8px;flex-wrap:wrap">
					<span class="pill pill-muted" style="font-family:'JetBrains Mono',monospace;font-size:12px">{ d.Issue.Key }</span>
					@issueTypeBadge(d.Issue.IssueType)
					@jiraStatusBadge(d.Issue.Status)
				</div>
				<h1 style="font-size:18px;font-weight:600;margin:8px 0 0">{ d.Issue.Summary }</h1>
			</div>
			<!-- Linked PRs bar -->
			if len(d.LinkedPRs) > 0 {
				<div style="display:flex;gap:6px;align-items:center;margin-bottom:16px;padding:10px 12px;background:var(--card);border:1px solid var(--border);border-radius:8px;flex-wrap:wrap">
					<span style="font-size:12px;color:var(--muted);margin-right:4px">PRs:</span>
					for _, pr := range d.LinkedPRs {
						<a href={ templ.SafeURL(fmt.Sprintf("/prs/%s/%s/%d", pr.Owner, pr.Repo, pr.Number)) } class="pill pill-accent" style="font-size:12px">
							#{ fmt.Sprintf("%d", pr.Number) } { pr.Title }
						</a>
					}
				</div>
			}
			<!-- Main content: 2-column layout -->
			<div style="display:grid;grid-template-columns:1fr 280px;gap:20px;align-items:start">
				<!-- Left: description + comments -->
				<div>
					if d.Issue.Description != "" {
						<div class="prose" style="background:var(--card);border:1px solid var(--border);border-radius:8px;padding:16px;margin-bottom:16px">
							@templ.Raw(string(d.DescHTML))
						</div>
					}
					<div id="comment-list">
						for _, c := range d.Comments {
							@jiraCommentBlock(c)
						}
					</div>
					<!-- Reply form -->
					<div style="margin-top:16px;background:var(--card);border:1px solid var(--border);border-radius:8px;padding:16px">
						<form
							hx-post={ fmt.Sprintf("/jira/%s/comments", d.Issue.Key) }
							hx-target="#comment-list"
							hx-swap="beforeend">
							<textarea
								name="body"
								placeholder="Add a comment…"
								style="width:100%;min-height:80px;background:var(--surface);border:1px solid var(--border-solid);border-radius:6px;color:var(--text);padding:8px;font-size:13px;resize:vertical"></textarea>
							<div style="display:flex;justify-content:flex-end;margin-top:8px">
								<button type="submit" class="btn btn-primary">Comment</button>
							</div>
						</form>
					</div>
				</div>
				<!-- Right sidebar: issue metadata -->
				<div>
					<div style="background:var(--card);border:1px solid var(--border);border-radius:8px;padding:14px">
						<h3 style="font-size:12px;font-weight:600;color:var(--muted);margin-bottom:12px;text-transform:uppercase;letter-spacing:0.5px">Details</h3>
						@jiraMeta("Status",   d.Issue.Status)
						@jiraMeta("Priority", d.Issue.Priority)
						@jiraMeta("Type",     d.Issue.IssueType)
						@jiraMeta("Assignee", d.Issue.Assignee)
						@jiraMeta("Sprint",   d.Issue.Sprint)
						if d.Issue.StoryPoints > 0 {
							@jiraMeta("Points", fmt.Sprintf("%.0f", d.Issue.StoryPoints))
						}
						@jiraMeta("Created", d.Issue.CreatedAt.Format("Jan 2, 2006"))
						@jiraMeta("Updated", d.Issue.UpdatedAt.Format("Jan 2, 2006"))
					</div>
				</div>
			</div>
		</div>
	}
}

templ jiraCommentBlock(c RenderedComment) {
	<div class="comment" style="padding:14px 0;border-top:1px solid var(--border)">
		<div style="display:flex;gap:10px;align-items:center;margin-bottom:8px">
			<span style="font-weight:600;font-size:13px">{ c.Author }</span>
			<span style="color:var(--muted);font-size:12px">{ c.CreatedAt.Format("Jan 2, 2006 15:04") }</span>
		</div>
		<div class="prose">@templ.Raw(string(c.BodyHTML))</div>
	</div>
}

templ jiraMeta(label, value string) {
	if value != "" {
		<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;font-size:12px">
			<span style="color:var(--muted)">{ label }</span>
			<span>{ value }</span>
		</div>
	}
}

templ issueTypeBadge(issueType string) {
	switch issueType {
	case "Bug":
		<span class="pill pill-red">Bug</span>
	case "Story":
		<span class="pill pill-accent">Story</span>
	case "Task":
		<span class="pill pill-muted">Task</span>
	default:
		<span class="pill pill-muted">{ issueType }</span>
	}
}

templ jiraStatusBadge(status string) {
	switch status {
	case "Done", "Resolved", "Closed":
		<span class="pill pill-green">{ status }</span>
	case "In Progress":
		<span class="pill pill-accent">In Progress</span>
	default:
		<span class="pill pill-muted">{ status }</span>
	}
}

// Suppress unused import warning for fmt (used in template expressions above).
var _ = fmt.Sprintf
```

- [ ] **Step 2: Compile templates and verify build**

```bash
templ generate ./internal/web/templates/...
CGO_ENABLED=1 go build ./...
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/web/templates/jira.templ
git commit -m "feat: add Jira issue detail page template with linked PRs, metadata sidebar, and reply form"
```

---

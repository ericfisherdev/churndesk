// internal/web/handlers/pr.go
package handlers

import (
	"context"
	"html/template"
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

// PRIntegrationStore is the subset of port.IntegrationStore used by PRHandler.
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
	authenticatedUser string
}

// NewPRHandler constructs a PRHandler with all dependencies injected.
func NewPRHandler(gh PRGitHubClient, items PRItemStore, links PRLinkStore, integrations PRIntegrationStore, authenticatedUser string) *PRHandler {
	return &PRHandler{gh: gh, items: items, links: links, integrations: integrations, authenticatedUser: authenticatedUser}
}

// Page renders the full PR detail view.
func (h *PRHandler) Page(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid PR number")
		return
	}

	pr, err := h.gh.GetPR(r.Context(), owner, repo, number)
	if err != nil {
		log.Printf("get PR %s/%s#%d: %v", owner, repo, number, err)
		h.renderError(w, r, http.StatusBadGateway, "Failed to load PR from GitHub")
		return
	}

	comments, err := h.gh.ListPRComments(r.Context(), owner, repo, number)
	if err != nil {
		log.Printf("list PR comments: %v", err)
		h.renderError(w, r, http.StatusBadGateway, "Failed to load PR comments")
		return
	}

	reviews, err := h.gh.ListPRReviews(r.Context(), owner, repo, number)
	if err != nil {
		log.Printf("list PR reviews: %v", err)
		h.renderError(w, r, http.StatusBadGateway, "Failed to load PR reviews")
		return
	}

	checks, err := h.gh.ListCheckRuns(r.Context(), owner, repo, pr.HeadSHA)
	if err != nil {
		log.Printf("list check runs: %v", err)
		h.renderError(w, r, http.StatusBadGateway, "Failed to load CI checks")
		return
	}

	jiraKeys, err := h.links.GetJiraKeysForPR(r.Context(), owner, repo, number)
	if err != nil {
		log.Printf("get jira keys for PR %s/%s#%d: %v", owner, repo, number, err)
	}
	teammates := h.listTeammates(r.Context())
	bodyHTML := template.HTML(markdown.Render(pr.Body)) //nolint:gosec

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

// PostComment posts a comment and returns a comment HTML partial.
func (h *PRHandler) PostComment(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templates.ErrorPartial("Invalid PR number").Render(r.Context(), w) //nolint:errcheck
		return
	}
	if r.FormValue("body") == "" {
		w.WriteHeader(http.StatusBadRequest)
		templates.ErrorPartial("Comment body cannot be empty").Render(r.Context(), w) //nolint:errcheck
		return
	}

	if err := h.gh.PostPRComment(r.Context(), owner, repo, number, r.FormValue("body")); err != nil {
		log.Printf("post PR comment: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		templates.ErrorPartial("Failed to post comment").Render(r.Context(), w) //nolint:errcheck
		return
	}

	comment := domain.Comment{
		Author:    h.authenticatedUser,
		Body:      r.FormValue("body"),
		CreatedAt: time.Now(),
	}
	rendered := template.HTML(markdown.Render(comment.Body)) //nolint:gosec
	templates.CommentPartial(comment.Author, comment.Body, rendered, comment.CreatedAt).Render(r.Context(), w) //nolint:errcheck
}

// SubmitReview submits a PR review.
func (h *PRHandler) SubmitReview(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templates.ErrorPartial("Invalid PR number").Render(r.Context(), w) //nolint:errcheck
		return
	}

	verdict := r.FormValue("verdict")
	switch verdict {
	case "APPROVE", "REQUEST_CHANGES", "COMMENT":
		// valid
	default:
		w.WriteHeader(http.StatusBadRequest)
		templates.ErrorPartial("Invalid review verdict").Render(r.Context(), w) //nolint:errcheck
		return
	}

	if err := h.gh.SubmitReview(r.Context(), owner, repo, number, verdict, r.FormValue("body")); err != nil {
		log.Printf("submit review: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		templates.ErrorPartial("Failed to submit review").Render(r.Context(), w) //nolint:errcheck
		return
	}
	w.WriteHeader(http.StatusOK)
}

// RequestReviewers requests reviewers for a PR.
func (h *PRHandler) RequestReviewers(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	logins := r.Form["logins"]
	if len(logins) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		templates.ErrorPartial("Please select at least one reviewer").Render(r.Context(), w) //nolint:errcheck
		return
	}

	if err := h.gh.RequestReviewers(r.Context(), owner, repo, number, logins); err != nil {
		log.Printf("request reviewers: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		templates.ErrorPartial("Failed to request reviewers").Render(r.Context(), w) //nolint:errcheck
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *PRHandler) renderError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	w.WriteHeader(status)
	templates.ErrorPage(msg).Render(r.Context(), w) //nolint:errcheck
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

func renderComments(comments []domain.Comment) []templates.RenderedComment {
	out := make([]templates.RenderedComment, 0, len(comments))
	for _, c := range comments {
		out = append(out, templates.RenderedComment{
			Comment:  c,
			BodyHTML: template.HTML(markdown.Render(c.Body)), //nolint:gosec
		})
	}
	return out
}

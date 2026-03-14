package handlers

import (
	"context"
	"html/template"
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
func (h *JiraHandler) Page(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	issue, err := h.jira.GetIssue(r.Context(), key)
	if err != nil || issue == nil {
		log.Printf("get Jira issue %s: %v", key, err)
		templates.ErrorPage("Failed to load Jira issue").Render(r.Context(), w) //nolint:errcheck
		return
	}

	comments, err := h.jira.ListIssueComments(r.Context(), key)
	if err != nil {
		log.Printf("list Jira comments for %s: %v", key, err)
		templates.ErrorPage("Failed to load comments").Render(r.Context(), w) //nolint:errcheck
		return
	}

	linkedPRs, _ := h.links.GetPRsForJiraKey(r.Context(), key)
	descHTML := template.HTML(markdown.Render(issue.Description)) //nolint:gosec

	if err := h.items.MarkSeenByJiraKey(r.Context(), key); err != nil {
		log.Printf("mark seen jira %s: %v", key, err)
	}

	data := templates.JiraPageData{
		Issue:     issue,
		DescHTML:  descHTML,
		Comments:  renderComments(comments),
		LinkedPRs: linkedPRs,
	}
	if err := templates.JiraPage(data).Render(r.Context(), w); err != nil {
		log.Printf("jira page render: %v", err)
	}
}

// PostComment posts a Jira comment and returns a comment HTML partial.
func (h *JiraHandler) PostComment(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	body := r.FormValue("body")
	if body == "" {
		templates.ErrorPartial("Comment body required").Render(r.Context(), w) //nolint:errcheck
		return
	}

	if err := h.jira.PostComment(r.Context(), key, body); err != nil {
		log.Printf("post Jira comment on %s: %v", key, err)
		templates.ErrorPartial("Failed to post comment").Render(r.Context(), w) //nolint:errcheck
		return
	}

	comment := domain.Comment{
		Author:    "me",
		Body:      body,
		CreatedAt: time.Now(),
	}
	rendered := template.HTML(markdown.Render(comment.Body)) //nolint:gosec
	templates.CommentPartial(comment.Author, comment.Body, rendered, comment.CreatedAt).Render(r.Context(), w) //nolint:errcheck
}

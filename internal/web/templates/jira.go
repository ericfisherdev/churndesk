// internal/web/templates/jira.go
// Stub types and functions for JiraHandler — replaced by real templ in Task 24.
package templates

import (
	"context"
	"html/template"
	"io"

	"github.com/churndesk/churndesk/internal/domain"
)

// JiraPageData carries all data needed to render the Jira issue detail page.
type JiraPageData struct {
	Issue     *domain.JiraIssue
	DescHTML  template.HTML
	Comments  []RenderedComment
	LinkedPRs []domain.PRRef
}

type jiraComponent struct{ data JiraPageData }

func (c jiraComponent) Render(_ context.Context, w io.Writer) error {
	if c.data.Issue != nil {
		io.WriteString(w, c.data.Issue.Key) //nolint:errcheck
	}
	return nil
}

// JiraPage returns a component that renders the Jira issue detail view.
func JiraPage(data JiraPageData) interface{ Render(context.Context, io.Writer) error } {
	return jiraComponent{data: data}
}

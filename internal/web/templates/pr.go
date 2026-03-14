// internal/web/templates/pr.go
// Stub templates for PRHandler — replaced by real templ in Task 22.
package templates

import (
	"context"
	"html/template"
	"io"

	"github.com/churndesk/churndesk/internal/domain"
)

// PRPageData holds all data for the PR detail view.
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

// RenderedComment pairs a raw comment with its rendered HTML body.
type RenderedComment struct {
	domain.Comment
	BodyHTML template.HTML
}

type prComponent struct{ data PRPageData }

func (c prComponent) Render(_ context.Context, w io.Writer) error {
	// Stub: write the PR title so tests can assert on it.
	if c.data.PR != nil {
		io.WriteString(w, c.data.PR.Title) //nolint:errcheck
	}
	return nil
}

// PRPage returns a component that renders the PR detail view.
func PRPage(data PRPageData) interface{ Render(context.Context, io.Writer) error } {
	return prComponent{data: data}
}

type errorPageComponent struct{ msg string }

func (c errorPageComponent) Render(_ context.Context, w io.Writer) error {
	io.WriteString(w, c.msg) //nolint:errcheck
	return nil
}

// ErrorPage renders a full-page error. Stub until real templ template is created.
func ErrorPage(msg string) interface{ Render(context.Context, io.Writer) error } {
	return errorPageComponent{msg: msg}
}

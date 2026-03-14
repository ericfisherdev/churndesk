// internal/web/templates/feed.go
// Stub feed template functions — replaced by real templ components in Task 20.
package templates

import (
	"context"
	"io"

	"github.com/churndesk/churndesk/internal/domain"
)

// Component is a renderable template component (mirrors templ.Component).
type Component interface {
	Render(ctx context.Context, w io.Writer) error
}

// noopComponent is a stub component that renders nothing.
type noopComponent struct{}

func (noopComponent) Render(_ context.Context, _ io.Writer) error { return nil }

// FeedPage renders the full feed page (stub until Task 20).
func FeedPage(_ []domain.Item, _ int) Component {
	return noopComponent{}
}

// FeedFragment renders the feed list fragment for HTMX updates (stub until Task 20).
func FeedFragment(_ []domain.Item, _ bool) Component {
	return noopComponent{}
}

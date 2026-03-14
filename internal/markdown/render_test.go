package markdown_test

import (
	"strings"
	"testing"

	"github.com/churndesk/churndesk/internal/markdown"
	"github.com/stretchr/testify/assert"
)

func TestRender_BasicMarkdown(t *testing.T) {
	html := markdown.Render("**bold** and _italic_")
	assert.Contains(t, html, "<strong>bold</strong>")
	assert.Contains(t, html, "<em>italic</em>")
}

func TestRender_CodeFence_ProducesHighlightedHTML(t *testing.T) {
	input := "```go\nfmt.Println(\"hello\")\n```"
	html := markdown.Render(input)
	// chroma wraps highlighted code in a div with class
	assert.Contains(t, html, "fmt")
	assert.Contains(t, html, "Println")
}

func TestRender_ScriptTagStripped(t *testing.T) {
	html := markdown.Render("<script>alert('xss')</script>")
	assert.NotContains(t, html, "<script>")
	assert.NotContains(t, html, "alert")
}

func TestRender_EmptyString(t *testing.T) {
	assert.Equal(t, "", strings.TrimSpace(markdown.Render("")))
}

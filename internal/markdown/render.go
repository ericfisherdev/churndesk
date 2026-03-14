package markdown

import (
	"bytes"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

var (
	md     goldmark.Markdown
	policy *bluemonday.Policy
)

func init() {
	md = goldmark.New(
		goldmark.WithExtensions(
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
				highlighting.WithFormatOptions(
					chromahtml.WithClasses(false),
					chromahtml.WithLineNumbers(false),
				),
			),
		),
	)
	policy = bluemonday.UGCPolicy()
	// Allow chroma's inline style attributes for syntax highlighting
	policy.AllowAttrs("style").OnElements("span", "pre", "code")
	policy.AllowElements("pre", "code")
}

// Render converts markdown to sanitized HTML.
// Uses goldmark for parsing, chroma for syntax highlighting, and bluemonday for XSS sanitization.
func Render(src string) string {
	if src == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return ""
	}
	return policy.Sanitize(buf.String())
}

package markdown

import (
	"bytes"
	"regexp"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/renderer/html"
)

var (
	md            goldmark.Markdown
	policy        *bluemonday.Policy
	emojiShortcode = regexp.MustCompile(`:([a-z0-9_+-]+):`)

	// Common GitHub emoji shortcodes used by bots (CodeRabbit, etc.).
	// Add entries here as new shortcodes appear in practice.
	emojiTable = map[string]string{
		"label":             "🏷️",
		"gear":              "⚙️",
		"warning":           "⚠️",
		"white_check_mark": "✅",
		"x":                "❌",
		"rocket":            "🚀",
		"bug":               "🐛",
		"memo":              "📝",
		"wrench":            "🔧",
		"tada":              "🎉",
		"eyes":              "👀",
		"bulb":              "💡",
		"information_source": "ℹ️",
		"mag":               "🔍",
		"clipboard":         "📋",
		"bookmark":          "🔖",
		"books":             "📚",
		"sparkles":          "✨",
		"arrow_right":       "➡️",
		"heavy_plus_sign":   "➕",
		"heavy_minus_sign":  "➖",
	}
)

func init() {
	md = goldmark.New(
		goldmark.WithRendererOptions(
			// Allow raw HTML (e.g. <details>/<summary>) to pass through.
			html.WithUnsafe(),
		),
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
	// Allow <details>/<summary> for collapsible sections (e.g. CodeRabbit comments)
	policy.AllowElements("details", "summary")
}

// replaceEmoji substitutes GitHub emoji shortcodes (e.g. :label:) with their
// Unicode equivalents before the markdown is parsed. This is necessary because
// goldmark treats raw HTML blocks as opaque and does not process text inside
// them, so shortcodes inside <summary> tags would otherwise appear as literals.
func replaceEmoji(src string) string {
	return emojiShortcode.ReplaceAllStringFunc(src, func(match string) string {
		name := match[1 : len(match)-1]
		if emoji, ok := emojiTable[name]; ok {
			return emoji
		}
		return match
	})
}

// Render converts markdown to sanitized HTML.
// Uses goldmark for parsing, chroma for syntax highlighting, and bluemonday for XSS sanitization.
func Render(src string) string {
	if src == "" {
		return ""
	}
	src = replaceEmoji(src)
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return ""
	}
	return policy.Sanitize(buf.String())
}

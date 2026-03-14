## Chunk 4: Web Foundation + Feed

This chunk wires the frontend layer: markdown rendering, static assets, HTTP server with onboarding middleware, and the feed handlers with their templates. After this chunk, the app is navigable — the feed page renders and auto-refreshes, and dismiss/seen/sync all work.

---

### Task 16: Markdown Renderer

**Files:**
- Create: `internal/markdown/render.go`
- Create: `internal/markdown/render_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/markdown/render_test.go
package markdown_test

import (
	"strings"
	"testing"

	"github.com/churndesk/churndesk/internal/markdown"
	"github.com/stretchr/testify/assert"
)

func TestRender_BasicMarkdown(t *testing.T) {
	out := string(markdown.Render("**hello**"))
	assert.Contains(t, out, "<strong>hello</strong>")
}

func TestRender_SyntaxHighlighting(t *testing.T) {
	src := "```go\nfunc main() {}\n```"
	out := string(markdown.Render(src))
	// chroma wraps code in a pre element
	assert.Contains(t, out, "<pre")
}

func TestRender_Sanitization(t *testing.T) {
	// bluemonday must strip script tags
	out := string(markdown.Render("<script>alert('xss')</script>"))
	assert.NotContains(t, out, "<script>")
	assert.NotContains(t, out, "alert")
}

func TestRender_EmptyInput(t *testing.T) {
	out := string(markdown.Render(""))
	assert.True(t, len(strings.TrimSpace(out)) == 0 || out == "\n")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/markdown/...
```
Expected: FAIL

- [ ] **Step 3: Implement**

```go
// internal/markdown/render.go
package markdown

import (
	"bytes"
	"html/template"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var md = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		highlighting.NewHighlighting(
			highlighting.WithStyle("dracula"),
			highlighting.WithFormatOptions(
				chromahtml.WithClasses(false),
				chromahtml.WithLineNumbers(false),
			),
		),
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		html.WithHardWraps(),
		html.WithXHTML(),
	),
)

var sanitizer = bluemonday.UGCPolicy()

// Render converts markdown src to sanitized HTML safe for inclusion in Templ templates.
func Render(src string) template.HTML {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return template.HTML("")
	}
	return template.HTML(sanitizer.SanitizeBytes(buf.Bytes())) //nolint:gosec // sanitized by bluemonday
}
```

> **Note:** `goldmark-highlighting/v2` is a separate module. Add it: `go get github.com/yuin/goldmark-highlighting/v2`

- [ ] **Step 4: Add dependency and run test**

```bash
go get github.com/yuin/goldmark-highlighting/v2
CGO_ENABLED=1 go test ./internal/markdown/...
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/markdown/ go.mod go.sum
git commit -m "feat: add markdown renderer with goldmark, chroma syntax highlighting, and bluemonday sanitization"
```

---

### Task 17: Static Assets

**Files:**
- Create: `cmd/churndesk/static/style.css`
- Create: `cmd/churndesk/static/app.js`

> **Path note:** Static assets live at `cmd/churndesk/static/` so that `main.go` can embed them with `//go:embed static` without `..` path components (which Go's embed tooling prohibits). The HTTP server serves them at `/static/*` regardless.

No unit tests for static files. The visual result is verified by running the app.

- [ ] **Step 1: Write `cmd/churndesk/static/style.css`**

```css
/* cmd/churndesk/static/style.css */
:root {
  --bg:           #0f0f10;
  --surface:      #16161a;
  --card:         #1c1c22;
  --border:       rgba(255,255,255,0.07);
  --border-solid: #2a2a33;
  --text:         #e8e8ee;
  --muted:        #7b7b8f;
  --accent:       #7c6af7;
  --green:        #34d399;
  --amber:        #fbbf24;
  --red:          #f87171;
  --blue:         #60a5fa;

  --accent-bg:  rgba(124,106,247,0.13);
  --green-bg:   rgba(52,211,153,0.13);
  --amber-bg:   rgba(251,191,36,0.13);
  --red-bg:     rgba(248,113,113,0.13);
  --blue-bg:    rgba(96,165,250,0.13);
}

*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

body {
  background: var(--bg);
  color: var(--text);
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  font-size: 14px;
  line-height: 1.5;
  min-height: 100vh;
}

/* Scrollbar */
::-webkit-scrollbar { width: 6px; height: 6px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: var(--border-solid); border-radius: 3px; }

/* Header */
.header {
  position: sticky; top: 0; z-index: 100;
  background: rgba(15,15,16,0.85);
  backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--border);
  padding: 0 24px;
  height: 52px;
  display: flex; align-items: center; gap: 16px;
}
.header-logo { font-weight: 700; font-size: 15px; letter-spacing: -0.3px; }
.header-spacer { flex: 1; }
.header-actions { display: flex; align-items: center; gap: 8px; }

/* Feed grid */
.feed-grid { display: grid; gap: 12px; padding: 20px 24px; }
.feed-grid[data-cols="1"] { grid-template-columns: 1fr; max-width: 720px; margin: 0 auto; }
.feed-grid[data-cols="2"] { grid-template-columns: repeat(2, 1fr); }
.feed-grid[data-cols="3"] { grid-template-columns: repeat(3, 1fr); }

/* Feed item card */
.feed-item {
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 14px 16px;
  cursor: pointer;
  position: relative;
  transition: border-color 0.15s, background 0.15s;
}
.feed-item:hover { background: #202028; border-color: rgba(255,255,255,0.12); }
.feed-item.unseen::before {
  content: '';
  position: absolute; top: 12px; right: 12px;
  width: 7px; height: 7px;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 6px var(--accent);
}

/* Item type colors */
.feed-item.pr-review-needed  { border-left: 3px solid var(--accent); }
.feed-item.pr-stale-review   { border-left: 3px solid var(--amber); }
.feed-item.pr-changes        { border-left: 3px solid var(--amber); }
.feed-item.pr-comment        { border-left: 3px solid var(--blue); }
.feed-item.pr-ci-failing     { border-left: 3px solid var(--red); }
.feed-item.pr-approved       { border-left: 3px solid var(--green); }
.feed-item.jira-status       { border-left: 3px solid var(--amber); }
.feed-item.jira-comment      { border-left: 3px solid var(--blue); }
.feed-item.jira-bug          { border-left: 3px solid var(--red); }

/* Pills */
.pill {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 2px 8px; border-radius: 999px;
  font-size: 11px; font-weight: 500; white-space: nowrap;
}
.pill-accent  { background: var(--accent-bg); color: var(--accent); }
.pill-green   { background: var(--green-bg);  color: var(--green); }
.pill-amber   { background: var(--amber-bg);  color: var(--amber); }
.pill-red     { background: var(--red-bg);    color: var(--red); }
.pill-blue    { background: var(--blue-bg);   color: var(--blue); }
.pill-muted   { background: rgba(255,255,255,0.06); color: var(--muted); }

/* Item title */
.item-title { font-weight: 500; font-size: 13.5px; margin: 6px 0 4px; line-height: 1.4; }
.item-meta  { font-size: 12px; color: var(--muted); display: flex; gap: 12px; flex-wrap: wrap; }
.item-comment-preview {
  margin-top: 8px; padding: 8px 10px;
  background: rgba(255,255,255,0.04); border-radius: 6px;
  font-size: 12px; color: var(--muted); font-style: italic;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.item-prereq-warning {
  margin-top: 6px; display: inline-flex; align-items: center; gap: 4px;
  font-size: 11px; color: var(--amber);
}
.dismiss-btn {
  position: absolute; top: 10px; right: 10px;
  opacity: 0; transition: opacity 0.15s;
  background: none; border: none; cursor: pointer;
  color: var(--muted); padding: 4px;
}
.feed-item:hover .dismiss-btn { opacity: 1; }

/* Buttons */
.btn {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 6px 12px; border-radius: 6px; border: 1px solid var(--border-solid);
  background: var(--card); color: var(--text);
  font-size: 13px; font-weight: 500; cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
}
.btn:hover { background: #252530; }
.btn-primary { background: var(--accent); border-color: var(--accent); color: #fff; }
.btn-primary:hover { background: #6a58e0; }

/* Toast */
.toast-container {
  position: fixed; top: 16px; left: 50%; transform: translateX(-50%);
  z-index: 1000; display: flex; flex-direction: column; gap: 8px;
  pointer-events: none;
}
.toast {
  background: rgba(28,28,34,0.9);
  backdrop-filter: blur(16px);
  border: 1px solid var(--border-solid);
  border-radius: 8px; padding: 10px 16px;
  font-size: 13px; font-weight: 500;
  pointer-events: auto;
  min-width: 240px; text-align: center;
}
.toast-new-items  { border-color: var(--accent); box-shadow: 0 0 12px rgba(124,106,247,0.3); color: var(--accent); }
.toast-error      { border-color: var(--red);    box-shadow: 0 0 12px rgba(248,113,113,0.3); color: var(--red); }
.toast-success    { border-color: var(--green);  box-shadow: 0 0 12px rgba(52,211,153,0.3);  color: var(--green); }

/* Markdown rendered content */
.prose { line-height: 1.65; }
.prose p { margin-bottom: 12px; }
.prose h1,.prose h2,.prose h3 { margin: 20px 0 10px; font-weight: 600; }
.prose code { font-family: 'JetBrains Mono', monospace; font-size: 12px; }
.prose pre { background: rgba(0,0,0,0.3); border-radius: 6px; padding: 12px; overflow-x: auto; margin-bottom: 12px; }
.prose a { color: var(--accent); }
.prose blockquote { border-left: 3px solid var(--border-solid); padding-left: 12px; color: var(--muted); }

/* Settings */
.settings-section { background: var(--card); border: 1px solid var(--border); border-radius: 10px; padding: 20px; margin-bottom: 16px; }
.settings-section h2 { font-size: 14px; font-weight: 600; margin-bottom: 16px; }
.form-group { margin-bottom: 14px; }
.form-label { display: block; font-size: 12px; color: var(--muted); margin-bottom: 4px; }
.form-input {
  width: 100%; padding: 7px 10px;
  background: var(--surface); border: 1px solid var(--border-solid);
  border-radius: 6px; color: var(--text); font-size: 13px;
}
.form-input:focus { outline: none; border-color: var(--accent); }

/* Utility */
.empty-state { text-align: center; padding: 60px 20px; color: var(--muted); }
.empty-state-icon { font-size: 40px; margin-bottom: 12px; }
.flex { display: flex; } .gap-8 { gap: 8px; } .items-center { align-items: center; }
```

- [ ] **Step 2: Write `cmd/churndesk/static/app.js`**

```js
// cmd/churndesk/static/app.js
// Alpine.js component + GSAP wiring + HTMX HX-Trigger handler

function churndesk() {
  return {
    toasts: [],

    addToast(message, type) {
      const id = Date.now()
      const toast = { id, message, type }
      this.toasts.push(toast)
      if (this.toasts.length > 4) this.toasts.shift()
      // GSAP entrance
      this.$nextTick(() => {
        const el = document.querySelector(`[data-toast="${id}"]`)
        if (el && window.gsap) {
          gsap.from(el, { y: -30, scale: 0.85, opacity: 0, duration: 0.4, ease: 'back.out(1.5)' })
        }
      })
      // Auto-dismiss after 4.4s
      setTimeout(() => { this.toasts = this.toasts.filter(t => t.id !== id) }, 4400)
    },

    init() {
      // Handle HX-Trigger after HTMX swaps
      document.addEventListener('htmx:afterSwap', (evt) => {
        const trigger = evt.detail.xhr?.getResponseHeader('HX-Trigger')
        if (!trigger) return
        try {
          const data = JSON.parse(trigger)
          if (data.newItems) {
            this.addToast(`${data.newItems} new item${data.newItems > 1 ? 's' : ''}`, 'new-items')
            // Animate new feed items
            const newEls = document.querySelectorAll('.feed-item[data-new]')
            if (window.gsap && newEls.length) {
              gsap.from(newEls, {
                height: 0, opacity: 0, overflow: 'hidden',
                duration: 0.4, stagger: 0.05,
                onComplete: () => newEls.forEach(el => el.removeAttribute('data-new'))
              })
            }
          }
          if (data.syncError) {
            this.addToast(data.syncError, 'error')
          }
          if (data.settingsSaved) {
            this.addToast('Settings saved', 'success')
          }
        } catch (e) { /* non-JSON trigger, ignore */ }
      })

      // Initial page load animation
      if (window.gsap) {
        const header = document.querySelector('.header')
        if (header) gsap.from(header, { y: -50, opacity: 0, duration: 0.7, ease: 'power3.out' })

        const items = document.querySelectorAll('.feed-item')
        if (items.length) {
          gsap.from(items, {
            y: 30, scale: 0.97, opacity: 0,
            duration: 0.45, stagger: 0.07, ease: 'power2.out'
          })
        }
      }
    }
  }
}

// Navigation animations
function navigateTo(url) {
  const feed = document.querySelector('#feed')
  if (feed && window.gsap) {
    gsap.to(feed, {
      opacity: 0, x: -30, duration: 0.2,
      onComplete: () => { window.location.href = url }
    })
  } else {
    window.location.href = url
  }
}
```

- [ ] **Step 3: Commit**

```bash
git add cmd/churndesk/static/
git commit -m "feat: add static assets — dark theme CSS with color tokens and Alpine.js/GSAP wiring"
```

---

### Task 18: HTTP Server + Middleware

**Files:**
- Create: `internal/web/server.go`
- Create: `internal/web/middleware.go`
- Create: `internal/web/middleware_test.go`

- [ ] **Step 1: Write the failing middleware test**

```go
// internal/web/middleware_test.go
package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/web"
	"github.com/stretchr/testify/assert"
)

type stubOnboardingStore struct{ complete bool }

func (s *stubOnboardingStore) IsOnboardingComplete(ctx context.Context) (bool, error) {
	return s.complete, nil
}

// All other port.IntegrationStore methods — no-op (only IsOnboardingComplete is used by middleware)
func (s *stubOnboardingStore) CreateIntegration(_ context.Context, _ domain.Integration) (int, error)       { return 0, nil }
func (s *stubOnboardingStore) GetIntegration(_ context.Context, _ int) (*domain.Integration, error)         { return nil, nil }
func (s *stubOnboardingStore) UpdateIntegration(_ context.Context, _ domain.Integration) error              { return nil }
func (s *stubOnboardingStore) DeleteIntegration(_ context.Context, _ int) error                             { return nil }
func (s *stubOnboardingStore) ListIntegrations(_ context.Context) ([]domain.Integration, error)             { return nil, nil }
func (s *stubOnboardingStore) UpdateLastSyncedAt(_ context.Context, _ int, _ time.Time) error              { return nil }
func (s *stubOnboardingStore) CreateSpace(_ context.Context, _ domain.Space) (int, error)                   { return 0, nil }
func (s *stubOnboardingStore) ListSpaces(_ context.Context, _ int) ([]domain.Space, error)                  { return nil, nil }
func (s *stubOnboardingStore) UpdateSpace(_ context.Context, _ domain.Space) error                          { return nil }
func (s *stubOnboardingStore) DeleteSpace(_ context.Context, _ int) error                                   { return nil }
func (s *stubOnboardingStore) CreateTeammate(_ context.Context, _ domain.Teammate) error                    { return nil }
func (s *stubOnboardingStore) ListTeammates(_ context.Context, _ int) ([]domain.Teammate, error)            { return nil, nil }
func (s *stubOnboardingStore) DeleteTeammate(_ context.Context, _ int) error                                { return nil }
func (s *stubOnboardingStore) CreatePrerequisite(_ context.Context, _ domain.ReviewPrerequisite) error      { return nil }
func (s *stubOnboardingStore) ListPrerequisites(_ context.Context, _ int) ([]domain.ReviewPrerequisite, error) { return nil, nil }
func (s *stubOnboardingStore) DeletePrerequisite(_ context.Context, _ int) error                            { return nil }

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestOnboardingGate_RedirectsWhenIncomplete(t *testing.T) {
	store := &stubOnboardingStore{complete: false}
	mw := web.OnboardingGate(store)(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/settings?setup=1", rec.Header().Get("Location"))
}

func TestOnboardingGate_HTMXRedirectWhenIncomplete(t *testing.T) {
	// HTMX requests use HX-Redirect header instead of 302
	store := &stubOnboardingStore{complete: false}
	mw := web.OnboardingGate(store)(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/settings?setup=1", rec.Header().Get("HX-Redirect"))
}

func TestOnboardingGate_PassthroughWhenComplete(t *testing.T) {
	store := &stubOnboardingStore{complete: true}
	mw := web.OnboardingGate(store)(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestOnboardingGate_ExemptsPaths(t *testing.T) {
	store := &stubOnboardingStore{complete: false}
	mw := web.OnboardingGate(store)(okHandler)

	exemptPaths := []string{
		"/settings",
		"/settings?setup=1",
		"/static/style.css",
		"/feed",
		"/items/github:approved:1/dismiss",
		"/sync",
	}
	for _, path := range exemptPaths {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "path %s should be exempt", path)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/web/...
```
Expected: FAIL

- [ ] **Step 3: Implement middleware**

```go
// internal/web/middleware.go
package web

import (
	"context"
	"net/http"
	"strings"
)

// OnboardingChecker is the subset of port.IntegrationStore used by the onboarding gate.
// Kept minimal per ISP — middleware should not depend on the full store interface.
type OnboardingChecker interface {
	IsOnboardingComplete(ctx context.Context) (bool, error)
}

var onboardingExemptPrefixes = []string{
	"/settings",
	"/static/",
	"/feed",
	"/items/",  // POST /items/:id/dismiss and /seen
	"/sync",    // POST /sync
}

// OnboardingGate returns middleware that redirects to /settings?setup=1
// when no enabled integrations with spaces are configured.
// HTMX requests (HX-Request: true) receive HX-Redirect instead of 302.
func OnboardingGate(checker OnboardingChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isExemptPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			done, err := checker.IsOnboardingComplete(r.Context())
			if err != nil || done {
				next.ServeHTTP(w, r)
				return
			}
			// Onboarding incomplete — redirect
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/settings?setup=1")
				w.WriteHeader(http.StatusOK)
				return
			}
			http.Redirect(w, r, "/settings?setup=1", http.StatusFound)
		})
	}
}

func isExemptPath(path string) bool {
	for _, prefix := range onboardingExemptPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Implement server**

> **Embed note:** Go's `//go:embed` does not allow `..` path components, so the embed cannot be declared in `internal/web/server.go` if `static/` is at the repo root. Instead, `NewServer` accepts an `fs.FS` injected from `main.go`, where the embed is declared adjacent to the `static/` directory. See Task 29 (main.go wiring) for the embed declaration.

```go
// internal/web/server.go
package web

import (
	"io/fs"
	"net/http"

	"github.com/churndesk/churndesk/internal/web/handlers"
)

// Server wires all HTTP routes and middleware.
type Server struct {
	mux *http.ServeMux
}

// NewServer constructs the HTTP server. All handler dependencies are injected.
// staticFS is the embedded static asset filesystem, declared in main.go adjacent to static/.
func NewServer(
	staticFS fs.FS,
	feed *handlers.FeedHandler,
	pr *handlers.PRHandler,
	jira *handlers.JiraHandler,
	settings *handlers.SettingsHandler,
	gate func(http.Handler) http.Handler,
) *Server {
	mux := http.NewServeMux()

	// Static assets (embedded, passed from main.go)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Feed
	mux.HandleFunc("GET /", gate(http.HandlerFunc(feed.Page)).ServeHTTP)
	mux.HandleFunc("GET /feed", feed.Fragment)
	mux.HandleFunc("POST /items/{id}/dismiss", feed.Dismiss)
	mux.HandleFunc("POST /items/{id}/seen", feed.Seen)
	mux.HandleFunc("POST /sync", feed.Sync)

	// PR view
	mux.HandleFunc("GET /prs/{owner}/{repo}/{number}", gate(http.HandlerFunc(pr.Page)).ServeHTTP)
	mux.HandleFunc("POST /prs/{owner}/{repo}/{number}/comments", pr.PostComment)
	mux.HandleFunc("POST /prs/{owner}/{repo}/{number}/reviews", pr.SubmitReview)
	mux.HandleFunc("POST /prs/{owner}/{repo}/{number}/reviewers", pr.RequestReviewers)

	// Jira view
	mux.HandleFunc("GET /jira/{key}", gate(http.HandlerFunc(jira.Page)).ServeHTTP)
	mux.HandleFunc("POST /jira/{key}/comments", jira.PostComment)

	// Settings
	mux.HandleFunc("GET /settings", settings.Page)
	mux.HandleFunc("POST /settings/integration", settings.SaveIntegration)
	mux.HandleFunc("POST /settings/spaces", settings.SaveSpaces)
	mux.HandleFunc("POST /settings/teammates", settings.SaveTeammates)
	mux.HandleFunc("POST /settings/prerequisites", settings.SavePrerequisites)
	mux.HandleFunc("POST /settings/weights", settings.SaveWeights)
	mux.HandleFunc("POST /settings/general", settings.SaveGeneral)
	mux.HandleFunc("POST /settings/rescore", settings.Rescore)

	return &Server{mux: mux}
}

// Handler returns the root http.Handler for use with http.ListenAndServe.
func (s *Server) Handler() http.Handler { return s.mux }
```

- [ ] **Step 5: Run middleware tests**

```bash
CGO_ENABLED=1 go test ./internal/web/... -run TestOnboardingGate
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/web/server.go internal/web/middleware.go internal/web/middleware_test.go
git commit -m "feat: add HTTP server with route registration and onboarding gate middleware"
```

---

### Task 19: Feed Handlers

**Files:**
- Create: `internal/web/handlers/feed.go`
- Create: `internal/web/handlers/feed_test.go`

All feed actions (dismiss, seen, sync) live in `FeedHandler`. One handler struct, one file.

- [ ] **Step 1: Write the failing test**

```go
// internal/web/handlers/feed_test.go
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

// stubItemStore for handler tests.
type stubItemStore struct {
	items    []domain.Item
	itemCount int
	dismissed []string
	seen      []string
}

func (s *stubItemStore) ListRanked(_ context.Context, _ int) ([]domain.Item, error) { return s.items, nil }
func (s *stubItemStore) Count(_ context.Context) (int, error)                       { return s.itemCount, nil }
func (s *stubItemStore) Dismiss(_ context.Context, id string) error                 { s.dismissed = append(s.dismissed, id); return nil }
func (s *stubItemStore) MarkSeen(_ context.Context, id string) error                { s.seen = append(s.seen, id); return nil }
func (s *stubItemStore) MarkSeenByPR(_ context.Context, _, _ string, _ int) error   { return nil }
func (s *stubItemStore) MarkSeenByJiraKey(_ context.Context, _ string) error        { return nil }
func (s *stubItemStore) Upsert(_ context.Context, _ []domain.Item) error            { return nil }
func (s *stubItemStore) Delete(_ context.Context, _ string) error                   { return nil }
func (s *stubItemStore) RescoreAll(_ context.Context, _ map[domain.ItemType]int, _ []string, _, _ float64) error {
	return nil
}

// compile-time check: stubItemStore satisfies FeedItemStore
var _ FeedItemStore = (*stubItemStore)(nil)

type stubSyncer struct{ synced bool }

func (s *stubSyncer) SyncAll(_ context.Context) error { s.synced = true; return nil }

func makeItem(id string, t domain.ItemType) domain.Item {
	return domain.Item{
		ID: id, Type: t, Source: "github",
		Title: "test item", URL: "/prs/o/r/1",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func TestFeedFragment_SetsNewItemsHeader(t *testing.T) {
	// DB has 5 items, client says it had 3 → server sets HX-Trigger newItems:2
	store := &stubItemStore{
		items:     []domain.Item{makeItem("a", domain.ItemTypePRReviewNeeded)},
		itemCount: 5,
	}
	h := handlers.NewFeedHandler(store, &stubSyncer{}, 1)

	req := httptest.NewRequest("GET", "/feed?count=3", nil)
	rec := httptest.NewRecorder()
	h.Fragment(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	trigger := rec.Header().Get("HX-Trigger")
	assert.Contains(t, trigger, `"newItems":2`)
}

func TestFeedFragment_NoHeaderWhenCountUnchanged(t *testing.T) {
	store := &stubItemStore{items: []domain.Item{}, itemCount: 3}
	h := handlers.NewFeedHandler(store, &stubSyncer{}, 1)

	req := httptest.NewRequest("GET", "/feed?count=3", nil)
	rec := httptest.NewRecorder()
	h.Fragment(rec, req)

	assert.Empty(t, rec.Header().Get("HX-Trigger"))
}

func TestDismiss_CallsStore(t *testing.T) {
	store := &stubItemStore{}
	h := handlers.NewFeedHandler(store, &stubSyncer{}, 1)

	req := httptest.NewRequest("POST", "/items/github:review_needed:42/dismiss", nil)
	req.SetPathValue("id", "github:review_needed:42")
	rec := httptest.NewRecorder()
	h.Dismiss(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"github:review_needed:42"}, store.dismissed)
}

func TestSync_CallsSyncer(t *testing.T) {
	store := &stubItemStore{}
	syncer := &stubSyncer{}
	h := handlers.NewFeedHandler(store, syncer, 1)

	req := httptest.NewRequest("POST", "/sync", nil)
	rec := httptest.NewRecorder()
	h.Sync(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, syncer.synced)
}

func TestFeedFragment_CountParamIgnoredIfNotInt(t *testing.T) {
	store := &stubItemStore{items: []domain.Item{}, itemCount: 3}
	h := handlers.NewFeedHandler(store, &stubSyncer{}, 1)

	req := httptest.NewRequest("GET", "/feed?count=abc", nil)
	rec := httptest.NewRecorder()
	h.Fragment(rec, req)
	// Should not panic; HX-Trigger should not be set when count is non-numeric
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("HX-Trigger"))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/web/handlers/... -run TestFeed -run TestDismiss -run TestSync
```
Expected: FAIL

- [ ] **Step 3: Implement FeedHandler**

```go
// internal/web/handlers/feed.go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/web/templates"
)

// FeedItemStore is the subset of port.ItemStore used by FeedHandler.
// RescoreAll is intentionally excluded — it is called by the settings handler, not the feed handler.
type FeedItemStore interface {
	ListRanked(ctx context.Context, limit int) ([]domain.Item, error)
	Count(ctx context.Context) (int, error)
	Dismiss(ctx context.Context, id string) error
	MarkSeen(ctx context.Context, id string) error
}

// Syncer triggers an immediate full sync across all integrations.
type Syncer interface {
	SyncAll(ctx context.Context) error
}

// FeedHandler handles the main feed page and all feed-related actions.
type FeedHandler struct {
	items   FeedItemStore
	syncer  Syncer
	columns int // default column count (1, 2, or 3)
}

// NewFeedHandler constructs a FeedHandler with all dependencies injected.
func NewFeedHandler(items FeedItemStore, syncer Syncer, defaultColumns int) *FeedHandler {
	return &FeedHandler{items: items, syncer: syncer, columns: defaultColumns}
}

// Page renders the full feed page (GET /).
func (h *FeedHandler) Page(w http.ResponseWriter, r *http.Request) {
	items, err := h.items.ListRanked(r.Context(), 200)
	if err != nil {
		http.Error(w, "failed to load feed", http.StatusInternalServerError)
		return
	}
	if err := templates.FeedPage(items, h.columns).Render(r.Context(), w); err != nil {
		log.Printf("feed page render error: %v", err)
	}
}

// Fragment renders only the feed items div (GET /feed). Used by HTMX auto-refresh.
// Accepts ?count=N to detect new items — if DB count > N, sets HX-Trigger: {"newItems": delta}.
func (h *FeedHandler) Fragment(w http.ResponseWriter, r *http.Request) {
	items, err := h.items.ListRanked(r.Context(), 200)
	if err != nil {
		http.Error(w, "failed to load feed", http.StatusInternalServerError)
		return
	}

	if countStr := r.URL.Query().Get("count"); countStr != "" {
		if clientCount, err := strconv.Atoi(countStr); err == nil {
			dbCount, _ := h.items.Count(r.Context())
			if dbCount > clientCount {
				delta := dbCount - clientCount
				trigger, _ := json.Marshal(map[string]int{"newItems": delta})
				w.Header().Set("HX-Trigger", string(trigger))
			}
		}
	}

	if err := templates.FeedFragment(items, h.columns).Render(r.Context(), w); err != nil {
		log.Printf("feed fragment render error: %v", err)
	}
}

// Dismiss marks an item dismissed (POST /items/{id}/dismiss).
// On success: returns 200 with empty body — HTMX removes the element via hx-swap="outerHTML".
// On error: returns 200 with HX-Trigger syncError so HTMX still performs the swap gracefully.
func (h *FeedHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.items.Dismiss(r.Context(), id); err != nil {
		log.Printf("dismiss item %s: %v", id, err)
		trigger, _ := json.Marshal(map[string]string{"syncError": "Failed to dismiss item"})
		w.Header().Set("HX-Trigger", string(trigger))
	}
	w.WriteHeader(http.StatusOK)
}

// Seen marks an item seen (POST /items/{id}/seen). Returns 204.
func (h *FeedHandler) Seen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.items.MarkSeen(r.Context(), id); err != nil {
		log.Printf("mark seen %s: %v", id, err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// Sync triggers an immediate full sync (POST /sync). Blocks until complete.
// On error, returns 200 with HX-Trigger: {"syncError": "message"} — HTMX standard error pattern.
func (h *FeedHandler) Sync(w http.ResponseWriter, r *http.Request) {
	if err := h.syncer.SyncAll(r.Context()); err != nil {
		log.Printf("manual sync error: %v", err)
		trigger, _ := json.Marshal(map[string]string{"syncError": fmt.Sprintf("Sync failed: %v", err)})
		w.Header().Set("HX-Trigger", string(trigger))
	}
	w.WriteHeader(http.StatusOK)
}
```

- [ ] **Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/web/handlers/... -run TestFeed -run TestDismiss -run TestSync
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/handlers/feed.go internal/web/handlers/feed_test.go
git commit -m "feat: add FeedHandler — feed page, fragment with new-items detection, dismiss, seen, sync"
```

---

### Task 20: Feed Templates

**Files:**
- Create: `internal/web/templates/layout.templ`
- Create: `internal/web/templates/feed.templ`
- Create: `internal/web/templates/components.templ`

Templ files compile to `.templ.go` via `templ generate`. Write the `.templ` source files — the generated `.go` files are committed too (needed for `go build` without `templ` installed).

- [ ] **Step 1: Write `layout.templ`**

```go
// internal/web/templates/layout.templ
package templates

templ Layout(title string) {
	<!DOCTYPE html>
	<html lang="en">
		<head>
			<meta charset="UTF-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
			<title>{ title } — Churndesk</title>
			<link rel="stylesheet" href="/static/style.css"/>
			<script src="https://unpkg.com/alpinejs@3.x.x/dist/cdn.min.js" defer></script>
			<script src="https://cdnjs.cloudflare.com/ajax/libs/gsap/3.12.5/gsap.min.js"></script>
			<script src="https://unpkg.com/htmx.org@2.0.4" integrity="sha384-HGfztofotfshcF7+8n44JQL2oJmowVChPTg48S+jvZoztPfvwD79OC/LTtG6dMp+" crossorigin="anonymous"></script>
			<script src="/static/app.js" defer></script>
		</head>
		<body x-data="churndesk()" x-init="init()">
			<header class="header">
				<a href="/" class="header-logo">Churndesk</a>
				<div class="header-spacer"></div>
				<div class="header-actions">
					<button
						class="btn"
						hx-post="/sync"
						hx-swap="none"
						title="Sync now">
						↻
					</button>
					<a href="/settings" class="btn">Settings</a>
				</div>
			</header>
			<!-- Toast container -->
			<div class="toast-container" aria-live="polite">
				<template x-for="toast in toasts" :key="toast.id">
					<div
						class="toast"
						:class="{
							'toast-new-items': toast.type === 'new-items',
							'toast-error':     toast.type === 'error',
							'toast-success':   toast.type === 'success'
						}"
						:data-toast="toast.id"
						x-text="toast.message">
					</div>
				</template>
			</div>
			{ children... }
		</body>
	</html>
}
```

- [ ] **Step 2: Write `feed.templ`**

```go
// internal/web/templates/feed.templ
package templates

import "github.com/churndesk/churndesk/internal/domain"

// FeedPage renders the full feed page with layout wrapper.
templ FeedPage(items []domain.Item, columns int) {
	@Layout("Feed") {
		<div
			id="feed"
			hx-get="/feed"
			hx-trigger={ "every " + autoRefreshTrigger() + "s" }
			hx-swap="innerHTML"
			hx-vals={ `js:{"count": document.querySelectorAll('.feed-item').length}` }>
			@feedGrid(items, columns)
		</div>
	}
}

// FeedFragment renders only the inner grid (returned by GET /feed for HTMX swap).
templ FeedFragment(items []domain.Item, columns int) {
	@feedGrid(items, columns)
}

templ feedGrid(items []domain.Item, columns int) {
	if len(items) == 0 {
		<div class="empty-state">
			<div class="empty-state-icon">✓</div>
			<p>All clear — no items need your attention.</p>
		</div>
	} else {
		<div class="feed-grid" data-cols={ intStr(columns) }>
			for _, item := range items {
				@FeedItem(item)
			}
		</div>
	}
}

// FeedItem renders a single feed card. Called by feed grid and returned on new-item HTMX events.
templ FeedItem(item domain.Item) {
	<div
		class={ "feed-item", itemTypeClass(item.Type), unseenClass(item.Seen) }
		onclick={ navigateToItem(item) }
		hx-post={ "/items/" + item.ID + "/seen" }
		hx-trigger="click"
		hx-swap="none">
		<div class="flex gap-8 items-center" style="flex-wrap:wrap">
			@TypePill(item.Type)
			@SourcePill(item.Source, item.PROwner, item.PRRepo)
		</div>
		<div class="item-title">{ item.Title }</div>
		if item.PrerequisitesMet == 0 {
			<div class="item-prereq-warning">⏳ Awaiting required approvals</div>
		}
		<div class="item-meta">
			<span>{ relativeTime(item.CreatedAt) }</span>
		</div>
		if latestComment := extractLatestComment(item.Metadata); latestComment != "" {
			<div class="item-comment-preview">{ latestComment }</div>
		}
		<button
			class="dismiss-btn"
			hx-post={ "/items/" + item.ID + "/dismiss" }
			hx-target={ "closest .feed-item" }
			hx-swap="outerHTML"
			onclick="event.stopPropagation()"
			title="Dismiss">
			⊗
		</button>
	</div>
}
```

- [ ] **Step 3: Write `components.templ`**

> **⚠️ DEFERRED:** `autoRefreshTrigger()` returns the hardcoded value `"20"`. This **must** be wired to the `auto_refresh_interval` setting before the app ships. The wiring is completed in Task 28 (Settings handler) — `FeedPage` will need to accept the interval as a parameter passed from the handler, which reads it from `port.SettingsStore`. Do not mark this task complete without creating a follow-up note in Task 28 to complete this wiring.

```go
// internal/web/templates/components.templ
package templates

import (
	"fmt"
	"html/template"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
)

// TypePill renders the item type badge.
templ TypePill(t domain.ItemType) {
	<span class={ "pill", typePillClass(t) }>{ typeLabel(t) }</span>
}

// SourcePill renders the source (github repo or jira space).
templ SourcePill(source, prOwner, prRepo string) {
	if source == "github" && prRepo != "" {
		<span class="pill pill-muted">{ prOwner }/{ prRepo }</span>
	} else if source == "jira" {
		<span class="pill pill-muted">Jira</span>
	}
}

// CommentPartial renders a single comment for HTMX append response.
templ CommentPartial(author, body string, renderedBody template.HTML, createdAt time.Time) {
	<div class="comment" style="padding:12px 0;border-top:1px solid var(--border)">
		<div class="flex gap-8 items-center" style="margin-bottom:6px">
			<span style="font-weight:600;font-size:13px">{ author }</span>
			<span style="color:var(--muted);font-size:12px">{ createdAt.Format("Jan 2, 2006 15:04") }</span>
		</div>
		<div class="prose">@templ.Raw(string(renderedBody))</div>
	</div>
}

// ErrorPartial renders an inline error for failed HTMX operations.
templ ErrorPartial(message string) {
	<div class="pill pill-red" style="display:block;padding:8px 12px;border-radius:6px">
		{ message }
	</div>
}

// ── Template helpers (plain Go functions, not templ components) ─────────────

func itemTypeClass(t domain.ItemType) string {
	switch t {
	case domain.ItemTypePRReviewNeeded:
		return "pr-review-needed"
	case domain.ItemTypePRStaleReview:
		return "pr-stale-review"
	case domain.ItemTypePRChangesRequested:
		return "pr-changes"
	case domain.ItemTypePRNewComment:
		return "pr-comment"
	case domain.ItemTypePRCIFailing:
		return "pr-ci-failing"
	case domain.ItemTypePRApproved:
		return "pr-approved"
	case domain.ItemTypeJiraStatusChange:
		return "jira-status"
	case domain.ItemTypeJiraComment:
		return "jira-comment"
	case domain.ItemTypeJiraNewBug:
		return "jira-bug"
	default:
		return ""
	}
}

func typePillClass(t domain.ItemType) string {
	switch t {
	case domain.ItemTypePRReviewNeeded:
		return "pill-accent"
	case domain.ItemTypePRStaleReview, domain.ItemTypePRChangesRequested,
		domain.ItemTypeJiraStatusChange:
		return "pill-amber"
	case domain.ItemTypePRCIFailing, domain.ItemTypeJiraNewBug:
		return "pill-red"
	case domain.ItemTypePRApproved:
		return "pill-green"
	case domain.ItemTypePRNewComment, domain.ItemTypeJiraComment:
		return "pill-blue"
	default:
		return "pill-muted"
	}
}

func typeLabel(t domain.ItemType) string {
	switch t {
	case domain.ItemTypePRReviewNeeded:
		return "Review needed"
	case domain.ItemTypePRStaleReview:
		return "Stale review"
	case domain.ItemTypePRChangesRequested:
		return "Changes requested"
	case domain.ItemTypePRNewComment:
		return "New comment"
	case domain.ItemTypePRCIFailing:
		return "CI failing"
	case domain.ItemTypePRApproved:
		return "Approved"
	case domain.ItemTypeJiraStatusChange:
		return "Status update"
	case domain.ItemTypeJiraComment:
		return "New comment"
	case domain.ItemTypeJiraNewBug:
		return "New bug"
	default:
		return string(t)
	}
}

func unseenClass(seen int) string {
	if seen == 0 {
		return "unseen"
	}
	return ""
}

func intStr(n int) string { return fmt.Sprintf("%d", n) }

func autoRefreshTrigger() string { return "20" } // TODO: load from settings in FeedPage

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// navigateToItem returns a JS expression for onclick navigation.
// item.URL is validated to contain only safe characters before interpolation.
// Internal paths (starting with /) use navigateTo for GSAP animation; external URLs open in place.
func navigateToItem(item domain.Item) string {
	url := sanitizeURL(item.URL)
	return fmt.Sprintf("navigateTo('%s')", url)
}

// sanitizeURL strips characters that could break a JS single-quoted string.
// Only alphanumeric, /, -, _, ., :, #, and ? are allowed.
func sanitizeURL(url string) string {
	var out []byte
	for i := 0; i < len(url); i++ {
		c := url[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '/' || c == '-' || c == '_' || c == '.' || c == ':' || c == '#' || c == '?' || c == '=' {
			out = append(out, c)
		}
	}
	return string(out)
}

func extractLatestComment(metadata string) string {
	// Fast path: look for "latest_comment" key in metadata JSON string.
	// Full JSON parsing is done in the adapter layer; here we just extract for display.
	const key = `"latest_comment":"`
	start := -1
	for i := range metadata {
		if i+len(key) <= len(metadata) && metadata[i:i+len(key)] == key {
			start = i + len(key)
			break
		}
	}
	if start < 0 || start >= len(metadata) {
		return ""
	}
	end := -1
	for i := start; i < len(metadata); i++ {
		if metadata[i] == '"' && (i == start || metadata[i-1] != '\\') {
			end = i
			break
		}
	}
	if end < 0 {
		return ""
	}
	comment := metadata[start:end]
	if len(comment) > 120 {
		return comment[:120] + "…"
	}
	return comment
}
```

- [ ] **Step 4: Compile templates and verify they build**

```bash
templ generate ./internal/web/templates/...
CGO_ENABLED=1 go build ./...
```
Expected: no errors. If `templ` is not installed: `go install github.com/a-h/templ/cmd/templ@latest` first.

- [ ] **Step 5: Commit**

```bash
git add internal/web/templates/
git commit -m "feat: add feed templates — layout, feed page/fragment, item cards, pills, comment partials"
```

---

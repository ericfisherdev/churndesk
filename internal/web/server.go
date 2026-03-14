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

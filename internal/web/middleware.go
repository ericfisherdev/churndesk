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

var onboardingExemptExact = []string{
	"/feed",
	"/sync",
	"/settings",
}

var onboardingExemptPrefixes = []string{
	"/static/",
	"/items/",
	"/settings/", // settings sub-paths like POST /settings/integration
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
			if err != nil {
				http.Error(w, "service unavailable", http.StatusInternalServerError)
				return
			}
			if done {
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
	for _, exact := range onboardingExemptExact {
		if path == exact {
			return true
		}
	}
	for _, prefix := range onboardingExemptPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

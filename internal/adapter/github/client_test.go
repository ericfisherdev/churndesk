// internal/adapter/github/client_test.go
package github_test

import (
	"testing"

	"github.com/churndesk/churndesk/internal/adapter/github"
	"github.com/churndesk/churndesk/internal/domain/port"
	gogithub "github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

// TestNewClient_ImplementsInterface verifies the adapter satisfies the port at compile time.
func TestNewClient_ImplementsInterface(t *testing.T) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test"})
	tc := oauth2.NewClient(t.Context(), ts)
	gc := gogithub.NewClient(tc)

	var _ port.GitHubClient = github.NewClient(gc, "testuser")
}

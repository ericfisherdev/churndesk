// internal/adapter/jira/client_test.go
package jira_test

import (
	"testing"

	jiradapter "github.com/churndesk/churndesk/internal/adapter/jira"
	"github.com/churndesk/churndesk/internal/domain/port"
)

// TestNewClient_ImplementsInterface verifies the adapter satisfies the port at compile time.
func TestNewClient_ImplementsInterface(t *testing.T) {
	var _ port.JiraClient = jiradapter.NewClient("https://example.atlassian.net", "user@example.com", "token", "account-id")
}

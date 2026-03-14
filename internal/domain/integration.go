// internal/domain/integration.go
package domain

import "time"

type Provider string

const (
	ProviderGitHub Provider = "github"
	ProviderJira   Provider = "jira"
)

type Integration struct {
	ID                  int
	Provider            Provider
	AccessToken         string
	BaseURL             string
	Username            string
	AccountID           string
	PollIntervalSeconds int
	LastSyncedAt        *time.Time
	Enabled             bool
}

type Space struct {
	ID            int
	IntegrationID int
	Provider      Provider
	Owner         string
	Name          string
	BoardType     string
	JiraBoardID   int
	Enabled       bool
}

type Teammate struct {
	ID             int
	IntegrationID  int
	GitHubUsername string
	DisplayName    string
}

type ReviewPrerequisite struct {
	ID             int
	IntegrationID  int
	GitHubUsername string
	DisplayName    string
}

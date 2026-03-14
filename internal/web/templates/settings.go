// internal/web/templates/settings.go
// Stub types for SettingsHandler — replaced by real templ in Task 26.
package templates

import (
	"context"
	"io"

	"github.com/churndesk/churndesk/internal/domain"
)

// IntegrationWithSpaces bundles an integration with its child data.
type IntegrationWithSpaces struct {
	domain.Integration
	Spaces        []domain.Space
	Teammates     []domain.Teammate
	Prerequisites []domain.ReviewPrerequisite
}

// SettingsPageData carries all data needed by the settings page.
type SettingsPageData struct {
	Integrations []IntegrationWithSpaces
	Settings     map[domain.SettingKey]string
	Weights      []domain.CategoryWeight
}

type settingsComponent struct{ data SettingsPageData }

func (c settingsComponent) Render(_ context.Context, w io.Writer) error {
	io.WriteString(w, "Settings") //nolint:errcheck
	return nil
}

// SettingsPage returns a component that renders the settings page.
func SettingsPage(data SettingsPageData) interface{ Render(context.Context, io.Writer) error } {
	return settingsComponent{data: data}
}

## Chunk 6: Settings, main.go Wiring, and Deploy

This final chunk wires all dependencies together in `main.go`, implements the settings UI (handler + template), and adds the Docker and CI configuration. After this chunk the app builds, runs in Docker, and is ready for release.

---

### Task 25: Settings Handler

**Files:**
- Create: `internal/web/handlers/settings.go`
- Create: `internal/web/handlers/settings_test.go`

The settings handler is the most complex handler: it owns all CRUD for integrations, spaces, teammates, prerequisites, category weights, and general settings. It also owns `POST /settings/rescore` which calls `RescoreAll`.

- [ ] **Step 1: Write the failing test**

```go
// internal/web/handlers/settings_test.go
package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/web/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSettingsIntegrationStore records mutations for settings tests.
type stubSettingsIntegrationStore struct {
	integrations []domain.Integration
	spaces       []domain.Space
	teammates    []domain.Teammate
	prerequisites []domain.ReviewPrerequisite
	created      []domain.Integration
}

func (s *stubSettingsIntegrationStore) CreateIntegration(_ context.Context, i domain.Integration) (int, error) {
	s.created = append(s.created, i)
	return 1, nil
}
func (s *stubSettingsIntegrationStore) GetIntegration(_ context.Context, id int) (*domain.Integration, error) {
	for _, i := range s.integrations {
		if i.ID == id {
			return &i, nil
		}
	}
	return nil, nil
}
func (s *stubSettingsIntegrationStore) UpdateIntegration(_ context.Context, _ domain.Integration) error { return nil }
func (s *stubSettingsIntegrationStore) DeleteIntegration(_ context.Context, _ int) error               { return nil }
func (s *stubSettingsIntegrationStore) ListIntegrations(_ context.Context) ([]domain.Integration, error) {
	return s.integrations, nil
}
func (s *stubSettingsIntegrationStore) UpdateLastSyncedAt(_ context.Context, _ int, _ time.Time) error { return nil }
func (s *stubSettingsIntegrationStore) CreateSpace(_ context.Context, _ domain.Space) (int, error)       { return 0, nil }
func (s *stubSettingsIntegrationStore) ListSpaces(_ context.Context, _ int) ([]domain.Space, error)      { return s.spaces, nil }
func (s *stubSettingsIntegrationStore) UpdateSpace(_ context.Context, _ domain.Space) error              { return nil }
func (s *stubSettingsIntegrationStore) DeleteSpace(_ context.Context, _ int) error                       { return nil }
func (s *stubSettingsIntegrationStore) CreateTeammate(_ context.Context, _ domain.Teammate) error        { return nil }
func (s *stubSettingsIntegrationStore) ListTeammates(_ context.Context, _ int) ([]domain.Teammate, error) {
	return s.teammates, nil
}
func (s *stubSettingsIntegrationStore) DeleteTeammate(_ context.Context, _ int) error                              { return nil }
func (s *stubSettingsIntegrationStore) CreatePrerequisite(_ context.Context, _ domain.ReviewPrerequisite) error    { return nil }
func (s *stubSettingsIntegrationStore) ListPrerequisites(_ context.Context, _ int) ([]domain.ReviewPrerequisite, error) {
	return s.prerequisites, nil
}
func (s *stubSettingsIntegrationStore) DeletePrerequisite(_ context.Context, _ int) error { return nil }
func (s *stubSettingsIntegrationStore) IsOnboardingComplete(_ context.Context) (bool, error) {
	return len(s.integrations) > 0, nil
}

// stubSettingsStore for settings handler tests.
type stubSettingsHandlerStore struct {
	settings map[domain.SettingKey]string
	weights  []domain.CategoryWeight
	rescored bool
}

func (s *stubSettingsHandlerStore) Get(_ context.Context, key domain.SettingKey) (string, error) {
	return s.settings[key], nil
}
func (s *stubSettingsHandlerStore) Set(_ context.Context, key domain.SettingKey, val string) error {
	if s.settings == nil {
		s.settings = make(map[domain.SettingKey]string)
	}
	s.settings[key] = val
	return nil
}
func (s *stubSettingsHandlerStore) GetAll(_ context.Context) (map[domain.SettingKey]string, error) {
	return s.settings, nil
}
func (s *stubSettingsHandlerStore) GetCategoryWeights(_ context.Context) ([]domain.CategoryWeight, error) {
	return s.weights, nil
}
func (s *stubSettingsHandlerStore) SetCategoryWeight(_ context.Context, _ domain.ItemType, _ int) error {
	return nil
}

// stubRescoreStore for Rescore endpoint.
type stubRescoreStore struct {
	rescored bool
}

func (s *stubRescoreStore) RescoreAll(_ context.Context, _ map[domain.ItemType]int, _ []string, _, _ float64) error {
	s.rescored = true
	return nil
}

func TestSettingsHandler_Page_Renders(t *testing.T) {
	integrations := &stubSettingsIntegrationStore{}
	settings := &stubSettingsHandlerStore{
		settings: map[domain.SettingKey]string{
			domain.SettingAutoRefreshInterval: "20",
			domain.SettingAgeMultiplier:       "0.5",
			domain.SettingMaxAgeBoost:         "50",
			domain.SettingFeedColumns:         "1",
			domain.SettingMinReviewCount:      "1",
		},
	}
	rescore := &stubRescoreStore{}

	h := handlers.NewSettingsHandler(integrations, settings, rescore, nil)
	req := httptest.NewRequest("GET", "/settings", nil)
	rec := httptest.NewRecorder()
	h.Page(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Settings")
}

func TestSettingsHandler_SaveGeneral_ClampsColumns(t *testing.T) {
	// feed_columns must be clamped to [1,3] — spec §4.8
	integrations := &stubSettingsIntegrationStore{}
	settings := &stubSettingsHandlerStore{settings: make(map[domain.SettingKey]string)}
	rescore := &stubRescoreStore{}

	h := handlers.NewSettingsHandler(integrations, settings, rescore, nil)

	form := url.Values{
		"feed_columns":            {"5"},   // out of range — must be clamped to 3
		"auto_refresh_interval":   {"30"},
		"age_multiplier":          {"0.5"},
		"max_age_boost":           {"50"},
		"min_review_count":        {"2"},
	}
	req := httptest.NewRequest("POST", "/settings/general", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.SaveGeneral(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "3", settings.settings[domain.SettingFeedColumns])
}

func TestSettingsHandler_Rescore_CallsStore(t *testing.T) {
	integrations := &stubSettingsIntegrationStore{}
	settings := &stubSettingsHandlerStore{
		settings: map[domain.SettingKey]string{
			domain.SettingAgeMultiplier: "0.5",
			domain.SettingMaxAgeBoost:   "50",
		},
	}
	rescore := &stubRescoreStore{}

	h := handlers.NewSettingsHandler(integrations, settings, rescore, nil)
	req := httptest.NewRequest("POST", "/settings/rescore", nil)
	rec := httptest.NewRecorder()
	h.Rescore(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, rescore.rescored)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/web/handlers/... -run TestSettingsHandler
```
Expected: FAIL

- [ ] **Step 3: Implement `settings.go`**

```go
// internal/web/handlers/settings.go
package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/web/templates"
)

// SettingsIntegrationStore is the full port.IntegrationStore interface — settings needs all operations.
type SettingsIntegrationStore interface {
	CreateIntegration(ctx context.Context, i domain.Integration) (int, error)
	GetIntegration(ctx context.Context, id int) (*domain.Integration, error)
	UpdateIntegration(ctx context.Context, i domain.Integration) error
	DeleteIntegration(ctx context.Context, id int) error
	ListIntegrations(ctx context.Context) ([]domain.Integration, error)
	CreateSpace(ctx context.Context, sp domain.Space) (int, error)
	ListSpaces(ctx context.Context, integrationID int) ([]domain.Space, error)
	UpdateSpace(ctx context.Context, sp domain.Space) error
	DeleteSpace(ctx context.Context, id int) error
	CreateTeammate(ctx context.Context, t domain.Teammate) error
	ListTeammates(ctx context.Context, integrationID int) ([]domain.Teammate, error)
	DeleteTeammate(ctx context.Context, id int) error
	CreatePrerequisite(ctx context.Context, p domain.ReviewPrerequisite) error
	ListPrerequisites(ctx context.Context, integrationID int) ([]domain.ReviewPrerequisite, error)
	DeletePrerequisite(ctx context.Context, id int) error
}

// SettingsStore is the full port.SettingsStore interface.
type SettingsStore interface {
	GetAll(ctx context.Context) (map[domain.SettingKey]string, error)
	Set(ctx context.Context, key domain.SettingKey, value string) error
	GetCategoryWeights(ctx context.Context) ([]domain.CategoryWeight, error)
	SetCategoryWeight(ctx context.Context, itemType domain.ItemType, weight int) error
}

// RescoreStore is the subset of port.ItemStore used for the rescore endpoint.
type RescoreStore interface {
	RescoreAll(ctx context.Context, weights map[domain.ItemType]int, prerequisiteUsernames []string, ageMultiplier, maxAgeBoost float64) error
}

// SchedulerReloader is implemented by the Scheduler — called after integration changes
// so new workers are started or existing ones stopped without restarting the process.
type SchedulerReloader interface {
	Reload(ctx context.Context) error
}

// SettingsHandler handles GET /settings and all POST /settings/* endpoints.
type SettingsHandler struct {
	integrations SettingsIntegrationStore
	settings     SettingsStore
	rescore      RescoreStore
	scheduler    SchedulerReloader // may be nil in tests
}

// NewSettingsHandler constructs a SettingsHandler.
func NewSettingsHandler(integrations SettingsIntegrationStore, settings SettingsStore, rescore RescoreStore, scheduler SchedulerReloader) *SettingsHandler {
	return &SettingsHandler{integrations: integrations, settings: settings, rescore: rescore, scheduler: scheduler}
}

// Page renders the settings page (GET /settings).
func (h *SettingsHandler) Page(w http.ResponseWriter, r *http.Request) {
	data, err := h.buildPageData(r.Context())
	if err != nil {
		log.Printf("settings page: %v", err)
		templates.ErrorPage("Failed to load settings").Render(r.Context(), w)
		return
	}
	templates.SettingsPage(data).Render(r.Context(), w)
}

// SaveIntegration creates or updates a GitHub or Jira integration (POST /settings/integration).
func (h *SettingsHandler) SaveIntegration(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	provider := domain.Provider(r.FormValue("provider"))
	pollInterval, _ := strconv.Atoi(r.FormValue("poll_interval_seconds"))
	if pollInterval <= 0 {
		pollInterval = 300
	}

	integration := domain.Integration{
		Provider:            provider,
		AccessToken:         r.FormValue("access_token"),
		BaseURL:             r.FormValue("base_url"),
		Username:            r.FormValue("username"),
		PollIntervalSeconds: pollInterval,
		Enabled:             true,
	}

	if idStr := r.FormValue("id"); idStr != "" {
		id, _ := strconv.Atoi(idStr)
		integration.ID = id
		if err := h.integrations.UpdateIntegration(r.Context(), integration); err != nil {
			log.Printf("update integration: %v", err)
			h.respondError(w, r, "Failed to update integration")
			return
		}
	} else {
		if _, err := h.integrations.CreateIntegration(r.Context(), integration); err != nil {
			log.Printf("create integration: %v", err)
			h.respondError(w, r, "Failed to create integration")
			return
		}
	}

	if h.scheduler != nil {
		if err := h.scheduler.Reload(r.Context()); err != nil {
			log.Printf("scheduler reload: %v", err)
		}
	}
	h.respondSuccess(w, r)
}

// SaveSpaces replaces all spaces for an integration (POST /settings/spaces).
// Form fields: integration_id (int), owner[] (repeated), name[] (repeated), enabled[] (repeated checkboxes).
func (h *SettingsHandler) SaveSpaces(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	integrationID, _ := strconv.Atoi(r.FormValue("integration_id"))

	// Delete existing spaces and re-create — simple replace strategy
	spaces, _ := h.integrations.ListSpaces(r.Context(), integrationID)
	for _, sp := range spaces {
		if err := h.integrations.DeleteSpace(r.Context(), sp.ID); err != nil {
			log.Printf("delete space %d: %v", sp.ID, err)
		}
	}

	owners := r.Form["owner"]
	names := r.Form["name"]
	for i := range owners {
		if i >= len(names) {
			break
		}
		sp := domain.Space{
			IntegrationID: integrationID,
			Owner:         owners[i],
			Name:          names[i],
			Enabled:       true,
		}
		h.integrations.CreateSpace(r.Context(), sp)
	}
	h.respondSuccess(w, r)
}

// SaveTeammates replaces all teammates for an integration (POST /settings/teammates).
func (h *SettingsHandler) SaveTeammates(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	integrationID, _ := strconv.Atoi(r.FormValue("integration_id"))

	existing, _ := h.integrations.ListTeammates(r.Context(), integrationID)
	for _, t := range existing {
		if err := h.integrations.DeleteTeammate(r.Context(), t.ID); err != nil {
			log.Printf("delete teammate %d: %v", t.ID, err)
		}
	}
	usernames := r.Form["github_username"]
	displayNames := r.Form["display_name"]
	for i, u := range usernames {
		if u == "" {
			continue
		}
		dn := u
		if i < len(displayNames) {
			dn = displayNames[i]
		}
		h.integrations.CreateTeammate(r.Context(), domain.Teammate{
			IntegrationID:  integrationID,
			GitHubUsername: u,
			DisplayName:    dn,
		})
	}
	h.respondSuccess(w, r)
}

// SavePrerequisites replaces all review prerequisites for an integration (POST /settings/prerequisites).
func (h *SettingsHandler) SavePrerequisites(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	integrationID, _ := strconv.Atoi(r.FormValue("integration_id"))

	existing, _ := h.integrations.ListPrerequisites(r.Context(), integrationID)
	for _, p := range existing {
		if err := h.integrations.DeletePrerequisite(r.Context(), p.ID); err != nil {
			log.Printf("delete prerequisite %d: %v", p.ID, err)
		}
	}
	usernames := r.Form["github_username"]
	displayNames := r.Form["display_name"]
	for i, u := range usernames {
		if u == "" {
			continue
		}
		dn := u
		if i < len(displayNames) {
			dn = displayNames[i]
		}
		h.integrations.CreatePrerequisite(r.Context(), domain.ReviewPrerequisite{
			IntegrationID:  integrationID,
			GitHubUsername: u,
			DisplayName:    dn,
		})
	}
	h.respondSuccess(w, r)
}

// SaveWeights updates category weights and triggers a rescore (POST /settings/weights).
// Form fields: weight_{itemType} (repeated per type, value 1–100).
func (h *SettingsHandler) SaveWeights(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	allTypes := []domain.ItemType{
		domain.ItemTypePRCIFailing, domain.ItemTypePRChangesRequested,
		domain.ItemTypePRStaleReview, domain.ItemTypeJiraNewBug,
		domain.ItemTypePRReviewNeeded, domain.ItemTypePRNewComment,
		domain.ItemTypeJiraStatusChange, domain.ItemTypeJiraComment,
		domain.ItemTypePRApproved,
	}
	for _, t := range allTypes {
		key := "weight_" + string(t)
		if val := r.FormValue(key); val != "" {
			w8, err := strconv.Atoi(val)
			if err != nil || w8 < 1 {
				w8 = 1
			}
			if w8 > 100 {
				w8 = 100
			}
			h.settings.SetCategoryWeight(r.Context(), t, w8)
		}
	}
	h.respondSuccess(w, r)
}

// SaveGeneral saves general settings (POST /settings/general).
// Clamps feed_columns to [1,3] before both persisting and rendering — spec §4.8.
func (h *SettingsHandler) SaveGeneral(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	cols, _ := strconv.Atoi(r.FormValue("feed_columns"))
	if cols < 1 {
		cols = 1
	}
	if cols > 3 {
		cols = 3
	}

	settings := map[domain.SettingKey]string{
		domain.SettingFeedColumns:          strconv.Itoa(cols),
		domain.SettingAutoRefreshInterval:  r.FormValue("auto_refresh_interval"),
		domain.SettingAgeMultiplier:        r.FormValue("age_multiplier"),
		domain.SettingMaxAgeBoost:          r.FormValue("max_age_boost"),
		domain.SettingMinReviewCount:       r.FormValue("min_review_count"),
	}
	for k, v := range settings {
		if v == "" {
			continue
		}
		if err := h.settings.Set(r.Context(), k, v); err != nil {
			log.Printf("set setting %s: %v", k, err)
		}
	}
	h.respondSuccess(w, r)
}

// Rescore triggers an immediate RescoreAll with current weights and settings (POST /settings/rescore).
func (h *SettingsHandler) Rescore(w http.ResponseWriter, r *http.Request) {
	all, err := h.settings.GetAll(r.Context())
	if err != nil {
		h.respondError(w, r, "Failed to load settings")
		return
	}
	ageMultiplier, _ := strconv.ParseFloat(all[domain.SettingAgeMultiplier], 64)
	maxAgeBoost, _ := strconv.ParseFloat(all[domain.SettingMaxAgeBoost], 64)

	cw, _ := h.settings.GetCategoryWeights(r.Context())
	weights := make(map[domain.ItemType]int, len(cw))
	for _, w8 := range cw {
		weights[w8.ItemType] = w8.Weight
	}

	// Collect prerequisite usernames (used for prerequisites_met column)
	integrations, _ := h.integrations.ListIntegrations(r.Context())
	prereqSet := map[string]struct{}{}
	for _, ig := range integrations {
		prereqs, _ := h.integrations.ListPrerequisites(r.Context(), ig.ID)
		for _, p := range prereqs {
			prereqSet[p.GitHubUsername] = struct{}{}
		}
	}
	prereqUsernames := make([]string, 0, len(prereqSet))
	for u := range prereqSet {
		prereqUsernames = append(prereqUsernames, u)
	}

	if err := h.rescore.RescoreAll(r.Context(), weights, prereqUsernames, ageMultiplier, maxAgeBoost); err != nil {
		log.Printf("rescore: %v", err)
		h.respondError(w, r, "Rescore failed")
		return
	}
	h.respondSuccess(w, r)
}

// respondSuccess returns 200 with HX-Trigger settingsSaved (Alpine.js shows a toast).
func (h *SettingsHandler) respondSuccess(w http.ResponseWriter, r *http.Request) {
	trigger, _ := json.Marshal(map[string]bool{"settingsSaved": true})
	w.Header().Set("HX-Trigger", string(trigger))
	w.WriteHeader(http.StatusOK)
}

// respondError returns 200 with HX-Trigger syncError.
func (h *SettingsHandler) respondError(w http.ResponseWriter, r *http.Request, msg string) {
	trigger, _ := json.Marshal(map[string]string{"syncError": msg})
	w.Header().Set("HX-Trigger", string(trigger))
	w.WriteHeader(http.StatusOK)
}

func (h *SettingsHandler) buildPageData(ctx context.Context) (templates.SettingsPageData, error) {
	integrations, err := h.integrations.ListIntegrations(ctx)
	if err != nil {
		return templates.SettingsPageData{}, err
	}

	settings, err := h.settings.GetAll(ctx)
	if err != nil {
		return templates.SettingsPageData{}, err
	}

	weights, err := h.settings.GetCategoryWeights(ctx)
	if err != nil {
		return templates.SettingsPageData{}, err
	}

	var ig templates.IntegrationWithSpaces
	var allTeammates []domain.Teammate
	var allPrereqs []domain.ReviewPrerequisite
	igs := make([]templates.IntegrationWithSpaces, 0, len(integrations))
	for _, integration := range integrations {
		spaces, _ := h.integrations.ListSpaces(ctx, integration.ID)
		teammates, _ := h.integrations.ListTeammates(ctx, integration.ID)
		prereqs, _ := h.integrations.ListPrerequisites(ctx, integration.ID)
		ig = templates.IntegrationWithSpaces{
			Integration:   integration,
			Spaces:        spaces,
			Teammates:     teammates,
			Prerequisites: prereqs,
		}
		igs = append(igs, ig)
		allTeammates = append(allTeammates, teammates...)
		allPrereqs = append(allPrereqs, prereqs...)
	}
	_ = allTeammates
	_ = allPrereqs

	return templates.SettingsPageData{
		Integrations: igs,
		Settings:     settings,
		Weights:      weights,
	}, nil
}
```

- [ ] **Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/web/handlers/... -run TestSettingsHandler
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/handlers/settings.go internal/web/handlers/settings_test.go
git commit -m "feat: add settings handler — CRUD for integrations, spaces, teammates, prerequisites, weights, rescore"
```

> **Compile-time interface check:** Add `var _ handlers.SettingsIntegrationStore = (*stubSettingsIntegrationStore)(nil)` to `settings_test.go` so the test file fails with a clear error if any method signature drifts.

---

### Task 26: Settings Template

**Files:**
- Create: `internal/web/templates/settings.templ`

- [ ] **Step 1: Write `settings.templ`**

```go
// internal/web/templates/settings.templ
package templates

import (
	"fmt"
	"strconv"

	"github.com/churndesk/churndesk/internal/domain"
)

// IntegrationWithSpaces bundles an integration with its child data for the settings page.
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

templ SettingsPage(d SettingsPageData) {
	@Layout("Settings") {
		<div style="max-width:800px;margin:0 auto;padding:20px 24px">
			<h1 style="font-size:20px;font-weight:700;margin-bottom:20px">Settings</h1>
			<!-- GitHub Integration -->
			@integrationSection(d, domain.ProviderGitHub)
			<!-- Jira Integration -->
			@integrationSection(d, domain.ProviderJira)
			<!-- General settings -->
			@generalSection(d)
			<!-- Category weights -->
			@weightsSection(d)
		</div>
	}
}

templ integrationSection(d SettingsPageData, provider domain.Provider) {
	<div class="settings-section">
		<h2>{ providerLabel(provider) } Integration</h2>
		for _, ig := range d.Integrations {
			if ig.Provider == provider {
				<form
					hx-post="/settings/integration"
					hx-swap="none"
					style="margin-bottom:20px">
					<input type="hidden" name="id" value={ strconv.Itoa(ig.ID) }/>
					<input type="hidden" name="provider" value={ string(provider) }/>
					if provider == domain.ProviderGitHub {
						<div class="form-group">
							<label class="form-label">Personal Access Token</label>
							<input class="form-input" type="password" name="access_token" value={ ig.AccessToken }/>
						</div>
						<div class="form-group">
							<label class="form-label">GitHub Username</label>
							<input class="form-input" type="text" name="username" value={ ig.Username }/>
						</div>
					} else {
						<div class="form-group">
							<label class="form-label">Jira Base URL</label>
							<input class="form-input" type="url" name="base_url" value={ ig.BaseURL }/>
						</div>
						<div class="form-group">
							<label class="form-label">API Token</label>
							<input class="form-input" type="password" name="access_token" value={ ig.AccessToken }/>
						</div>
						<div class="form-group">
							<label class="form-label">Account ID</label>
							<input class="form-input" type="text" name="username" value={ ig.Username }/>
						</div>
					}
					<div class="form-group">
						<label class="form-label">Poll interval (seconds)</label>
						<input class="form-input" type="number" name="poll_interval_seconds" value={ strconv.Itoa(ig.PollIntervalSeconds) } min="60"/>
					</div>
					<button type="submit" class="btn btn-primary">Save</button>
				</form>
				<!-- Spaces / Repos -->
				@spacesSection(ig)
				<!-- Teammates (GitHub only) -->
				if provider == domain.ProviderGitHub {
					@teammatesSection(ig)
					@prerequisitesSection(ig)
				}
			}
		}
		<!-- Add new integration (shown when none exists for this provider) -->
		if !hasProvider(d.Integrations, provider) {
			<form hx-post="/settings/integration" hx-swap="none">
				<input type="hidden" name="provider" value={ string(provider) }/>
				if provider == domain.ProviderGitHub {
					<div class="form-group">
						<label class="form-label">Personal Access Token</label>
						<input class="form-input" type="password" name="access_token" placeholder="ghp_…"/>
					</div>
					<div class="form-group">
						<label class="form-label">GitHub Username</label>
						<input class="form-input" type="text" name="username"/>
					</div>
				} else {
					<div class="form-group">
						<label class="form-label">Jira Base URL</label>
						<input class="form-input" type="url" name="base_url" placeholder="https://myorg.atlassian.net"/>
					</div>
					<div class="form-group">
						<label class="form-label">API Token</label>
						<input class="form-input" type="password" name="access_token"/>
					</div>
					<div class="form-group">
						<label class="form-label">Account ID</label>
						<input class="form-input" type="text" name="username"/>
					</div>
				}
				<button type="submit" class="btn btn-primary">Connect { providerLabel(provider) }</button>
			</form>
		}
	</div>
}

templ spacesSection(ig IntegrationWithSpaces) {
	<div style="margin-top:16px">
		<h3 style="font-size:13px;font-weight:600;margin-bottom:10px">
			if ig.Provider == domain.ProviderGitHub {
				Tracked repositories
			} else {
				Tracked Jira spaces
			}
		</h3>
		<form hx-post="/settings/spaces" hx-swap="none">
			<input type="hidden" name="integration_id" value={ strconv.Itoa(ig.ID) }/>
			<div id={ fmt.Sprintf("spaces-%d", ig.ID) }>
				for _, sp := range ig.Spaces {
					<div style="display:flex;gap:8px;align-items:center;margin-bottom:6px">
						<input class="form-input" type="text" name="owner" value={ sp.Owner } style="flex:1"/>
						<input class="form-input" type="text" name="name" value={ sp.Name } style="flex:1"/>
					</div>
				}
				<!-- Empty row for adding new -->
				<div style="display:flex;gap:8px;align-items:center;margin-bottom:6px">
					if ig.Provider == domain.ProviderGitHub {
						<input class="form-input" type="text" name="owner" placeholder="org or user"/>
						<input class="form-input" type="text" name="name" placeholder="repo name"/>
					} else {
						<input class="form-input" type="text" name="owner" placeholder="project key e.g. FRONT"/>
						<input class="form-input" type="text" name="name" placeholder="space name"/>
					}
				</div>
			</div>
			<button type="submit" class="btn" style="margin-top:6px">Save</button>
		</form>
		if ig.Provider == domain.ProviderGitHub {
			<p style="font-size:11px;color:var(--muted);margin-top:6px">
				Note: Stale review detection requires "Dismiss stale reviews on new commits" branch protection to be enabled.
			</p>
		}
	</div>
}

templ teammatesSection(ig IntegrationWithSpaces) {
	<div style="margin-top:16px">
		<h3 style="font-size:13px;font-weight:600;margin-bottom:10px">Teammates to watch</h3>
		<form hx-post="/settings/teammates" hx-swap="none">
			<input type="hidden" name="integration_id" value={ strconv.Itoa(ig.ID) }/>
			for _, t := range ig.Teammates {
				<div style="display:flex;gap:8px;margin-bottom:6px">
					<input class="form-input" type="text" name="github_username" value={ t.GitHubUsername } placeholder="github_username"/>
					<input class="form-input" type="text" name="display_name" value={ t.DisplayName } placeholder="display name"/>
				</div>
			}
			<div style="display:flex;gap:8px;margin-bottom:6px">
				<input class="form-input" type="text" name="github_username" placeholder="github_username"/>
				<input class="form-input" type="text" name="display_name" placeholder="display name"/>
			</div>
			<button type="submit" class="btn">Save</button>
		</form>
	</div>
}

templ prerequisitesSection(ig IntegrationWithSpaces) {
	<div style="margin-top:16px">
		<h3 style="font-size:13px;font-weight:600;margin-bottom:10px">Review prerequisites</h3>
		<p style="font-size:12px;color:var(--muted);margin-bottom:10px">
			PRs must be approved by all listed bots before gaining age points. Leave empty to disable.
		</p>
		<form hx-post="/settings/prerequisites" hx-swap="none">
			<input type="hidden" name="integration_id" value={ strconv.Itoa(ig.ID) }/>
			for _, p := range ig.Prerequisites {
				<div style="display:flex;gap:8px;margin-bottom:6px">
					<input class="form-input" type="text" name="github_username" value={ p.GitHubUsername } placeholder="copilot[bot]"/>
					<input class="form-input" type="text" name="display_name" value={ p.DisplayName } placeholder="Copilot"/>
				</div>
			}
			<div style="display:flex;gap:8px;margin-bottom:6px">
				<input class="form-input" type="text" name="github_username" placeholder="copilot[bot]"/>
				<input class="form-input" type="text" name="display_name" placeholder="display name"/>
			</div>
			<button type="submit" class="btn">Save</button>
		</form>
	</div>
}

templ generalSection(d SettingsPageData) {
	<div class="settings-section">
		<h2>General</h2>
		<form hx-post="/settings/general" hx-swap="none">
			<div class="form-group">
				<label class="form-label">Feed columns (1–3)</label>
				<input class="form-input" type="number" name="feed_columns" min="1" max="3"
					value={ d.Settings[domain.SettingFeedColumns] }/>
			</div>
			<div class="form-group">
				<label class="form-label">Auto-refresh interval (seconds)</label>
				<input class="form-input" type="number" name="auto_refresh_interval" min="5"
					value={ d.Settings[domain.SettingAutoRefreshInterval] }/>
			</div>
			<div class="form-group">
				<label class="form-label">Age multiplier (points per hour)</label>
				<input class="form-input" type="number" name="age_multiplier" min="0" step="0.1"
					value={ d.Settings[domain.SettingAgeMultiplier] }/>
			</div>
			<div class="form-group">
				<label class="form-label">Max age boost</label>
				<input class="form-input" type="number" name="max_age_boost" min="0"
					value={ d.Settings[domain.SettingMaxAgeBoost] }/>
			</div>
			<div class="form-group">
				<label class="form-label">Minimum review count for approval</label>
				<input class="form-input" type="number" name="min_review_count" min="1"
					value={ d.Settings[domain.SettingMinReviewCount] }/>
			</div>
			<button type="submit" class="btn btn-primary">Save general settings</button>
		</form>
	</div>
}

templ weightsSection(d SettingsPageData) {
	<div class="settings-section">
		<h2>Priority weights</h2>
		<p style="font-size:12px;color:var(--muted);margin-bottom:16px">
			Applies to newly created items. Use "Re-score all" to apply retroactively.
		</p>
		<form hx-post="/settings/weights" hx-swap="none">
			for _, w := range d.Weights {
				<div class="form-group" style="display:flex;align-items:center;gap:12px">
					<label class="form-label" style="width:180px;margin:0">{ typeLabel(w.ItemType) }</label>
					<input type="range" min="1" max="100" name={ "weight_" + string(w.ItemType) }
						value={ strconv.Itoa(w.Weight) } style="flex:1"/>
					<span style="width:30px;text-align:right;font-size:12px">{ strconv.Itoa(w.Weight) }</span>
				</div>
			}
			<div style="display:flex;gap:8px;margin-top:16px">
				<button type="submit" class="btn btn-primary">Save weights</button>
				<button type="button" class="btn"
					hx-post="/settings/rescore"
					hx-swap="none">
					Re-score all items
				</button>
			</div>
		</form>
	</div>
}

func providerLabel(p domain.Provider) string {
	if p == domain.ProviderGitHub {
		return "GitHub"
	}
	return "Jira"
}

func hasProvider(igs []IntegrationWithSpaces, provider domain.Provider) bool {
	for _, ig := range igs {
		if ig.Provider == provider {
			return true
		}
	}
	return false
}

var _ = fmt.Sprintf
var _ = strconv.Itoa
```

> **FeedHandler wiring:** The `autoRefreshTrigger()` deferred from chunk-4 is resolved in Task 27 Step 0, which patches `FeedHandler` to accept a `FeedSettingsStore` and read interval/columns per-request. Complete that step before building.

- [ ] **Step 2: Compile templates**

```bash
templ generate ./internal/web/templates/...
CGO_ENABLED=1 go build ./...
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/web/templates/settings.templ
git commit -m "feat: add settings page template — integrations, spaces, teammates, prerequisites, weights, general"
```

---

### Task 27: main.go — Composition Root

**Files:**
- Create: `cmd/churndesk/main.go`

`main.go` is the composition root: it wires every dependency together and starts the HTTP server, scheduler, and scorer. Nothing else does this wiring.

- [ ] **Step 0: Patch FeedHandler to read interval and columns from settings (resolves ⚠️ DEFERRED from chunk-4 Task 20)**

The chunk-4 `autoRefreshTrigger()` was hardcoded to `"20"`. Now that `SettingsStore` exists, wire the real values. Make these changes to the files created in chunk-4:

**`internal/web/handlers/feed.go` — add `FeedSettingsStore` interface and update `FeedHandler`:**

```go
// FeedSettingsStore is the subset of port.SettingsStore used by FeedHandler.
type FeedSettingsStore interface {
	Get(ctx context.Context, key domain.SettingKey) (string, error)
}
```

Replace the `FeedHandler` struct and constructor:

```go
// FeedHandler handles the main feed page and all feed-related actions.
type FeedHandler struct {
	items    FeedItemStore
	syncer   Syncer
	settings FeedSettingsStore
}

// NewFeedHandler constructs a FeedHandler with all dependencies injected.
func NewFeedHandler(items FeedItemStore, syncer Syncer, settings FeedSettingsStore) *FeedHandler {
	return &FeedHandler{items: items, syncer: syncer, settings: settings}
}
```

Add helper methods to read settings per-request (defaults used when setting absent or invalid):

```go
func (h *FeedHandler) readColumnsSetting(ctx context.Context) int {
	v, _ := h.settings.Get(ctx, domain.SettingFeedColumns)
	n, _ := strconv.Atoi(v)
	if n < 1 || n > 3 {
		return 1
	}
	return n
}

func (h *FeedHandler) readIntervalSetting(ctx context.Context) int {
	v, _ := h.settings.Get(ctx, domain.SettingAutoRefreshInterval)
	n, _ := strconv.Atoi(v)
	if n < 10 {
		return 20
	}
	return n
}
```

Update `Page` and `Fragment` to pass live values:

```go
func (h *FeedHandler) Page(w http.ResponseWriter, r *http.Request) {
	items, err := h.items.ListRanked(r.Context(), 200)
	if err != nil {
		log.Printf("feed page: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	columns := h.readColumnsSetting(r.Context())
	interval := h.readIntervalSetting(r.Context())
	if err := templates.FeedPage(items, columns, interval).Render(r.Context(), w); err != nil {
		log.Printf("feed page render: %v", err)
	}
}

func (h *FeedHandler) Fragment(w http.ResponseWriter, r *http.Request) {
	// ... existing count logic unchanged ...
	columns := h.readColumnsSetting(r.Context())
	interval := h.readIntervalSetting(r.Context())
	if err := templates.FeedFragment(items, columns, interval).Render(r.Context(), w); err != nil {
		log.Printf("feed fragment render: %v", err)
	}
}
```

Add `"strconv"` to the imports in `feed.go` if not already present.

**`internal/web/templates/feed.templ` (or `components.templ`) — update `FeedPage` and `FeedFragment` to accept `interval int`:**

```go
// FeedPage renders the full feed page with layout wrapper.
templ FeedPage(items []domain.Item, columns, interval int) {
	@Layout("Churndesk") {
		<div
			id="feed-container"
			hx-get="/feed"
			hx-trigger={ "every " + strconv.Itoa(interval) + "s" }
			hx-swap="innerHTML">
			@feedGrid(items, columns)
		</div>
	}
}

// FeedFragment renders only the inner grid (returned by GET /feed for HTMX swap).
templ FeedFragment(items []domain.Item, columns, interval int) {
	@feedGrid(items, columns)
}
```

Remove `autoRefreshTrigger()` function from `components.templ`. Add `"strconv"` to imports in the templ file.

**`internal/web/handlers/feed_test.go` — add stub and update handler construction:**

```go
// stubFeedSettingsStore returns configurable setting values.
type stubFeedSettingsStore struct {
	values map[domain.SettingKey]string
}

func (s *stubFeedSettingsStore) Get(_ context.Context, key domain.SettingKey) (string, error) {
	return s.values[key], nil
}

var _ handlers.FeedSettingsStore = (*stubFeedSettingsStore)(nil)
```

Replace all `handlers.NewFeedHandler(store, &stubSyncer{}, 1)` calls with:

```go
handlers.NewFeedHandler(store, &stubSyncer{}, &stubFeedSettingsStore{
	values: map[domain.SettingKey]string{
		domain.SettingFeedColumns:         "1",
		domain.SettingAutoRefreshInterval: "20",
	},
})
```

```bash
CGO_ENABLED=1 go test ./internal/web/handlers/... -run TestFeed
```
Expected: PASS

- [ ] **Step 1: Write `main.go`**

```go
// cmd/churndesk/main.go
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	gogithub "github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"

	"github.com/churndesk/churndesk/internal/adapter/github"
	jiradapter "github.com/churndesk/churndesk/internal/adapter/jira"
	"github.com/churndesk/churndesk/internal/adapter/sqlite"
	"github.com/churndesk/churndesk/internal/app"
	"github.com/churndesk/churndesk/internal/config"
	"github.com/churndesk/churndesk/internal/db"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
	"github.com/churndesk/churndesk/internal/web"
	"github.com/churndesk/churndesk/internal/web/handlers"
)

// static/ lives at cmd/churndesk/static/ so the embed path is adjacent (no ".." needed).
//go:embed static
var staticFiles embed.FS

func main() {
	cfg := config.Load()

	// ── Database ──────────────────────────────────────────────────────────────
	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	// ── Stores (SQLite adapters) ─────────────────────────────────────────────
	itemStore := sqlite.NewItemStore(conn)
	linkStore := sqlite.NewLinkStore(conn)
	integrationStore := sqlite.NewIntegrationStore(conn)
	settingsStore := sqlite.NewSettingsStore(conn)

	// ── GitHub client ─────────────────────────────────────────────────────────
	// Token and username come from the first GitHub integration in the DB.
	// If no integration is configured yet, GitHub features are unavailable until setup.
	ghToken, ghUsername := loadGitHubCredentials(integrationStore)
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: ghToken})
	tc := oauth2.NewClient(context.Background(), ts)
	ghClient := github.NewClient(gogithub.NewClient(tc), ghUsername)

	// ── Jira client ───────────────────────────────────────────────────────────
	jiraBaseURL, jiraEmail, jiraToken, jiraAccountID := loadJiraCredentials(integrationStore)
	jiraClient := jiradapter.NewClient(jiraBaseURL, jiraEmail, jiraToken, jiraAccountID)

	// ── GitHub fetcher ────────────────────────────────────────────────────────
	minReviewCount := loadMinReviewCount(settingsStore)
	teammates := loadTeammates(integrationStore)
	ghFetcher := github.NewFetcher(ghClient, ghUsername, teammates, minReviewCount)

	// ── Jira fetcher ──────────────────────────────────────────────────────────
	jiraFetcher := jiradapter.NewFetcher(jiraClient, jiraAccountID)

	// ── Application services ──────────────────────────────────────────────────
	fetchers := map[domain.Provider]port.Fetcher{
		domain.ProviderGitHub: ghFetcher,
		domain.ProviderJira:   jiraFetcher,
	}
	scheduler := app.NewScheduler(itemStore, integrationStore, fetchers)
	scorer := app.NewScorer(itemStore, settingsStore, integrationStore)

	// ── Web handlers ──────────────────────────────────────────────────────────
	feedHandler := handlers.NewFeedHandler(itemStore, scheduler, settingsStore)
	prHandler := handlers.NewPRHandler(ghClient, itemStore, linkStore, integrationStore, ghUsername)
	jiraHandler := handlers.NewJiraHandler(jiraClient, itemStore, linkStore)
	settingsHandler := handlers.NewSettingsHandler(integrationStore, settingsStore, itemStore, scheduler)

	// ── Onboarding gate middleware ────────────────────────────────────────────
	gate := web.OnboardingGate(integrationStore)

	// ── Static assets (embed declared here, adjacent to static/) ─────────────
	staticFS, _ := fs.Sub(staticFiles, "static")

	// ── HTTP server ───────────────────────────────────────────────────────────
	srv := web.NewServer(staticFS, feedHandler, prHandler, jiraHandler, settingsHandler, gate)
	httpServer := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: srv.Handler(),
	}

	// ── Start background services ─────────────────────────────────────────────
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	scorer.Start(ctx)
	go func() {
		if err := scheduler.Start(ctx); err != nil {
			log.Printf("scheduler stopped: %v", err)
		}
	}()

	// ── Start HTTP server ─────────────────────────────────────────────────────
	go func() {
		log.Printf("churndesk listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down…")
	httpServer.Shutdown(context.Background())
}

// ── Helpers for loading credentials from DB at startup ───────────────────────

func loadGitHubCredentials(store port.IntegrationStore) (token, username string) {
	integrations, err := store.ListIntegrations(context.Background())
	if err != nil {
		return "", ""
	}
	for _, ig := range integrations {
		if ig.Provider == domain.ProviderGitHub && ig.Enabled {
			return ig.AccessToken, ig.Username
		}
	}
	return "", ""
}

func loadJiraCredentials(store port.IntegrationStore) (baseURL, email, token, accountID string) {
	integrations, err := store.ListIntegrations(context.Background())
	if err != nil {
		return
	}
	for _, ig := range integrations {
		if ig.Provider == domain.ProviderJira && ig.Enabled {
			// domain.Integration has a single Username field used for both the Jira
			// account email (HTTP Basic auth) and the account ID (issue filtering).
			// The settings UI labels this field "Account ID"; the value is the Atlassian
			// account ID which also doubles as the username for go-atlassian's auth.
			return ig.BaseURL, ig.Username, ig.AccessToken, ig.Username
		}
	}
	return
}

func loadMinReviewCount(store port.SettingsStore) int {
	val, _ := store.Get(context.Background(), domain.SettingMinReviewCount)
	n := 1
	if val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			n = parsed
		}
	}
	return n
}

func loadTeammates(store port.IntegrationStore) []domain.Teammate {
	integrations, _ := store.ListIntegrations(context.Background())
	var out []domain.Teammate
	for _, ig := range integrations {
		ts, _ := store.ListTeammates(context.Background(), ig.ID)
		out = append(out, ts...)
	}
	return out
}
```

> **Static directory location:** The embed uses `//go:embed static` with `static/` located at `cmd/churndesk/static/` (adjacent to `main.go`). Go's `//go:embed` prohibits `..` path components, so the directory cannot live at the repo root. Task 17 (chunk-4) creates the static assets at `cmd/churndesk/static/style.css` and `cmd/churndesk/static/app.js`. `00-overview.md` file structure shows `cmd/churndesk/static/`.

- [ ] **Step 2: Build to verify wiring compiles**

```bash
templ generate ./internal/web/templates/...
CGO_ENABLED=1 go build ./cmd/churndesk/...
```
Expected: no errors (may require stub implementations for incomplete adapters)

- [ ] **Step 3: Commit**

```bash
git add cmd/churndesk/
git commit -m "feat: add main.go composition root — wires all adapters, app services, and HTTP server"
```

---

### Task 28: Dockerfile + docker-compose

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`

No unit tests for deployment config. Verified by running `docker build`.

- [ ] **Step 1: Write `Dockerfile`**

```dockerfile
# Dockerfile

# Stage 1: build
FROM golang:1.23-bookworm AS builder

WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc libc-dev libsqlite3-dev && rm -rf /var/lib/apt/lists/*

ENV CGO_ENABLED=1

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Compile templ templates before building
RUN go install github.com/a-h/templ/cmd/templ@latest && templ generate ./internal/web/templates/...
RUN go build -o /churndesk ./cmd/churndesk

# Stage 2: runtime
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libsqlite3-0 ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /churndesk /churndesk

EXPOSE 8080
ENTRYPOINT ["/churndesk"]
```

- [ ] **Step 2: Write `docker-compose.yml`**

```yaml
# docker-compose.yml
services:
  churndesk:
    image: churndesk/churndesk:latest
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    environment:
      - CHURNDESK_DB_PATH=/data/churndesk.db
      - CHURNDESK_PORT=8080
    restart: unless-stopped
```

- [ ] **Step 3: Test Docker build**

```bash
docker build -t churndesk:dev .
```
Expected: build succeeds, image ~80-120MB

- [ ] **Step 4: Smoke test**

```bash
mkdir -p data
docker run --rm -p 8080:8080 -v $(pwd)/data:/data churndesk:dev
```
Expected: app starts, `localhost:8080` redirects to `/settings?setup=1` (onboarding gate)

- [ ] **Step 5: Create `data/.gitkeep` and commit**

```bash
mkdir -p data && touch data/.gitkeep
git add Dockerfile docker-compose.yml data/.gitkeep
git commit -m "feat: add multi-stage Dockerfile and docker-compose for local deployment"
```

---

### Task 29: GitHub Actions CI

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Write `.github/workflows/ci.yml`**

```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Install templ
        run: go install github.com/a-h/templ/cmd/templ@latest

      - name: Generate templates
        run: templ generate ./internal/web/templates/...

      - name: Test
        run: CGO_ENABLED=1 go test ./...

      - name: Build
        run: CGO_ENABLED=1 go build ./cmd/churndesk/...
```

- [ ] **Step 2: Write `.github/workflows/release.yml`**

```yaml
# .github/workflows/release.yml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Log in to Docker Hub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKERHUB_USERNAME }}
          password: ${{ secrets.DOCKERHUB_TOKEN }}

      - name: Extract version
        id: meta
        run: echo "version=${GITHUB_REF#refs/tags/}" >> $GITHUB_OUTPUT

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: |
            churndesk/churndesk:latest
            churndesk/churndesk:${{ steps.meta.outputs.version }}
```

- [ ] **Step 3: Commit**

```bash
mkdir -p .github/workflows
git add .github/workflows/
git commit -m "feat: add GitHub Actions CI (test+build on push/PR) and release workflow (Docker push on tag)"
```

---

### Task 30: Final Integration Verification

- [ ] **Step 1: Full test suite**

```bash
templ generate ./internal/web/templates/...
CGO_ENABLED=1 go test ./...
```
Expected: all PASS

- [ ] **Step 2: Full build**

```bash
CGO_ENABLED=1 go build ./cmd/churndesk/...
```
Expected: binary produced, no errors

- [ ] **Step 3: Docker build**

```bash
docker build -t churndesk:final .
```
Expected: success

- [ ] **Step 4: Smoke test onboarding flow**

```bash
docker run --rm -p 8080:8080 -v /tmp/churndesk-test:/data churndesk:final
```
Open `http://localhost:8080` — expected: redirect to `/settings?setup=1`

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "chore: final integration — all chunks complete, build and tests green"
```

---

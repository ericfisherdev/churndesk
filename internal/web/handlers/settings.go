package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

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
	UpdateLastSyncedAt(ctx context.Context, id int, t time.Time) error
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
	IsOnboardingComplete(ctx context.Context) (bool, error)
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

// SchedulerReloader is implemented by the Scheduler.
type SchedulerReloader interface {
	Reload(ctx context.Context) error
}

// SettingsHandler handles GET /settings and all POST /settings/* endpoints.
type SettingsHandler struct {
	integrations SettingsIntegrationStore
	settings     SettingsStore
	rescore      RescoreStore
	scheduler    SchedulerReloader
}

// NewSettingsHandler constructs a SettingsHandler.
func NewSettingsHandler(integrations SettingsIntegrationStore, settings SettingsStore, rescore RescoreStore, scheduler SchedulerReloader) *SettingsHandler {
	return &SettingsHandler{integrations: integrations, settings: settings, rescore: rescore, scheduler: scheduler}
}

// Page renders the settings page.
func (h *SettingsHandler) Page(w http.ResponseWriter, r *http.Request) {
	data, err := h.buildPageData(r.Context())
	if err != nil {
		log.Printf("settings page: %v", err)
		templates.ErrorPage("Failed to load settings").Render(r.Context(), w) //nolint:errcheck
		return
	}
	templates.SettingsPage(data).Render(r.Context(), w) //nolint:errcheck
}

// SaveIntegration creates or updates an integration.
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
		AccountID:           r.FormValue("account_id"),
		PollIntervalSeconds: pollInterval,
		Enabled:             true,
	}

	if idStr := r.FormValue("id"); idStr != "" {
		id, _ := strconv.Atoi(idStr)
		integration.ID = id
		if err := h.integrations.UpdateIntegration(r.Context(), integration); err != nil {
			log.Printf("update integration: %v", err)
			h.respondError(w, "Failed to update integration")
			return
		}
	} else {
		if _, err := h.integrations.CreateIntegration(r.Context(), integration); err != nil {
			log.Printf("create integration: %v", err)
			h.respondError(w, "Failed to create integration")
			return
		}
	}

	if h.scheduler != nil {
		if err := h.scheduler.Reload(r.Context()); err != nil {
			log.Printf("scheduler reload: %v", err)
		}
	}
	h.respondSuccess(w)
}

// SaveSpaces replaces all spaces for an integration.
func (h *SettingsHandler) SaveSpaces(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	integrationID, _ := strconv.Atoi(r.FormValue("integration_id"))

	spaces, err := h.integrations.ListSpaces(r.Context(), integrationID)
	if err != nil {
		log.Printf("list spaces for integration %d: %v", integrationID, err)
		h.respondError(w, "Failed to load spaces")
		return
	}
	for _, sp := range spaces {
		if err := h.integrations.DeleteSpace(r.Context(), sp.ID); err != nil {
			log.Printf("delete space %d: %v", sp.ID, err)
			h.respondError(w, "Failed to delete space")
			return
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
		if _, err := h.integrations.CreateSpace(r.Context(), sp); err != nil {
			log.Printf("create space: %v", err)
			h.respondError(w, "Failed to create space")
			return
		}
	}
	h.respondSuccess(w)
}

// SaveTeammates replaces all teammates for an integration.
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
		h.integrations.CreateTeammate(r.Context(), domain.Teammate{ //nolint:errcheck
			IntegrationID:  integrationID,
			GitHubUsername: u,
			DisplayName:    dn,
		})
	}
	h.respondSuccess(w)
}

// SavePrerequisites replaces all review prerequisites for an integration.
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
		h.integrations.CreatePrerequisite(r.Context(), domain.ReviewPrerequisite{ //nolint:errcheck
			IntegrationID:  integrationID,
			GitHubUsername: u,
			DisplayName:    dn,
		})
	}
	h.respondSuccess(w)
}

// SaveWeights updates category weights.
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
			h.settings.SetCategoryWeight(r.Context(), t, w8) //nolint:errcheck
		}
	}
	h.respondSuccess(w)
}

// SaveGeneral saves general settings. feed_columns is clamped to [1,3].
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
		domain.SettingFeedColumns:         strconv.Itoa(cols),
		domain.SettingAutoRefreshInterval: r.FormValue("auto_refresh_interval"),
		domain.SettingAgeMultiplier:       r.FormValue("age_multiplier"),
		domain.SettingMaxAgeBoost:         r.FormValue("max_age_boost"),
		domain.SettingMinReviewCount:      r.FormValue("min_review_count"),
	}
	for k, v := range settings {
		if v == "" {
			continue
		}
		if err := h.settings.Set(r.Context(), k, v); err != nil {
			log.Printf("set setting %s: %v", k, err)
		}
	}
	h.respondSuccess(w)
}

// Rescore triggers an immediate RescoreAll.
func (h *SettingsHandler) Rescore(w http.ResponseWriter, r *http.Request) {
	all, err := h.settings.GetAll(r.Context())
	if err != nil {
		h.respondError(w, "Failed to load settings")
		return
	}
	ageMultiplier, _ := strconv.ParseFloat(all[domain.SettingAgeMultiplier], 64)
	maxAgeBoost, _ := strconv.ParseFloat(all[domain.SettingMaxAgeBoost], 64)

	cw, err := h.settings.GetCategoryWeights(r.Context())
	if err != nil {
		log.Printf("get category weights: %v", err)
		h.respondError(w, "Failed to load category weights")
		return
	}
	weights := make(map[domain.ItemType]int, len(cw))
	for _, w8 := range cw {
		weights[w8.ItemType] = w8.Weight
	}

	integrations, err := h.integrations.ListIntegrations(r.Context())
	if err != nil {
		log.Printf("list integrations: %v", err)
		h.respondError(w, "Failed to load integrations")
		return
	}
	prereqSet := map[string]struct{}{}
	for _, ig := range integrations {
		prereqs, err := h.integrations.ListPrerequisites(r.Context(), ig.ID)
		if err != nil {
			log.Printf("list prerequisites for integration %d: %v", ig.ID, err)
			h.respondError(w, "Failed to load prerequisites")
			return
		}
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
		h.respondError(w, "Rescore failed")
		return
	}
	h.respondSuccess(w)
}

func (h *SettingsHandler) respondSuccess(w http.ResponseWriter) {
	trigger, _ := json.Marshal(map[string]bool{"settingsSaved": true})
	w.Header().Set("HX-Trigger", string(trigger))
	w.WriteHeader(http.StatusOK)
}

func (h *SettingsHandler) respondError(w http.ResponseWriter, msg string) {
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

	igs := make([]templates.IntegrationWithSpaces, 0, len(integrations))
	for _, integration := range integrations {
		spaces, _ := h.integrations.ListSpaces(ctx, integration.ID)
		teammates, _ := h.integrations.ListTeammates(ctx, integration.ID)
		prereqs, _ := h.integrations.ListPrerequisites(ctx, integration.ID)
		igs = append(igs, templates.IntegrationWithSpaces{
			Integration:   integration,
			Spaces:        spaces,
			Teammates:     teammates,
			Prerequisites: prereqs,
		})
	}

	return templates.SettingsPageData{
		Integrations: igs,
		Settings:     settings,
		Weights:      weights,
	}, nil
}

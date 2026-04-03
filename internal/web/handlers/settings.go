package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	ReplaceSpaces(ctx context.Context, integrationID int, spaces []domain.Space) error
	CreateTeammate(ctx context.Context, t domain.Teammate) error
	ListTeammates(ctx context.Context, integrationID int) ([]domain.Teammate, error)
	DeleteTeammate(ctx context.Context, id int) error
	ReplaceTeammates(ctx context.Context, integrationID int, teammates []domain.Teammate) error
	CreatePrerequisite(ctx context.Context, p domain.ReviewPrerequisite) error
	ListPrerequisites(ctx context.Context, integrationID int) ([]domain.ReviewPrerequisite, error)
	DeletePrerequisite(ctx context.Context, id int) error
	ReplacePrerequisites(ctx context.Context, integrationID int, prereqs []domain.ReviewPrerequisite) error
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
		w.WriteHeader(http.StatusInternalServerError)
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
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			h.respondError(w, "Invalid integration ID", http.StatusBadRequest)
			return
		}
		integration.ID = id
		// Blank access_token means "keep existing" — fetch and preserve the stored token.
		if integration.AccessToken == "" {
			existing, err := h.integrations.GetIntegration(r.Context(), id)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					h.respondError(w, "Integration not found", http.StatusNotFound)
					return
				}
				log.Printf("get integration for token preservation: %v", err)
				h.respondError(w, "Failed to load integration", http.StatusInternalServerError)
				return
			}
			integration.AccessToken = existing.AccessToken
		}
		if err := h.integrations.UpdateIntegration(r.Context(), integration); err != nil {
			log.Printf("update integration: %v", err)
			h.respondError(w, "Failed to update integration", http.StatusInternalServerError)
			return
		}
	} else {
		if _, err := h.integrations.CreateIntegration(r.Context(), integration); err != nil {
			if errors.Is(err, domain.ErrDuplicateProvider) {
				h.respondError(w, "An integration for this provider already exists", http.StatusConflict)
				return
			}
			log.Printf("create integration: %v", err)
			h.respondError(w, "Failed to create integration", http.StatusInternalServerError)
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

// SaveSpaces atomically replaces all spaces for an integration.
func (h *SettingsHandler) SaveSpaces(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	integrationID, ok := parseIntegrationID(w, r)
	if !ok {
		return
	}

	existing, err := h.integrations.ListSpaces(r.Context(), integrationID)
	if err != nil {
		log.Printf("list spaces for integration %d: %v", integrationID, err)
		h.respondError(w, "Failed to load spaces", http.StatusInternalServerError)
		return
	}
	existingByKey := make(map[string]domain.Space, len(existing))
	for _, sp := range existing {
		existingByKey[sp.Owner+"/"+sp.Name] = sp
	}

	owners := r.Form["owner"]
	names := r.Form["name"]
	limit := len(owners)
	if len(names) < limit {
		limit = len(names)
	}
	spaces := make([]domain.Space, 0, limit)
	for i := 0; i < limit; i++ {
		sp := domain.Space{
			IntegrationID: integrationID,
			Owner:         owners[i],
			Name:          names[i],
			Enabled:       true,
		}
		if prev, ok := existingByKey[owners[i]+"/"+names[i]]; ok {
			sp.Provider = prev.Provider
			sp.BoardType = prev.BoardType
			sp.JiraBoardID = prev.JiraBoardID
		}
		spaces = append(spaces, sp)
	}

	if err := h.integrations.ReplaceSpaces(r.Context(), integrationID, spaces); err != nil {
		log.Printf("replace spaces for integration %d: %v", integrationID, err)
		h.respondError(w, "Failed to save spaces", http.StatusInternalServerError)
		return
	}
	h.respondSuccess(w)
}

// SaveTeammates replaces all teammates for an integration.
func (h *SettingsHandler) SaveTeammates(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	integrationID, ok := parseIntegrationID(w, r)
	if !ok {
		return
	}

	usernames := r.Form["github_username"]
	displayNames := r.Form["display_name"]
	teammates := make([]domain.Teammate, 0, len(usernames))
	for i, u := range usernames {
		if u == "" {
			continue
		}
		dn := u
		if i < len(displayNames) {
			dn = displayNames[i]
		}
		teammates = append(teammates, domain.Teammate{
			IntegrationID:  integrationID,
			GitHubUsername: u,
			DisplayName:    dn,
		})
	}
	if err := h.integrations.ReplaceTeammates(r.Context(), integrationID, teammates); err != nil {
		log.Printf("replace teammates for integration %d: %v", integrationID, err)
		h.respondError(w, "Failed to save teammates", http.StatusInternalServerError)
		return
	}
	h.respondSuccess(w)
}

// SavePrerequisites replaces all review prerequisites for an integration.
func (h *SettingsHandler) SavePrerequisites(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	integrationID, ok := parseIntegrationID(w, r)
	if !ok {
		return
	}

	usernames := r.Form["github_username"]
	displayNames := r.Form["display_name"]
	prereqs := make([]domain.ReviewPrerequisite, 0, len(usernames))
	for i, u := range usernames {
		if u == "" {
			continue
		}
		dn := u
		if i < len(displayNames) {
			dn = displayNames[i]
		}
		prereqs = append(prereqs, domain.ReviewPrerequisite{
			IntegrationID:  integrationID,
			GitHubUsername: u,
			DisplayName:    dn,
		})
	}
	if err := h.integrations.ReplacePrerequisites(r.Context(), integrationID, prereqs); err != nil {
		log.Printf("replace prerequisites for integration %d: %v", integrationID, err)
		h.respondError(w, "Failed to save prerequisites", http.StatusInternalServerError)
		return
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
	var writeErr bool
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
			if err := h.settings.SetCategoryWeight(r.Context(), t, w8); err != nil {
				log.Printf("set category weight %s=%d: %v", t, w8, err)
				writeErr = true
			}
		}
	}
	if writeErr {
		h.respondError(w, "Failed to save some weights", http.StatusInternalServerError)
		return
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
	var writeErr bool
	for k, v := range settings {
		if v == "" {
			continue
		}
		if err := h.settings.Set(r.Context(), k, v); err != nil {
			log.Printf("set setting %s: %v", k, err)
			writeErr = true
		}
	}
	if writeErr {
		h.respondError(w, "Failed to save some settings", http.StatusInternalServerError)
		return
	}
	h.respondSuccess(w)
}

// Rescore triggers an immediate RescoreAll.
func (h *SettingsHandler) Rescore(w http.ResponseWriter, r *http.Request) {
	all, err := h.settings.GetAll(r.Context())
	if err != nil {
		h.respondError(w, "Failed to load settings", http.StatusInternalServerError)
		return
	}
	ageMultiplier, err := strconv.ParseFloat(all[domain.SettingAgeMultiplier], 64)
	if err != nil {
		log.Printf("rescore: missing or invalid age_multiplier setting, age scoring disabled")
	}
	maxAgeBoost, err := strconv.ParseFloat(all[domain.SettingMaxAgeBoost], 64)
	if err != nil {
		log.Printf("rescore: missing or invalid max_age_boost setting, age scoring disabled")
	}

	cw, err := h.settings.GetCategoryWeights(r.Context())
	if err != nil {
		log.Printf("get category weights: %v", err)
		h.respondError(w, "Failed to load category weights", http.StatusInternalServerError)
		return
	}
	weights := make(map[domain.ItemType]int, len(cw))
	for _, w8 := range cw {
		weights[w8.ItemType] = w8.Weight
	}

	integrations, err := h.integrations.ListIntegrations(r.Context())
	if err != nil {
		log.Printf("list integrations: %v", err)
		h.respondError(w, "Failed to load integrations", http.StatusInternalServerError)
		return
	}
	prereqSet := map[string]struct{}{}
	for _, ig := range integrations {
		prereqs, err := h.integrations.ListPrerequisites(r.Context(), ig.ID)
		if err != nil {
			log.Printf("list prerequisites for integration %d: %v", ig.ID, err)
			h.respondError(w, "Failed to load prerequisites", http.StatusInternalServerError)
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
		h.respondError(w, "Rescore failed", http.StatusInternalServerError)
		return
	}
	h.respondSuccess(w)
}

// parseIntegrationID extracts and validates the integration_id form value.
// It writes a 400 response and returns false when the value is missing or non-numeric.
func parseIntegrationID(w http.ResponseWriter, r *http.Request) (int, bool) {
	val := r.FormValue("integration_id")
	if val == "" {
		http.Error(w, "missing integration_id", http.StatusBadRequest)
		return 0, false
	}
	id, err := strconv.Atoi(val)
	if err != nil || id <= 0 {
		http.Error(w, "invalid integration_id: must be positive", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func (h *SettingsHandler) respondSuccess(w http.ResponseWriter) {
	trigger, _ := json.Marshal(map[string]bool{"settingsSaved": true})
	w.Header().Set("HX-Trigger", string(trigger))
	w.WriteHeader(http.StatusOK)
}

func (h *SettingsHandler) respondError(w http.ResponseWriter, msg string, status int) { //nolint:unparam
	trigger, _ := json.Marshal(map[string]string{"syncError": msg})
	w.Header().Set("HX-Trigger", string(trigger))
	w.WriteHeader(status)
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
		spaces, err := h.integrations.ListSpaces(ctx, integration.ID)
		if err != nil {
			return templates.SettingsPageData{}, fmt.Errorf("list spaces for integration %d: %w", integration.ID, err)
		}
		teammates, err := h.integrations.ListTeammates(ctx, integration.ID)
		if err != nil {
			return templates.SettingsPageData{}, fmt.Errorf("list teammates for integration %d: %w", integration.ID, err)
		}
		prereqs, err := h.integrations.ListPrerequisites(ctx, integration.ID)
		if err != nil {
			return templates.SettingsPageData{}, fmt.Errorf("list prerequisites for integration %d: %w", integration.ID, err)
		}
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

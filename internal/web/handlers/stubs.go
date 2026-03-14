// internal/web/handlers/stubs.go
// Stub handler types — replaced by real implementations in subsequent tasks.
// This file exists only to allow server.go to compile before handlers are implemented.
package handlers

import "net/http"

// FeedHandler handles the feed page and actions. Real implementation in feed.go.
type FeedHandler struct{}

func (h *FeedHandler) Page(w http.ResponseWriter, r *http.Request)     {}
func (h *FeedHandler) Fragment(w http.ResponseWriter, r *http.Request) {}
func (h *FeedHandler) Dismiss(w http.ResponseWriter, r *http.Request)  {}
func (h *FeedHandler) Seen(w http.ResponseWriter, r *http.Request)     {}
func (h *FeedHandler) Sync(w http.ResponseWriter, r *http.Request)     {}

// PRHandler handles PR detail page and actions. Real implementation in pr.go.
type PRHandler struct{}

func (h *PRHandler) Page(w http.ResponseWriter, r *http.Request)             {}
func (h *PRHandler) PostComment(w http.ResponseWriter, r *http.Request)      {}
func (h *PRHandler) SubmitReview(w http.ResponseWriter, r *http.Request)     {}
func (h *PRHandler) RequestReviewers(w http.ResponseWriter, r *http.Request) {}

// JiraHandler handles Jira detail page and actions. Real implementation in jira.go.
type JiraHandler struct{}

func (h *JiraHandler) Page(w http.ResponseWriter, r *http.Request)       {}
func (h *JiraHandler) PostComment(w http.ResponseWriter, r *http.Request) {}

// SettingsHandler handles the settings page and save actions. Real implementation in settings.go.
type SettingsHandler struct{}

func (h *SettingsHandler) Page(w http.ResponseWriter, r *http.Request)              {}
func (h *SettingsHandler) SaveIntegration(w http.ResponseWriter, r *http.Request)   {}
func (h *SettingsHandler) SaveSpaces(w http.ResponseWriter, r *http.Request)        {}
func (h *SettingsHandler) SaveTeammates(w http.ResponseWriter, r *http.Request)     {}
func (h *SettingsHandler) SavePrerequisites(w http.ResponseWriter, r *http.Request) {}
func (h *SettingsHandler) SaveWeights(w http.ResponseWriter, r *http.Request)       {}
func (h *SettingsHandler) SaveGeneral(w http.ResponseWriter, r *http.Request)       {}
func (h *SettingsHandler) Rescore(w http.ResponseWriter, r *http.Request)           {}

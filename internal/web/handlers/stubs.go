// internal/web/handlers/stubs.go
// Stub handler types — replaced by real implementations in subsequent tasks.
// This file exists only to allow server.go to compile before handlers are implemented.
package handlers

import "net/http"

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

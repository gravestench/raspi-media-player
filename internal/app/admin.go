package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/auth"
	"github.com/dylanknuth/raspi-media-player/internal/enrichment"
)

func (a *application) requireInstallation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.installed.Load() || r.URL.Path == "/api/v1/setup/status" || r.URL.Path == "/api/v1/setup/complete" || strings.HasPrefix(r.URL.Path, "/api/v1/health/") || r.URL.Path == "/api/v1/version" || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusServiceUnavailable, "setup_required", "complete first-run installation before using the player")
	})
}

func (a *application) setupStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"installed": a.installed.Load(),
		"steps":     []string{"welcome", "household_access", "administrator", "metadata", "review"},
		"links": map[string]string{
			"lastfm_api": "https://www.last.fm/api/account/create",
			"privacy":    "https://www.last.fm/api/tos",
		},
	})
}

func (a *application) completeSetup(w http.ResponseWriter, r *http.Request) {
	if a.installed.Load() {
		writeError(w, http.StatusConflict, "already_installed", "installation is already complete")
		return
	}
	var request struct {
		Username             string `json:"username"`
		Password             string `json:"password"`
		PasswordConfirmation string `json:"password_confirmation"`
		AccessMode           string `json:"access_mode"`
		LastFMAPIKey         string `json:"lastfm_api_key"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.Password != request.PasswordConfirmation {
		writeError(w, http.StatusUnprocessableEntity, "password_confirmation_mismatch", "password confirmation does not match")
		return
	}
	if request.AccessMode == "" {
		request.AccessMode = "open"
	}
	if request.AccessMode != "open" && request.AccessMode != "accounts_optional" && request.AccessMode != "accounts_required" {
		writeError(w, http.StatusUnprocessableEntity, "invalid_access_mode", "choose open, accounts_optional, or accounts_required")
		return
	}
	issued, err := a.auth.CreateInitialAdminAndSession(r.Context(), request.Username, request.Password)
	if err != nil {
		if strings.Contains(err.Error(), "username") || strings.Contains(err.Error(), "password") {
			writeError(w, http.StatusUnprocessableEntity, "invalid_account", err.Error())
			return
		}
		if strings.Contains(err.Error(), "already complete") {
			writeError(w, http.StatusConflict, "already_installed", "installation is already complete")
			return
		}
		a.internalError(w, r, "complete setup", err)
		return
	}
	if err := a.settings.Set(r.Context(), "access_mode", request.AccessMode, issued.Session.User.ID); err != nil && !strings.Contains(err.Error(), "unknown setting") {
		a.logger.Warn("could not save setup access mode", "error", err)
	}
	if request.LastFMAPIKey != "" {
		if err := a.settings.Set(r.Context(), "lastfm_api_key", request.LastFMAPIKey, issued.Session.User.ID); err != nil {
			a.logger.Warn("could not save setup metadata key", "error", err)
		}
	}
	a.installed.Store(true)
	a.logger.Info("first-run installation completed", "user_id", issued.Session.User.ID, "username", issued.Session.User.Username)
	a.issueSession(w, issued, http.StatusCreated)
}

func (a *application) requireAdmin(w http.ResponseWriter, r *http.Request) *Identity {
	identity := identityFromContext(r.Context())
	if identity == nil {
		writeError(w, http.StatusUnauthorized, "authentication_required", "sign in as an administrator")
		return nil
	}
	if !identity.Session.User.IsAdmin {
		writeError(w, http.StatusForbidden, "administrator_required", "administrator access is required")
		return nil
	}
	return identity
}

func (a *application) listAdminSettings(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	values, err := a.settings.List(r.Context())
	if err != nil {
		a.internalError(w, r, "list admin settings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": values,
		"links": map[string]string{
			"lastfm_create_key":  "https://www.last.fm/api/account/create",
			"lastfm_api_terms":   "https://www.last.fm/api/tos",
			"musicbrainz_policy": "https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting",
		},
	})
}

func (a *application) updateAdminSetting(w http.ResponseWriter, r *http.Request) {
	identity := a.requireAdmin(w, r)
	if identity == nil {
		return
	}
	var request struct {
		Value string `json:"value"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateSetting(r.PathValue("key"), request.Value); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_setting", err.Error())
		return
	}
	if err := a.settings.Set(r.Context(), r.PathValue("key"), request.Value, identity.Session.User.ID); err != nil {
		if strings.Contains(err.Error(), "unknown setting") || strings.Contains(err.Error(), "encryption") {
			writeError(w, http.StatusUnprocessableEntity, "invalid_setting", err.Error())
			return
		}
		a.internalError(w, r, "update admin setting", err)
		return
	}
	a.logger.Info("administrator setting updated", "setting", r.PathValue("key"), "user_id", identity.Session.User.ID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved", "restart_required": true})
}

func (a *application) deleteAdminSetting(w http.ResponseWriter, r *http.Request) {
	identity := a.requireAdmin(w, r)
	if identity == nil {
		return
	}
	if err := a.settings.Set(r.Context(), r.PathValue("key"), "", identity.Session.User.ID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_setting", err.Error())
		return
	}
	a.logger.Info("administrator setting removed", "setting", r.PathValue("key"), "user_id", identity.Session.User.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (a *application) listAdminUsers(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	users, err := a.auth.ListUsers(r.Context())
	if err != nil {
		a.internalError(w, r, "list admin users", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (a *application) updateAdminRole(w http.ResponseWriter, r *http.Request) {
	identity := a.requireAdmin(w, r)
	if identity == nil {
		return
	}
	var request struct {
		Admin bool `json:"admin"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := a.auth.SetAdmin(r.Context(), r.PathValue("id"), request.Admin); err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user_not_found", "user not found")
			return
		}
		if strings.Contains(err.Error(), "final administrator") {
			writeError(w, http.StatusConflict, "final_administrator", err.Error())
			return
		}
		a.internalError(w, r, "update administrator role", err)
		return
	}
	a.logger.Info("administrator role updated", "target_user_id", r.PathValue("id"), "admin", request.Admin, "user_id", identity.Session.User.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (a *application) testLastFM(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) == nil {
		return
	}
	var request struct {
		APIKey string `json:"api_key"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	key := strings.TrimSpace(request.APIKey)
	if key == "" {
		var err error
		key, err = a.settings.Value(r.Context(), "lastfm_api_key")
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "lastfm_not_configured", "a Last.fm API key is not configured")
			return
		}
	}
	if key == "" {
		writeError(w, http.StatusUnprocessableEntity, "lastfm_not_configured", "a Last.fm API key is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	_, err := enrichment.NewLastFMProvider(key, nil).Lookup(ctx, enrichment.TrackHint{Artist: "Cher", Title: "Believe"})
	if err != nil {
		writeError(w, http.StatusBadGateway, "lastfm_connection_failed", "Last.fm rejected the key or could not be reached")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

func validateSetting(key, value string) error {
	switch key {
	case "access_mode":
		if value != "open" && value != "accounts_optional" && value != "accounts_required" {
			return errors.New("invalid access mode")
		}
	case "player_backend":
		if value != "mpv" && value != "fake" {
			return errors.New("invalid player backend")
		}
	case "metadata_enabled", "vote_enabled", "youtube_search_enabled", "secure_cookie", "player_enabled":
		if value != "true" && value != "false" {
			return errors.New("value must be true or false")
		}
	case "queue_limit", "queue_rate", "cache_seconds", "player_retries", "history_days", "metadata_cache_days", "metadata_max_inflight", "vote_active_seconds", "vote_timeout_seconds", "vote_percent", "youtube_search_results", "auth_rate", "session_days", "argon_memory", "argon_iterations":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 || parsed > 100000 {
			return errors.New("value must be a reasonable non-negative number")
		}
	}
	return nil
}

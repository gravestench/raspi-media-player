package app

import (
	"fmt"
	"net/http"
	"strings"
)

func (a *application) autoQueueStatus(w http.ResponseWriter, r *http.Request) {
	enabled, err := a.settings.Value(r.Context(), "auto_queue_enabled")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "available": false})
		return
	}
	depth, _ := a.settings.Value(r.Context(), "auto_queue_depth")
	mode, _ := a.settings.Value(r.Context(), "auto_queue_mode")
	artists, _ := a.settings.Value(r.Context(), "auto_queue_artists")
	genres, _ := a.settings.Value(r.Context(), "auto_queue_genres")
	if mode == "" {
		mode = "active_users"
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled == "true", "available": true, "depth": depth, "mode": mode, "artists": artists, "genres": genres})
}

func (a *application) setAutoQueue(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enabled *bool   `json:"enabled"`
		Mode    *string `json:"mode"`
		Artists *string `json:"artists"`
		Genres  *string `json:"genres"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	userID := ""
	if identity := identityFromContext(r.Context()); identity != nil {
		userID = identity.Session.User.ID
	}
	updates := map[string]string{}
	if request.Enabled != nil {
		updates["auto_queue_enabled"] = fmt.Sprint(*request.Enabled)
	}
	if request.Mode != nil {
		updates["auto_queue_mode"] = strings.TrimSpace(*request.Mode)
	}
	if request.Artists != nil {
		updates["auto_queue_artists"] = strings.TrimSpace(*request.Artists)
	}
	if request.Genres != nil {
		updates["auto_queue_genres"] = strings.TrimSpace(*request.Genres)
	}
	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "provide an auto-queue setting to update")
		return
	}
	for key, value := range updates {
		if err := validateSetting(key, value); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "invalid_setting", err.Error())
			return
		}
	}
	for key, value := range updates {
		if err := a.settings.Set(r.Context(), key, value, userID); err != nil {
			a.internalError(w, r, "update auto-queue", err)
			return
		}
	}
	loggerFromContext(r.Context(), a.logger).Info("auto-queue settings updated", "keys", len(updates))
	a.autoQueueStatus(w, r)
}

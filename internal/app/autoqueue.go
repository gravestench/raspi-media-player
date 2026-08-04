package app

import (
	"net/http"
)

func (a *application) autoQueueStatus(w http.ResponseWriter, r *http.Request) {
	enabled, err := a.settings.Value(r.Context(), "auto_queue_enabled")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "available": false})
		return
	}
	depth, _ := a.settings.Value(r.Context(), "auto_queue_depth")
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled == "true", "available": true, "depth": depth})
}

func (a *application) setAutoQueue(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	userID := ""
	if identity := identityFromContext(r.Context()); identity != nil {
		userID = identity.Session.User.ID
	}
	value := "false"
	if request.Enabled {
		value = "true"
	}
	if err := a.settings.Set(r.Context(), "auto_queue_enabled", value, userID); err != nil {
		a.internalError(w, r, "update auto-queue", err)
		return
	}
	loggerFromContext(r.Context(), a.logger).Info("auto-queue toggled", "enabled", request.Enabled)
	a.autoQueueStatus(w, r)
}

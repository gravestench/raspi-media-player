package app

import (
	"net/http"
	"strconv"
	"strings"
)

func (a *application) searchYouTube(w http.ResponseWriter, r *http.Request) {
	if a.youtube == nil {
		writeError(w, http.StatusServiceUnavailable, "youtube_search_unavailable", "YouTube search is not enabled")
		return
	}
	if enabled, err := a.settings.Value(r.Context(), "youtube_search_enabled"); err == nil && enabled != "true" {
		writeError(w, http.StatusNotFound, "youtube_search_disabled", "YouTube search is disabled")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" || len(query) > 120 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_search_query", "search query must be between 1 and 120 characters")
		return
	}
	limit := 8
	if configured, err := a.settings.Value(r.Context(), "youtube_search_results"); err == nil {
		if parsed, parseErr := strconv.Atoi(configured); parseErr == nil && parsed >= 1 && parsed <= 20 {
			limit = parsed
		}
	}
	results, err := a.youtube.Search(r.Context(), query, limit)
	if err != nil {
		loggerFromContext(r.Context(), a.logger).Warn("YouTube search failed", "error", err)
		writeError(w, http.StatusBadGateway, "youtube_search_failed", "YouTube search is temporarily unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": query, "results": results})
}

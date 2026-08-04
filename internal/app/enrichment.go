package app

import (
	"errors"
	"github.com/dylanknuth/raspi-media-player/internal/enrichment"
	"net/http"
	"strings"
)

func (a *application) getEnrichment(w http.ResponseWriter, r *http.Request) {
	title := strings.TrimSpace(r.URL.Query().Get("title"))
	if title == "" {
		writeError(w, http.StatusBadRequest, "title_required", "title is required")
		return
	}
	if a.enrichment == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enrichment": enrichment.Result{Hint: enrichment.ParseTitle(title), Status: "disabled"}})
		return
	}
	value, err := a.enrichment.Lookup(r.Context(), title)
	if errors.Is(err, enrichment.ErrProviderNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"enrichment": value})
		return
	}
	if err != nil {
		a.internalError(w, r, "artist enrichment", err)
		return
	}
	status := http.StatusOK
	if value.Status == "pending" {
		status = http.StatusAccepted
	}
	writeJSON(w, status, map[string]any{"enrichment": value})
}

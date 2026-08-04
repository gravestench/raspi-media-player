package app

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/dylanknuth/raspi-media-player/internal/enrichment"
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

func (a *application) getEnrichmentImage(w http.ResponseWriter, r *http.Request) {
	if a.imageCache == nil {
		http.NotFound(w, r)
		return
	}
	data, mime, err := a.imageCache.Read(r.PathValue("key"))
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		a.internalError(w, r, "read artist image", err)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

package app

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/dylanknuth/raspi-media-player/internal/library"
)

type stationRequest struct {
	Name      string `json:"name"`
	StreamURL string `json:"stream_url"`
}

func (a *application) accountDashboard(w http.ResponseWriter, r *http.Request) {
	identity := requireIdentity(w, r)
	if identity == nil {
		return
	}
	userID := identity.Session.User.ID
	history, err := a.library.ListUserHistory(r.Context(), userID, 100)
	if err != nil {
		a.internalError(w, r, "list account history", err)
		return
	}
	stations, err := a.library.ListStations(r.Context(), userID, "")
	if err != nil {
		a.internalError(w, r, "list account stations", err)
		return
	}
	favorites := make([]library.Station, 0)
	for _, station := range stations {
		if station.Favorite {
			favorites = append(favorites, station)
		}
	}
	playlists, err := a.library.ListPlaylists(r.Context(), userID, "")
	if err != nil {
		a.internalError(w, r, "list account playlists", err)
		return
	}
	genreCounts := map[string]int{}
	if a.enrichment != nil {
		for _, item := range history {
			if item.Title == "" {
				continue
			}
			value, lookupErr := a.enrichment.Lookup(r.Context(), item.Title)
			if lookupErr != nil || value.Status != "ready" {
				continue
			}
			for _, genre := range value.Genres {
				genreCounts[genre]++
			}
		}
	}
	type genreCount struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	genres := make([]genreCount, 0, len(genreCounts))
	for name, count := range genreCounts {
		genres = append(genres, genreCount{Name: name, Count: count})
	}
	sort.Slice(genres, func(i, j int) bool {
		if genres[i].Count == genres[j].Count {
			return genres[i].Name < genres[j].Name
		}
		return genres[i].Count > genres[j].Count
	})
	if len(genres) > 20 {
		genres = genres[:20]
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": identity.Session.User, "genres": genres, "recent": history, "favorites": favorites, "playlists": playlists})
}

type favoriteRequest struct {
	Favorite bool `json:"favorite"`
}
type playlistRequest struct {
	Name string `json:"name"`
}
type playlistItemRequest struct {
	Name       string `json:"name"`
	SourceKind string `json:"source_kind"`
	SourceURL  string `json:"source_url"`
}

func userID(r *http.Request) string {
	if identity := identityFromContext(r.Context()); identity != nil {
		return identity.Session.User.ID
	}
	return ""
}
func requireIdentity(w http.ResponseWriter, r *http.Request) *Identity {
	identity := identityFromContext(r.Context())
	if identity == nil {
		writeError(w, http.StatusUnauthorized, "authentication_required", "sign in to use personal library features")
	}
	return identity
}

func (a *application) listStations(w http.ResponseWriter, r *http.Request) {
	values, err := a.library.ListStations(r.Context(), userID(r), r.URL.Query().Get("q"))
	if err != nil {
		a.internalError(w, r, "list stations", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stations": values})
}
func (a *application) listFavorites(w http.ResponseWriter, r *http.Request) {
	identity := requireIdentity(w, r)
	if identity == nil {
		return
	}
	values, err := a.library.ListStations(r.Context(), identity.Session.User.ID, r.URL.Query().Get("q"))
	if err != nil {
		a.internalError(w, r, "list favorites", err)
		return
	}
	favorites := make([]library.Station, 0)
	for _, value := range values {
		if value.Favorite {
			favorites = append(favorites, value)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"stations": favorites})
}

func (a *application) createStation(w http.ResponseWriter, r *http.Request) {
	identity := requireIdentity(w, r)
	if identity == nil {
		return
	}
	var request stationRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	request.StreamURL = strings.TrimSpace(request.StreamURL)
	if err := validateStreamURL(request.StreamURL); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_url", err.Error())
		return
	}
	value, err := a.library.CreateStation(r.Context(), identity.Session.User.ID, request.Name, request.StreamURL)
	if err != nil {
		a.libraryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (a *application) deleteStation(w http.ResponseWriter, r *http.Request) {
	identity := requireIdentity(w, r)
	if identity == nil {
		return
	}
	if err := a.library.DeleteStation(r.Context(), identity.Session.User.ID, r.PathValue("id")); err != nil {
		a.libraryError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *application) favoriteStation(w http.ResponseWriter, r *http.Request) {
	identity := requireIdentity(w, r)
	if identity == nil {
		return
	}
	var request favoriteRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := a.library.SetFavorite(r.Context(), identity.Session.User.ID, r.PathValue("id"), request.Favorite); err != nil {
		a.libraryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"favorite": request.Favorite})
}

func (a *application) listPlaylists(w http.ResponseWriter, r *http.Request) {
	identity := requireIdentity(w, r)
	if identity == nil {
		return
	}
	values, err := a.library.ListPlaylists(r.Context(), identity.Session.User.ID, r.URL.Query().Get("q"))
	if err != nil {
		a.internalError(w, r, "list playlists", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"playlists": values})
}
func (a *application) createPlaylist(w http.ResponseWriter, r *http.Request) {
	identity := requireIdentity(w, r)
	if identity == nil {
		return
	}
	var request playlistRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	value, err := a.library.CreatePlaylist(r.Context(), identity.Session.User.ID, request.Name)
	if err != nil {
		a.libraryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (a *application) deletePlaylist(w http.ResponseWriter, r *http.Request) {
	identity := requireIdentity(w, r)
	if identity == nil {
		return
	}
	if err := a.library.DeletePlaylist(r.Context(), identity.Session.User.ID, r.PathValue("id")); err != nil {
		a.libraryError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *application) addPlaylistItem(w http.ResponseWriter, r *http.Request) {
	identity := requireIdentity(w, r)
	if identity == nil {
		return
	}
	var request playlistItemRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.SourceKind == "" {
		request.SourceKind = "direct"
	}
	if request.SourceKind != "direct" {
		writeError(w, http.StatusUnprocessableEntity, "unsupported_source", "only direct sources are currently supported")
		return
	}
	request.SourceURL = strings.TrimSpace(request.SourceURL)
	if err := validateStreamURL(request.SourceURL); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_url", err.Error())
		return
	}
	value, err := a.library.AddPlaylistItem(r.Context(), identity.Session.User.ID, r.PathValue("id"), request.Name, request.SourceKind, request.SourceURL)
	if err != nil {
		a.libraryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (a *application) removePlaylistItem(w http.ResponseWriter, r *http.Request) {
	identity := requireIdentity(w, r)
	if identity == nil {
		return
	}
	if err := a.library.RemovePlaylistItem(r.Context(), identity.Session.User.ID, r.PathValue("id"), r.PathValue("itemID")); err != nil {
		a.libraryError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *application) listHistory(w http.ResponseWriter, r *http.Request) {
	values, err := a.library.ListHistory(r.Context(), r.URL.Query().Get("q"), 100)
	if err != nil {
		a.internalError(w, r, "list history", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": values})
}
func (a *application) searchLibrary(w http.ResponseWriter, r *http.Request) {
	results, err := a.library.Search(r.Context(), userID(r), r.URL.Query().Get("q"))
	if err != nil {
		a.internalError(w, r, "search library", err)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (a *application) libraryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusNotFound, "library_item_not_found", "library item was not found")
	case errors.Is(err, library.ErrConflict):
		writeError(w, http.StatusConflict, "library_item_conflict", "an item with this name already exists")
	case strings.Contains(err.Error(), "must be"):
		writeError(w, http.StatusUnprocessableEntity, "invalid_library_item", err.Error())
	default:
		a.internalError(w, r, "library operation", err)
	}
}

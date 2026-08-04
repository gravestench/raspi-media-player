package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestLibraryAPIOwnershipAndFavorites(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{ArgonMemory: 1024, ArgonIterations: 1, AuthRate: 20, SessionLifetime: time.Hour})
	public := authRequest(t, handler, http.MethodGet, "/api/v1/stations?q=KFJC", "", nil, "")
	if public.Code != http.StatusOK || !bytes.Contains(public.Body.Bytes(), []byte("household-kfjc")) {
		t.Fatalf("public stations: %d %s", public.Code, public.Body.String())
	}
	unauthorized := authRequest(t, handler, http.MethodPost, "/api/v1/stations", `{"name":"Mine","stream_url":"https://example.com/mine"}`, nil, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous personal station: %d", unauthorized.Code)
	}
	signup := authRequest(t, handler, http.MethodPost, "/api/v1/auth/signup", `{"username":"collector","password":"collector-password","password_confirmation":"collector-password"}`, nil, "")
	account := decodeAuth(t, signup)
	cookies := sessionCookies(signup)
	favorite := authRequest(t, handler, http.MethodPut, "/api/v1/stations/household-kfjc/favorite", `{"favorite":true}`, cookies, account.CSRFToken)
	if favorite.Code != http.StatusOK {
		t.Fatalf("favorite: %d %s", favorite.Code, favorite.Body.String())
	}
	favorites := authRequest(t, handler, http.MethodGet, "/api/v1/favorites", "", cookies, "")
	if !bytes.Contains(favorites.Body.Bytes(), []byte("household-kfjc")) {
		t.Fatalf("favorites missing: %s", favorites.Body.String())
	}
	station := authRequest(t, handler, http.MethodPost, "/api/v1/stations", `{"name":"Personal Radio","stream_url":"https://example.com/personal.mp3"}`, cookies, account.CSRFToken)
	if station.Code != http.StatusCreated {
		t.Fatalf("create station: %d %s", station.Code, station.Body.String())
	}
	playlist := authRequest(t, handler, http.MethodPost, "/api/v1/playlists", `{"name":"Kitchen"}`, cookies, account.CSRFToken)
	if playlist.Code != http.StatusCreated {
		t.Fatalf("create playlist: %d %s", playlist.Code, playlist.Body.String())
	}
	var playlistBody struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(playlist.Body).Decode(&playlistBody); err != nil {
		t.Fatal(err)
	}
	item := authRequest(t, handler, http.MethodPost, "/api/v1/playlists/"+playlistBody.ID+"/items", `{"name":"Radio","source_url":"https://example.com/personal.mp3"}`, cookies, account.CSRFToken)
	if item.Code != http.StatusCreated {
		t.Fatalf("add playlist item: %d %s", item.Code, item.Body.String())
	}
	search := authRequest(t, handler, http.MethodGet, "/api/v1/library/search?q=Kitchen", "", cookies, "")
	if search.Code != http.StatusOK || !bytes.Contains(search.Body.Bytes(), []byte("Kitchen")) {
		t.Fatalf("search: %d %s", search.Code, search.Body.String())
	}
}

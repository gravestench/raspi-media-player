package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
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

func TestLikeQueueItemAddsTrackToUserProfile(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{ArgonMemory: 1024, ArgonIterations: 1, AuthRate: 20, SessionLifetime: time.Hour})
	signup := authRequest(t, handler, http.MethodPost, "/api/v1/auth/signup", `{"username":"listener","password":"listener-password","password_confirmation":"listener-password"}`, nil, "")
	account := decodeAuth(t, signup)
	cookies := sessionCookies(signup)
	queued := authRequest(t, handler, http.MethodPost, "/api/v1/queue/items", `{"url":"https://www.youtube.com/watch?v=liked123","title":"Artist Name - Recommended Song"}`, nil, "")
	if queued.Code != http.StatusCreated {
		t.Fatalf("queue: %d %s", queued.Code, queued.Body.String())
	}
	var snapshot struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(queued.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("queue items: %+v", snapshot.Items)
	}
	liked := authRequest(t, handler, http.MethodPut, "/api/v1/queue/items/"+snapshot.Items[0].ID+"/like", `{}`, cookies, account.CSRFToken)
	if liked.Code != http.StatusOK || !bytes.Contains(liked.Body.Bytes(), []byte(`"liked":true`)) {
		t.Fatalf("like: %d %s", liked.Code, liked.Body.String())
	}
	dashboard := authRequest(t, handler, http.MethodGet, "/api/v1/account", "", cookies, "")
	if dashboard.Code != http.StatusOK || !bytes.Contains(dashboard.Body.Bytes(), []byte("Recommended Song")) {
		t.Fatalf("account likes: %d %s", dashboard.Code, dashboard.Body.String())
	}
	duplicate := authRequest(t, handler, http.MethodPut, "/api/v1/queue/items/"+snapshot.Items[0].ID+"/like", `{}`, cookies, account.CSRFToken)
	if duplicate.Code != http.StatusOK {
		t.Fatalf("repeat like: %d %s", duplicate.Code, duplicate.Body.String())
	}
	removePath := "/api/v1/account/likes?source_url=" + url.QueryEscape("https://www.youtube.com/watch?v=liked123") + "&title=" + url.QueryEscape("Artist Name - Recommended Song")
	removed := authRequest(t, handler, http.MethodDelete, removePath, "", cookies, account.CSRFToken)
	if removed.Code != http.StatusNoContent {
		t.Fatalf("remove like: %d %s", removed.Code, removed.Body.String())
	}
	dashboard = authRequest(t, handler, http.MethodGet, "/api/v1/account", "", cookies, "")
	if bytes.Contains(dashboard.Body.Bytes(), []byte("Recommended Song")) {
		t.Fatalf("removed like still in account: %s", dashboard.Body.String())
	}
}

package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type authResponse struct {
	Status    string `json:"status"`
	Username  string `json:"username"`
	CSRFToken string `json:"csrf_token"`
	Session   struct {
		ID   string `json:"id"`
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	} `json:"session"`
}

func authRequest(t *testing.T, handler http.Handler, method, path, body string, cookies []*http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeAuth(t *testing.T, response *httptest.ResponseRecorder) authResponse {
	t.Helper()
	var body authResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}
func sessionCookies(response *httptest.ResponseRecorder) []*http.Cookie {
	result := make([]*http.Cookie, 0)
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookieName || cookie.Name == csrfCookieName {
			result = append(result, cookie)
		}
	}
	return result
}

func TestUnknownLoginSignupSessionCSRFAndAttribution(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{ArgonMemory: 1024, ArgonIterations: 1, AuthRate: 20, SessionLifetime: time.Hour})
	available := authRequest(t, handler, http.MethodGet, "/api/v1/auth/usernames/HouseDJ", "", nil, "")
	if available.Code != http.StatusOK || !bytes.Contains(available.Body.Bytes(), []byte(`"available":true`)) {
		t.Fatalf("availability: %d %s", available.Code, available.Body.String())
	}
	unknown := authRequest(t, handler, http.MethodPost, "/api/v1/auth/login", `{"username":"HouseDJ","password":"house-password"}`, nil, "")
	if unknown.Code != http.StatusOK || decodeAuth(t, unknown).Status != "account_creation_required" {
		t.Fatalf("unknown login: %d %s", unknown.Code, unknown.Body.String())
	}
	mismatch := authRequest(t, handler, http.MethodPost, "/api/v1/auth/signup", `{"username":"HouseDJ","password":"house-password","password_confirmation":"different-password"}`, nil, "")
	if mismatch.Code != http.StatusUnprocessableEntity {
		t.Fatalf("confirmation mismatch: %d", mismatch.Code)
	}
	signup := authRequest(t, handler, http.MethodPost, "/api/v1/auth/signup", `{"username":"HouseDJ","password":"house-password","password_confirmation":"house-password"}`, nil, "")
	if signup.Code != http.StatusCreated {
		t.Fatalf("signup: %d %s", signup.Code, signup.Body.String())
	}
	authBody := decodeAuth(t, signup)
	cookies := sessionCookies(signup)
	if len(cookies) != 2 || authBody.CSRFToken == "" || authBody.Session.User.Username != "HouseDJ" {
		t.Fatalf("incomplete session response: %+v cookies=%d", authBody, len(cookies))
	}
	for _, cookie := range cookies {
		if cookie.Name == sessionCookieName && (!cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode) {
			t.Fatalf("unsafe session cookie: %+v", cookie)
		}
	}
	unavailable := authRequest(t, handler, http.MethodGet, "/api/v1/auth/usernames/housedj", "", nil, "")
	if !bytes.Contains(unavailable.Body.Bytes(), []byte(`"available":false`)) {
		t.Fatalf("used username reported available: %s", unavailable.Body.String())
	}
	current := authRequest(t, handler, http.MethodGet, "/api/v1/auth/session", "", cookies, "")
	if current.Code != http.StatusOK || !bytes.Contains(current.Body.Bytes(), []byte(`"authenticated":true`)) {
		t.Fatalf("current session: %d %s", current.Code, current.Body.String())
	}
	if !bytes.Contains(logs.Bytes(), []byte(`"route":"GET /api/v1/auth/session"`)) || !bytes.Contains(logs.Bytes(), []byte(`"username":"HouseDJ"`)) {
		t.Fatalf("authenticated request log missing route or identity: %s", logs.String())
	}
	withoutCSRF := authRequest(t, handler, http.MethodPost, "/api/v1/queue/items", `{"url":"https://example.com/member.mp3"}`, cookies, "")
	if withoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF: %d", withoutCSRF.Code)
	}
	withCSRF := authRequest(t, handler, http.MethodPost, "/api/v1/queue/items", `{"url":"https://example.com/member.mp3"}`, cookies, authBody.CSRFToken)
	if withCSRF.Code != http.StatusCreated {
		t.Fatalf("authenticated queue: %d %s", withCSRF.Code, withCSRF.Body.String())
	}
	snapshot := snapshotFrom(t, withCSRF)
	if snapshot.Items[0].Submitter.Kind != "user" || snapshot.Items[0].Submitter.Username != "HouseDJ" || snapshot.Items[0].Submitter.DisplayName != "" {
		t.Fatalf("wrong attribution: %+v", snapshot.Items[0].Submitter)
	}
	wrong := authRequest(t, handler, http.MethodPost, "/api/v1/auth/login", `{"username":"HouseDJ","password":"wrong-password"}`, nil, "")
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", wrong.Code)
	}
	duplicate := authRequest(t, handler, http.MethodPost, "/api/v1/auth/signup", `{"username":"housedj","password":"another-password","password_confirmation":"another-password"}`, nil, "")
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate username: %d %s", duplicate.Code, duplicate.Body.String())
	}
	logout := authRequest(t, handler, http.MethodPost, "/api/v1/auth/logout", `{}`, cookies, authBody.CSRFToken)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout: %d %s", logout.Code, logout.Body.String())
	}
	revoked := authRequest(t, handler, http.MethodGet, "/api/v1/auth/session", "", cookies, "")
	if !bytes.Contains(revoked.Body.Bytes(), []byte(`"authenticated":false`)) {
		t.Fatalf("revoked session remained active: %s", revoked.Body.String())
	}
}

func TestAuthenticationRateLimit(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{AuthRate: 1})
	first := authRequest(t, handler, http.MethodPost, "/api/v1/auth/login", `{"username":"unknown","password":"some-password"}`, nil, "")
	if first.Code != http.StatusOK {
		t.Fatalf("first attempt: %d", first.Code)
	}
	second := authRequest(t, handler, http.MethodPost, "/api/v1/auth/login", `{"username":"unknown","password":"some-password"}`, nil, "")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d", second.Code)
	}
}

func TestConcurrentSignupAllowsOneAccount(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{ArgonMemory: 1024, ArgonIterations: 1, AuthRate: 20})
	statuses := make(chan int, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response := authRequest(t, handler, http.MethodPost, "/api/v1/auth/signup", `{"username":"simultaneous","password":"some-password","password_confirmation":"some-password"}`, nil, "")
			statuses <- response.Code
		}()
	}
	wait.Wait()
	close(statuses)
	created, conflicts := 0, 0
	for status := range statuses {
		if status == http.StatusCreated {
			created++
		}
		if status == http.StatusConflict {
			conflicts++
		}
	}
	if created != 1 || conflicts != 1 {
		t.Fatalf("concurrent signup statuses: created=%d conflicts=%d", created, conflicts)
	}
}

func TestRequiredModeRejectsAnonymousQueueMutation(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{AccessMode: "accounts_required"})
	response := authRequest(t, handler, http.MethodPost, "/api/v1/queue/items", `{"url":"https://example.com/a.mp3"}`, nil, "")
	if response.Code != http.StatusUnauthorized || !bytes.Contains(response.Body.Bytes(), []byte("authentication_required")) {
		t.Fatalf("required mode: %d %s", response.Code, response.Body.String())
	}
	listing := authRequest(t, handler, http.MethodGet, "/api/v1/queue", "", nil, "")
	if listing.Code != http.StatusOK {
		t.Fatalf("public queue listing: %d", listing.Code)
	}
}

func TestSessionListingAndRevocation(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{ArgonMemory: 1024, ArgonIterations: 1, AuthRate: 20, SessionLifetime: time.Hour})
	signup := authRequest(t, handler, http.MethodPost, "/api/v1/auth/signup", `{"username":"listener","password":"listener-password","password_confirmation":"listener-password"}`, nil, "")
	first := decodeAuth(t, signup)
	firstCookies := sessionCookies(signup)
	login := authRequest(t, handler, http.MethodPost, "/api/v1/auth/login", `{"username":"listener","password":"listener-password"}`, nil, "")
	second := decodeAuth(t, login)
	secondCookies := sessionCookies(login)
	listing := authRequest(t, handler, http.MethodGet, "/api/v1/auth/sessions", "", secondCookies, "")
	if listing.Code != http.StatusOK || !bytes.Contains(listing.Body.Bytes(), []byte(first.Session.ID)) {
		t.Fatalf("session list: %d %s", listing.Code, listing.Body.String())
	}
	revoke := authRequest(t, handler, http.MethodDelete, "/api/v1/auth/sessions/"+first.Session.ID, "", secondCookies, second.CSRFToken)
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", revoke.Code, revoke.Body.String())
	}
	old := authRequest(t, handler, http.MethodGet, "/api/v1/auth/session", "", firstCookies, "")
	if !bytes.Contains(old.Body.Bytes(), []byte(`"authenticated":false`)) {
		t.Fatalf("revoked other session active: %s", old.Body.String())
	}
}

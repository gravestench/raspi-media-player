package app

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/settings"
)

func TestValidateAutoQueueSettings(t *testing.T) {
	for _, test := range []struct {
		key, value string
		valid      bool
	}{
		{"auto_queue_enabled", "true", true},
		{"auto_queue_enabled", "sometimes", false},
		{"auto_queue_depth", "1", true},
		{"auto_queue_depth", "21", false},
		{"auto_queue_active_seconds", "30", true},
		{"auto_queue_active_seconds", "29", false},
		{"auto_queue_mode", "related_last", true},
		{"auto_queue_mode", "anything", false},
	} {
		err := validateSetting(test.key, test.value)
		if (err == nil) != test.valid {
			t.Errorf("validateSetting(%q, %q) error=%v valid=%v", test.key, test.value, err, test.valid)
		}
	}
}

func TestGuidedSetupCreatesAdminAndProtectsConfiguration(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{
		SetupRequired:     true,
		ArgonMemory:       1024,
		ArgonIterations:   1,
		AuthRate:          20,
		SessionLifetime:   time.Hour,
		SettingsSecretKey: "test-only-settings-key",
		Settings: []settings.Definition{
			{Key: "access_mode", Label: "Access", Category: "Access", Type: "select", Value: "open"},
			{Key: "lastfm_api_key", Label: "Last.fm API key", Category: "Metadata", Type: "secret", Secret: true, Value: "environment-key", RestartRequired: true},
		},
	})
	blocked := authRequest(t, handler, http.MethodGet, "/api/v1/queue", "", nil, "")
	if blocked.Code != http.StatusServiceUnavailable || !bytes.Contains(blocked.Body.Bytes(), []byte(`"setup_required"`)) {
		t.Fatalf("uninstalled API was not blocked: %d %s", blocked.Code, blocked.Body.String())
	}
	status := authRequest(t, handler, http.MethodGet, "/api/v1/setup/status", "", nil, "")
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"installed":false`)) {
		t.Fatalf("setup status: %d %s", status.Code, status.Body.String())
	}
	setup := authRequest(t, handler, http.MethodPost, "/api/v1/setup/complete", `{"username":"HouseAdmin","password":"admin-password","password_confirmation":"admin-password","access_mode":"open","lastfm_api_key":"super-secret-key"}`, nil, "")
	if setup.Code != http.StatusCreated || !bytes.Contains(setup.Body.Bytes(), []byte(`"is_admin":true`)) {
		t.Fatalf("setup: %d %s", setup.Code, setup.Body.String())
	}
	admin := decodeAuth(t, setup)
	adminCookies := sessionCookies(setup)
	settingsResponse := authRequest(t, handler, http.MethodGet, "/api/v1/admin/settings", "", adminCookies, "")
	if settingsResponse.Code != http.StatusOK || bytes.Contains(settingsResponse.Body.Bytes(), []byte("super-secret-key")) || bytes.Contains(settingsResponse.Body.Bytes(), []byte("environment-key")) || !bytes.Contains(settingsResponse.Body.Bytes(), []byte(`"configured":true`)) {
		t.Fatalf("settings response leaked or omitted secret state: %d %s", settingsResponse.Code, settingsResponse.Body.String())
	}
	memberSignup := authRequest(t, handler, http.MethodPost, "/api/v1/auth/signup", `{"username":"Member","password":"member-password","password_confirmation":"member-password"}`, nil, "")
	if memberSignup.Code != http.StatusCreated {
		t.Fatalf("member signup: %d %s", memberSignup.Code, memberSignup.Body.String())
	}
	member := decodeAuth(t, memberSignup)
	nonAdmin := authRequest(t, handler, http.MethodGet, "/api/v1/admin/users", "", sessionCookies(memberSignup), "")
	if nonAdmin.Code != http.StatusForbidden {
		t.Fatalf("non-admin access: %d", nonAdmin.Code)
	}
	promote := authRequest(t, handler, http.MethodPut, "/api/v1/admin/users/"+member.Session.User.ID+"/role", `{"admin":true}`, adminCookies, admin.CSRFToken)
	if promote.Code != http.StatusOK {
		t.Fatalf("promote: %d %s", promote.Code, promote.Body.String())
	}
	demoteOriginal := authRequest(t, handler, http.MethodPut, "/api/v1/admin/users/"+admin.Session.User.ID+"/role", `{"admin":false}`, adminCookies, admin.CSRFToken)
	if demoteOriginal.Code != http.StatusOK {
		t.Fatalf("demote with replacement: %d %s", demoteOriginal.Code, demoteOriginal.Body.String())
	}
	if bytes.Contains(logs.Bytes(), []byte("super-secret-key")) {
		t.Fatal("Last.fm key appeared in structured logs")
	}
}

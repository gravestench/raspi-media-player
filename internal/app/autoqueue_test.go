package app

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/dylanknuth/raspi-media-player/internal/settings"
)

func TestHouseholdCanToggleAutoQueueInOpenMode(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{Settings: []settings.Definition{
		{Key: "auto_queue_enabled", Value: "false", Type: "boolean"},
		{Key: "auto_queue_depth", Value: "4", Type: "number"},
		{Key: "auto_queue_mode", Value: "active_users", Type: "select"},
		{Key: "auto_queue_artists", Value: "", Type: "text"},
		{Key: "auto_queue_genres", Value: "", Type: "text"},
	}})
	initial := authRequest(t, handler, http.MethodGet, "/api/v1/autoqueue", "", nil, "")
	if initial.Code != http.StatusOK || !strings.Contains(initial.Body.String(), `"enabled":false`) || !strings.Contains(initial.Body.String(), `"depth":"4"`) {
		t.Fatalf("initial status: %d %s", initial.Code, initial.Body.String())
	}
	updated := authRequest(t, handler, http.MethodPut, "/api/v1/autoqueue", `{"enabled":true}`, nil, "")
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"enabled":true`) {
		t.Fatalf("updated status: %d %s", updated.Code, updated.Body.String())
	}
	current := authRequest(t, handler, http.MethodGet, "/api/v1/autoqueue", "", nil, "")
	if current.Code != http.StatusOK || !strings.Contains(current.Body.String(), `"enabled":true`) {
		t.Fatalf("persisted status: %d %s", current.Code, current.Body.String())
	}
	strategy := authRequest(t, handler, http.MethodPut, "/api/v1/autoqueue", `{"mode":"specific_seeds","artists":"Björk, Stereolab","genres":"dream pop"}`, nil, "")
	if strategy.Code != http.StatusOK || !strings.Contains(strategy.Body.String(), `"mode":"specific_seeds"`) || !strings.Contains(strategy.Body.String(), `"genres":"dream pop"`) {
		t.Fatalf("strategy status: %d %s", strategy.Code, strategy.Body.String())
	}
}

func TestAutoQueueToggleHonorsAccountsRequiredMode(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{AccessMode: "accounts_required", Settings: []settings.Definition{{Key: "auto_queue_enabled", Value: "false", Type: "boolean"}}})
	response := authRequest(t, handler, http.MethodPut, "/api/v1/autoqueue", `{"enabled":true}`, nil, "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

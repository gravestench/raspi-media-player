package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestSensitiveValuesAreRedacted(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(&output, "json", "debug")
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("test",
		"username", "dylan",
		"password", "plain-password",
		"session_token", "session-value",
		slog.Group("request", "authorization", "Bearer private-value", "safe", "visible"),
	)
	logged := output.String()
	for _, secret := range []string{"plain-password", "session-value", "private-value"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("secret %q was logged: %s", secret, logged)
		}
	}
	if strings.Count(logged, "[REDACTED]") != 3 {
		t.Fatalf("expected three redactions: %s", logged)
	}
	if !strings.Contains(logged, "visible") || !strings.Contains(logged, "dylan") {
		t.Fatalf("safe values were removed: %s", logged)
	}
}

func TestHandlerOptionsArePreserved(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(&output, "text", "warn")
	if err != nil {
		t.Fatal(err)
	}
	logger.InfoContext(context.Background(), "hidden")
	logger.WarnContext(context.Background(), "shown")
	if strings.Contains(output.String(), "hidden") || !strings.Contains(output.String(), "shown") {
		t.Fatalf("unexpected level filtering: %s", output.String())
	}
}

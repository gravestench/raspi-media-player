#!/bin/sh
set -eu

TEST_BASE_URL=${TEST_BASE_URL:-http://127.0.0.1:8080}
AUTH_TMP=$(mktemp -d "${TMPDIR:-/tmp}/raspi-auth-test.XXXXXX")
trap 'rm -rf "$AUTH_TMP"' EXIT INT TERM
COOKIE_JAR=$AUTH_TMP/cookies

unknown=$(curl --silent --fail --request POST --header 'Content-Type: application/json' --data '{"username":"api_tester","password":"test-password"}' "$TEST_BASE_URL/api/v1/auth/login")
[ "$(printf '%s' "$unknown" | jq -r .status)" = account_creation_required ] || { echo "unknown login did not request account creation" >&2; exit 1; }
signup=$(curl --silent --fail --cookie-jar "$COOKIE_JAR" --request POST --header 'Content-Type: application/json' --data '{"username":"api_tester","password":"test-password","password_confirmation":"test-password"}' "$TEST_BASE_URL/api/v1/auth/signup")
csrf=$(printf '%s' "$signup" | jq -r .csrf_token)
[ "$(printf '%s' "$signup" | jq -r .status)" = authenticated ] || { echo "signup did not authenticate" >&2; exit 1; }
current=$(curl --silent --fail --cookie "$COOKIE_JAR" "$TEST_BASE_URL/api/v1/auth/session")
[ "$(printf '%s' "$current" | jq -r .authenticated)" = true ] || { echo "session cookie was not accepted" >&2; exit 1; }
status=$(curl --silent --output /dev/null --write-out '%{http_code}' --cookie "$COOKIE_JAR" --request POST --header 'Content-Type: application/json' --data '{"url":"https://example.com/auth-test.mp3"}' "$TEST_BASE_URL/api/v1/queue/items")
[ "$status" = 403 ] || { echo "authenticated mutation without CSRF: expected 403, got $status" >&2; exit 1; }
curl --silent --fail --cookie "$COOKIE_JAR" --request POST --header "X-CSRF-Token: $csrf" "$TEST_BASE_URL/api/v1/auth/logout" >/dev/null

if [ -n "${TEST_SERVER_BINARY:-}" ]; then
    REQUIRED_TMP=$AUTH_TMP/required
    REQUIRED_PORT=${TEST_REQUIRED_PORT:-18081}
    mkdir -p "$REQUIRED_TMP"
    "$TEST_SERVER_BINARY" -addr "127.0.0.1:$REQUIRED_PORT" -db "$REQUIRED_TMP/test.sqlite" -access-mode accounts_required >"$REQUIRED_TMP/server.log" 2>&1 &
    REQUIRED_PID=$!
    attempt=0
    until curl --silent --fail "http://127.0.0.1:$REQUIRED_PORT/api/v1/health/ready" >/dev/null; do
        attempt=$((attempt + 1))
        if [ "$attempt" -ge 50 ]; then echo "required-mode server did not start" >&2; kill "$REQUIRED_PID" 2>/dev/null || true; exit 1; fi
        sleep 0.1
    done
    status=$(curl --silent --output /dev/null --write-out '%{http_code}' --request POST --header 'Content-Type: application/json' --data '{"url":"https://example.com/required.mp3"}' "http://127.0.0.1:$REQUIRED_PORT/api/v1/queue/items")
    kill "$REQUIRED_PID" 2>/dev/null || true
    wait "$REQUIRED_PID" 2>/dev/null || true
    [ "$status" = 401 ] || { echo "required mode: expected anonymous mutation 401, got $status" >&2; exit 1; }
fi
echo "local account and session API: passed"

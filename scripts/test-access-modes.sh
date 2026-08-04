#!/bin/sh
set -eu

[ -n "${TEST_SERVER_BINARY:-}" ] || { echo "access modes: TEST_SERVER_BINARY is required" >&2; exit 1; }
MODE_TMP=$(mktemp -d "${TMPDIR:-/tmp}/raspi-access-mode-test.XXXXXX")
PIDS=""
cleanup() {
    for pid in $PIDS; do kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; done
    rm -rf "$MODE_TMP"
}
trap cleanup EXIT INT TERM

start_mode() {
    mode=$1 port=$2
    "$TEST_SERVER_BINARY" -addr "127.0.0.1:$port" -db "$MODE_TMP/$mode.sqlite" -access-mode "$mode" -player-enabled=false -setup-required=false >"$MODE_TMP/$mode.log" 2>&1 &
    pid=$!; PIDS="$PIDS $pid"
    attempt=0
    until curl --silent --fail "http://127.0.0.1:$port/api/v1/health/ready" >/dev/null; do
        attempt=$((attempt + 1)); [ "$attempt" -lt 50 ] || { echo "$mode server failed to start" >&2; exit 1; }; sleep 0.1
    done
}

OPTIONAL_PORT=${TEST_OPTIONAL_PORT:-18081}
REQUIRED_PORT=${TEST_REQUIRED_PORT:-18082}
start_mode accounts_optional "$OPTIONAL_PORT"
start_mode accounts_required "$REQUIRED_PORT"

optional_status=$(curl --silent --output /dev/null --write-out '%{http_code}' --request POST --header 'Content-Type: application/json' --data '{"url":"https://example.com/optional.mp3"}' "http://127.0.0.1:$OPTIONAL_PORT/api/v1/queue/items")
[ "$optional_status" = 201 ] || { echo "accounts_optional anonymous queue: expected 201, got $optional_status" >&2; exit 1; }

required_status=$(curl --silent --output /dev/null --write-out '%{http_code}' --request POST --header 'Content-Type: application/json' --data '{"url":"https://example.com/required.mp3"}' "http://127.0.0.1:$REQUIRED_PORT/api/v1/queue/items")
[ "$required_status" = 401 ] || { echo "accounts_required anonymous queue: expected 401, got $required_status" >&2; exit 1; }

jar=$MODE_TMP/required.cookies
signup=$(curl --silent --fail --cookie-jar "$jar" --request POST --header 'Content-Type: application/json' --data '{"username":"required_user","password":"required-password","password_confirmation":"required-password"}' "http://127.0.0.1:$REQUIRED_PORT/api/v1/auth/signup")
csrf=$(printf '%s' "$signup" | jq -r .csrf_token)
signed_status=$(curl --silent --output /dev/null --write-out '%{http_code}' --cookie "$jar" --request POST --header "X-CSRF-Token: $csrf" --header 'Content-Type: application/json' --data '{"url":"https://example.com/signed-required.mp3"}' "http://127.0.0.1:$REQUIRED_PORT/api/v1/queue/items")
[ "$signed_status" = 201 ] || { echo "accounts_required signed-in queue: expected 201, got $signed_status" >&2; exit 1; }

echo "open, accounts_optional, and accounts_required modes: passed"

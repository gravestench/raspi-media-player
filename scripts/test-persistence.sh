#!/bin/sh
set -eu

[ -n "${TEST_SERVER_BINARY:-}" ] || { echo "persistence: TEST_SERVER_BINARY is required" >&2; exit 1; }
PERSIST_TMP=$(mktemp -d "${TMPDIR:-/tmp}/raspi-persistence-test.XXXXXX")
PORT=${TEST_PERSIST_PORT:-18083}
BASE=http://127.0.0.1:$PORT
PID=""
cleanup() { [ -z "$PID" ] || { kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true; }; rm -rf "$PERSIST_TMP"; }
trap cleanup EXIT INT TERM

start_server() {
    "$TEST_SERVER_BINARY" -addr "127.0.0.1:$PORT" -db "$PERSIST_TMP/player.sqlite" -player-enabled=false -queue-rate 100 >"$PERSIST_TMP/server.log" 2>&1 & PID=$!
    attempt=0
    until curl --silent --fail "$BASE/api/v1/health/ready" >/dev/null; do attempt=$((attempt + 1)); [ "$attempt" -lt 50 ] || { echo "persistence server failed to start" >&2; exit 1; }; sleep 0.1; done
}
stop_server() { kill "$PID"; wait "$PID"; PID=""; }

start_server
jar=$PERSIST_TMP/cookies
signup=$(curl --silent --fail --cookie-jar "$jar" --request POST --header 'Content-Type: application/json' --data '{"username":"persistent_user","password":"persistent-password","password_confirmation":"persistent-password"}' "$BASE/api/v1/auth/signup")
csrf=$(printf '%s' "$signup" | jq -r .csrf_token)
curl --silent --fail --cookie "$jar" --request POST --header "X-CSRF-Token: $csrf" --header 'Content-Type: application/json' --data '{"name":"Persistent Radio","stream_url":"https://example.com/persistent.mp3"}' "$BASE/api/v1/stations" >/dev/null
curl --silent --fail --request POST --header 'Content-Type: application/json' --data '{"url":"https://example.com/persistent.mp3"}' "$BASE/api/v1/queue/items" >/dev/null
stop_server
start_server

[ "$(curl --silent --fail "$BASE/api/v1/queue" | jq -r '.items | length')" = 1 ] || { echo "restart persistence: queue item missing" >&2; exit 1; }
session=$(curl --silent --fail --cookie "$jar" "$BASE/api/v1/auth/session")
[ "$(printf '%s' "$session" | jq -r .authenticated)" = true ] || { echo "restart persistence: session missing" >&2; exit 1; }
[ "$(curl --silent --fail --cookie "$jar" "$BASE/api/v1/stations?q=Persistent" | jq -r '.stations | length')" = 1 ] || { echo "restart persistence: personal station missing" >&2; exit 1; }

revision=$(curl --silent --fail "$BASE/api/v1/queue" | jq -r .revision)
curl --silent --fail --request DELETE --header "If-Match: \"$revision\"" "$BASE/api/v1/queue" >/dev/null
i=1
curl_pids=""
while [ "$i" -le 10 ]; do
    curl --silent --fail --request POST --header 'Content-Type: application/json' --data "{\"url\":\"https://example.com/concurrent-$i.mp3\"}" "$BASE/api/v1/queue/items" >/dev/null &
    curl_pids="$curl_pids $!"
    i=$((i + 1))
done
for curl_pid in $curl_pids; do wait "$curl_pid"; done
queue=$(curl --silent --fail "$BASE/api/v1/queue")
[ "$(printf '%s' "$queue" | jq -r '.items | length')" = 10 ] || { echo "concurrent queue: expected 10 items" >&2; exit 1; }
[ "$(printf '%s' "$queue" | jq -r '[.items[].position] | unique | length')" = 10 ] || { echo "concurrent queue: duplicate positions" >&2; exit 1; }

echo "restart persistence and concurrent queue API: passed"

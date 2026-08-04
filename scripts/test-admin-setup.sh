#!/bin/sh
set -eu

binary=${TEST_SERVER_BINARY:?TEST_SERVER_BINARY is required}
test_tmp=$(mktemp -d "${TMPDIR:-/tmp}/raspi-media-player-setup.XXXXXX")
port=${SETUP_TEST_PORT:-18086}
base_url="http://127.0.0.1:$port"
server_pid=""
cleanup() {
    [ -z "$server_pid" ] || kill "$server_pid" 2>/dev/null || true
    [ -z "$server_pid" ] || wait "$server_pid" 2>/dev/null || true
    rm -rf "$test_tmp"
}
trap cleanup EXIT INT TERM

RASPI_MEDIA_PLAYER_SETTINGS_SECRET_KEY=setup-regression-secret "$binary" -addr "127.0.0.1:$port" -db "$test_tmp/setup.sqlite" -player-enabled=false -metadata-enabled=false -setup-required=true -log-format=json >"$test_tmp/server.log" 2>&1 &
server_pid=$!
attempt=0
until curl --silent --fail "$base_url/api/v1/health/ready" >/dev/null; do
    attempt=$((attempt + 1)); [ "$attempt" -lt 50 ] || { cat "$test_tmp/server.log" >&2; exit 1; }; sleep 0.1
done

[ "$(curl --silent "$base_url/api/v1/setup/status" | jq -r .installed)" = false ] || { echo "fresh database unexpectedly installed" >&2; exit 1; }
[ "$(curl --silent --output /dev/null --write-out '%{http_code}' "$base_url/api/v1/queue")" = 503 ] || { echo "player API available before setup" >&2; exit 1; }
setup=$(curl --silent --fail --cookie-jar "$test_tmp/cookies" -H 'Content-Type: application/json' -d '{"username":"SetupAdmin","password":"setup-password","password_confirmation":"setup-password","access_mode":"open","lastfm_api_key":"not-a-real-key"}' "$base_url/api/v1/setup/complete")
[ "$(printf '%s' "$setup" | jq -r .session.user.is_admin)" = true ] || { echo "first account was not administrator" >&2; exit 1; }
csrf=$(printf '%s' "$setup" | jq -r .csrf_token)
settings=$(curl --silent --fail --cookie "$test_tmp/cookies" "$base_url/api/v1/admin/settings")
printf '%s' "$settings" | grep -q 'not-a-real-key' && { echo "admin settings leaked secret" >&2; exit 1; }
curl --silent --fail --cookie "$test_tmp/cookies" -H "X-CSRF-Token: $csrf" -H 'Content-Type: application/json' -X PUT -d '{"value":"75"}' "$base_url/api/v1/admin/settings/vote_percent" >/dev/null
[ "$(curl --silent --output /dev/null --write-out '%{http_code}' -X POST "$base_url/api/v1/setup/complete")" = 409 ] || { echo "setup could be repeated" >&2; exit 1; }
echo "guided setup and administrator API: passed"

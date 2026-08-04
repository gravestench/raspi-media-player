#!/bin/sh
set -eu

TEST_BASE_URL=${TEST_BASE_URL:-http://127.0.0.1:8080}
tmp=$(mktemp -d "${TMPDIR:-/tmp}/raspi-media-player-features.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM

signup=$(curl --silent --show-error --fail --cookie-jar "$tmp/cookies" -H 'Content-Type: application/json' -d '{"username":"FeatureUser","password":"feature-password","password_confirmation":"feature-password"}' "$TEST_BASE_URL/api/v1/auth/signup")
[ "$(printf '%s' "$signup" | jq -r .session.user.username)" = FeatureUser ] || { echo "feature account signup failed" >&2; exit 1; }
account=$(curl --silent --show-error --fail --cookie "$tmp/cookies" "$TEST_BASE_URL/api/v1/account")
[ "$(printf '%s' "$account" | jq -r '.genres | type')" = array ] || { echo "account genre summary missing" >&2; exit 1; }
[ "$(printf '%s' "$account" | jq -r '.recent | type')" = array ] || { echo "account recent history missing" >&2; exit 1; }

youtube_status=$(curl --silent --output /dev/null --write-out '%{http_code}' "$TEST_BASE_URL/api/v1/youtube/search")
[ "$youtube_status" = 422 ] || { echo "YouTube search validation: expected 422, got $youtube_status" >&2; exit 1; }

queue=$(curl --silent --show-error --fail "$TEST_BASE_URL/api/v1/queue")
[ "$(printf '%s' "$queue" | jq -r '.skip_vote.enabled')" = true ] || { echo "skip vote state missing from queue snapshot" >&2; exit 1; }
[ "$(printf '%s' "$queue" | jq -r '(.items | length) == 0 or .skip_vote.required >= 1')" = true ] || { echo "skip vote threshold invalid" >&2; exit 1; }
echo "voting, account dashboard, and YouTube search API: passed"

#!/bin/sh
set -eu

TEST_BASE_URL=${TEST_BASE_URL:-http://127.0.0.1:8080}
LIB_TMP=$(mktemp -d "${TMPDIR:-/tmp}/raspi-library-test.XXXXXX")
trap 'rm -rf "$LIB_TMP"' EXIT INT TERM
COOKIE_JAR=$LIB_TMP/cookies

stations=$(curl --silent --fail "$TEST_BASE_URL/api/v1/stations?q=KFJC")
[ "$(printf '%s' "$stations" | jq -r '.stations[0].id')" = household-kfjc ] || { echo "default KFJC station missing" >&2; exit 1; }

signup=$(curl --silent --fail --cookie-jar "$COOKIE_JAR" --request POST --header 'Content-Type: application/json' \
    --data '{"username":"library_tester","password":"library-password","password_confirmation":"library-password"}' "$TEST_BASE_URL/api/v1/auth/signup")
csrf=$(printf '%s' "$signup" | jq -r .csrf_token)

curl --silent --fail --cookie "$COOKIE_JAR" --request PUT --header "X-CSRF-Token: $csrf" --header 'Content-Type: application/json' \
    --data '{"favorite":true}' "$TEST_BASE_URL/api/v1/stations/household-kfjc/favorite" >/dev/null
favorites=$(curl --silent --fail --cookie "$COOKIE_JAR" "$TEST_BASE_URL/api/v1/favorites")
[ "$(printf '%s' "$favorites" | jq -r '.stations[0].id')" = household-kfjc ] || { echo "favorite station missing" >&2; exit 1; }

station=$(curl --silent --fail --cookie "$COOKIE_JAR" --request POST --header "X-CSRF-Token: $csrf" --header 'Content-Type: application/json' \
    --data '{"name":"Regression Radio","stream_url":"https://example.com/regression-radio.mp3"}' "$TEST_BASE_URL/api/v1/stations")
[ "$(printf '%s' "$station" | jq -r .name)" = 'Regression Radio' ] || { echo "personal station creation failed" >&2; exit 1; }

playlist=$(curl --silent --fail --cookie "$COOKIE_JAR" --request POST --header "X-CSRF-Token: $csrf" --header 'Content-Type: application/json' \
    --data '{"name":"Regression Playlist"}' "$TEST_BASE_URL/api/v1/playlists")
playlist_id=$(printf '%s' "$playlist" | jq -r .id)
curl --silent --fail --cookie "$COOKIE_JAR" --request POST --header "X-CSRF-Token: $csrf" --header 'Content-Type: application/json' \
    --data '{"name":"Regression Radio","source_url":"https://example.com/regression-radio.mp3"}' "$TEST_BASE_URL/api/v1/playlists/$playlist_id/items" >/dev/null
search=$(curl --silent --fail --cookie "$COOKIE_JAR" "$TEST_BASE_URL/api/v1/library/search?q=Regression")
[ "$(printf '%s' "$search" | jq -r '.playlists | length')" = 1 ] || { echo "playlist search failed" >&2; exit 1; }

queue=$(curl --silent --fail --request POST --header 'Content-Type: application/json' --data '{"url":"https://example.com/history-test.mp3","title":"Regression Artist - Liked Recommendation"}' "$TEST_BASE_URL/api/v1/queue/items")
revision=$(printf '%s' "$queue" | jq -r .revision)
queue_item_id=$(printf '%s' "$queue" | jq -r '.items[0].id')
curl --silent --fail --cookie "$COOKIE_JAR" --request PUT --header "X-CSRF-Token: $csrf" --header 'Content-Type: application/json' \
    --data '{}' "$TEST_BASE_URL/api/v1/queue/items/$queue_item_id/like" >/dev/null
account=$(curl --silent --fail --cookie "$COOKIE_JAR" "$TEST_BASE_URL/api/v1/account")
[ "$(printf '%s' "$account" | jq -r '.likes[0].title')" = 'Regression Artist - Liked Recommendation' ] || { echo "liked track missing from account" >&2; exit 1; }
curl --silent --fail --request POST "$TEST_BASE_URL/api/v1/playback/resume" >/dev/null
attempt=0
until [ "$(curl --silent --fail "$TEST_BASE_URL/api/v1/history?q=history-test" | jq -r '.history | length')" -ge 1 ]; do
    attempt=$((attempt + 1)); [ "$attempt" -lt 30 ] || { echo "playback history was not recorded" >&2; exit 1; }; sleep 0.1
done
latest=$(curl --silent --fail "$TEST_BASE_URL/api/v1/queue")
revision=$(printf '%s' "$latest" | jq -r .revision)
curl --silent --fail --request DELETE --header "If-Match: \"$revision\"" "$TEST_BASE_URL/api/v1/queue" >/dev/null

echo "stations, favorites, playlists, likes, search, and history API: passed"

#!/bin/sh
set -eu

TEST_BASE_URL=${TEST_BASE_URL:-http://127.0.0.1:8080}

wait_for() {
    query=$1
    expected=$2
    attempt=0
    while [ "$attempt" -lt 50 ]; do
        actual=$(curl --silent --fail "$TEST_BASE_URL/api/v1/queue" | jq -r "$query")
        [ "$actual" = "$expected" ] && return 0
        attempt=$((attempt + 1))
        sleep 0.1
    done
    echo "playback state: expected $query to be '$expected', got '$actual'" >&2
    exit 1
}

curl --silent --fail --request POST --header 'Content-Type: application/json' \
    --data '{"url":"https://example.com/playback-control-test.mp3"}' \
    "$TEST_BASE_URL/api/v1/queue/items" >/dev/null
wait_for '.items[0].status' current
wait_for .playback.status playing

curl --silent --fail --request POST "$TEST_BASE_URL/api/v1/playback/pause" >/dev/null
wait_for .playback.paused true
curl --silent --fail --request POST --header 'Content-Type: application/json' --data '{"position_seconds":15}' "$TEST_BASE_URL/api/v1/playback/seek" >/dev/null
wait_for .playback.position_seconds 15
curl --silent --fail --request PUT --header 'Content-Type: application/json' --data '{"volume":34}' "$TEST_BASE_URL/api/v1/playback/volume" >/dev/null
wait_for .playback.volume 34
curl --silent --fail --request POST "$TEST_BASE_URL/api/v1/playback/resume" >/dev/null
wait_for .playback.paused false
curl --silent --fail --request POST "$TEST_BASE_URL/api/v1/playback/stop" >/dev/null
wait_for .playback.status stopped

snapshot=$(curl --silent --fail "$TEST_BASE_URL/api/v1/queue")
revision=$(printf '%s' "$snapshot" | jq -r .revision)
curl --silent --fail --request DELETE --header "If-Match: \"$revision\"" "$TEST_BASE_URL/api/v1/queue" >/dev/null

echo "playback control API: passed"

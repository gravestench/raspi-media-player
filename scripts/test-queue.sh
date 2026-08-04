#!/bin/sh
set -eu

TEST_BASE_URL=${TEST_BASE_URL:-http://127.0.0.1:8080}

response=$(curl --silent --fail --request POST \
    --header 'Content-Type: application/json' \
    --data '{"url":"https://netcast.kfjc.org/kfjc-128k-mp3","display_name":"API test"}' \
    "$TEST_BASE_URL/api/v1/queue/items")
revision=$(printf '%s' "$response" | jq -r .revision)
item_id=$(printf '%s' "$response" | jq -r '.items[0].id')
[ "$revision" = 1 ] || { echo "queue add: expected revision 1, got $revision" >&2; exit 1; }
[ -n "$item_id" ] || { echo "queue add: missing item ID" >&2; exit 1; }

status=$(curl --silent --output /dev/null --write-out '%{http_code}' --request DELETE \
    --header 'If-Match: "0"' "$TEST_BASE_URL/api/v1/queue/items/$item_id")
[ "$status" = 409 ] || { echo "stale revision: expected 409, got $status" >&2; exit 1; }

response=$(curl --silent --fail --request POST --header 'If-Match: "1"' \
    "$TEST_BASE_URL/api/v1/queue/skip")
[ "$(printf '%s' "$response" | jq -r '.items | length')" = 0 ] || { echo "skip did not empty queue" >&2; exit 1; }

echo "anonymous queue API: passed"

#!/bin/sh
set -eu

TEST_BASE_URL=${TEST_BASE_URL:-http://127.0.0.1:8080}
VALIDATION_TMP=$(mktemp -d "${TMPDIR:-/tmp}/raspi-validation-test.XXXXXX")
trap 'rm -rf "$VALIDATION_TMP"' EXIT INT TERM

invalid=$(curl --silent --output "$VALIDATION_TMP/invalid.json" --write-out '%{http_code}' --request POST --header 'Content-Type: application/json' --data '{"url":"file:///etc/passwd"}' "$TEST_BASE_URL/api/v1/queue/items")
[ "$invalid" = 422 ] || { echo "invalid URL: expected 422, got $invalid" >&2; exit 1; }
[ "$(jq -r .error.code "$VALIDATION_TMP/invalid.json")" = invalid_url ] || { echo "invalid URL: unstable error code" >&2; exit 1; }

youtube=$(curl --silent --output "$VALIDATION_TMP/youtube.json" --write-out '%{http_code}' --request POST --header 'Content-Type: application/json' --data '{"url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"}' "$TEST_BASE_URL/api/v1/queue/items")
[ "$youtube" = 201 ] || { echo "YouTube queue input: expected 201, got $youtube" >&2; exit 1; }
[ "$(jq -r '.items[0].source.kind' "$VALIDATION_TMP/youtube.json")" = youtube ] || { echo "YouTube queue input: expected youtube source kind" >&2; exit 1; }
revision=$(jq -r .revision "$VALIDATION_TMP/youtube.json")
curl --silent --fail --request DELETE --header "If-Match: \"$revision\"" "$TEST_BASE_URL/api/v1/queue" >/dev/null

malformed=$(curl --silent --output /dev/null --write-out '%{http_code}' --request POST --header 'Content-Type: application/json' --data '{' "$TEST_BASE_URL/api/v1/queue/items")
[ "$malformed" = 400 ] || { echo "malformed JSON: expected 400, got $malformed" >&2; exit 1; }

oversized_payload=$(awk 'BEGIN { printf "{\"url\":\"https://example.com/"; for (i=0;i<70000;i++) printf "x"; printf "\"}" }')
oversized=$(curl --silent --output /dev/null --write-out '%{http_code}' --request POST --header 'Content-Type: application/json' --data "$oversized_payload" "$TEST_BASE_URL/api/v1/queue/items")
[ "$oversized" = 400 ] || { echo "oversized request: expected 400, got $oversized" >&2; exit 1; }

echo "malformed, oversized, and invalid request handling: passed"

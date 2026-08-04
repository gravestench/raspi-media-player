#!/bin/sh
set -eu
TEST_BASE_URL=${TEST_BASE_URL:-http://127.0.0.1:8080}
body=$(curl --silent --fail --get --data-urlencode 'title=Björk - Jóga (Official Music Video)' "$TEST_BASE_URL/api/v1/enrichment")
[ "$(printf '%s' "$body" | jq -r .enrichment.status)" = disabled ] || { echo "disabled enrichment: expected disabled state" >&2; exit 1; }
[ "$(printf '%s' "$body" | jq -r .enrichment.hint.artist)" = Björk ] || { echo "enrichment parser: artist mismatch" >&2; exit 1; }
[ "$(printf '%s' "$body" | jq -r .enrichment.hint.title)" = Jóga ] || { echo "enrichment parser: title mismatch" >&2; exit 1; }
missing=$(curl --silent --output /dev/null --write-out '%{http_code}' "$TEST_BASE_URL/api/v1/enrichment")
[ "$missing" = 400 ] || { echo "enrichment missing title: expected 400, got $missing" >&2; exit 1; }
image=$(curl --silent --output /dev/null --write-out '%{http_code}' "$TEST_BASE_URL/api/v1/enrichment/images/not-present")
[ "$image" = 404 ] || { echo "unknown enrichment image: expected 404, got $image" >&2; exit 1; }
echo "artist enrichment API: passed"

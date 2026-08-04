#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TEST_TMP=$(mktemp -d "${TMPDIR:-/tmp}/raspi-media-player-test.XXXXXX")
TEST_PORT=${TEST_PORT:-18080}
TEST_BASE_URL="http://127.0.0.1:${TEST_PORT}"
SERVER_PID=""

cleanup() {
    if [ -n "$SERVER_PID" ]; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    rm -rf "$TEST_TMP"
}
trap cleanup EXIT INT TERM

cd "$PROJECT_ROOT"
go build -o "$TEST_TMP/raspi-media-player" ./cmd/raspi-media-player
export TEST_SERVER_BINARY="$TEST_TMP/raspi-media-player"
"$TEST_TMP/raspi-media-player" \
    -addr "127.0.0.1:${TEST_PORT}" \
    -db "$TEST_TMP/test.sqlite" \
    -player-enabled=true \
    -player-backend=fake \
	-metadata-enabled=false \
    -log-format json >"$TEST_TMP/server.log" 2>&1 &
SERVER_PID=$!

attempt=0
until curl --silent --fail "$TEST_BASE_URL/api/v1/health/ready" >/dev/null; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 50 ]; then
        echo "server did not become ready" >&2
        sed -n '1,160p' "$TEST_TMP/server.log" >&2
        exit 1
    fi
    sleep 0.1
done

TEST_BASE_URL="$TEST_BASE_URL" "$PROJECT_ROOT/scripts/test-health.sh"
TEST_BASE_URL="$TEST_BASE_URL" "$PROJECT_ROOT/scripts/test-queue.sh"
TEST_BASE_URL="$TEST_BASE_URL" "$PROJECT_ROOT/scripts/test-auth.sh"
TEST_BASE_URL="$TEST_BASE_URL" "$PROJECT_ROOT/scripts/test-playback.sh"
TEST_BASE_URL="$TEST_BASE_URL" "$PROJECT_ROOT/scripts/test-library.sh"
TEST_BASE_URL="$TEST_BASE_URL" "$PROJECT_ROOT/scripts/test-enrichment.sh"
TEST_BASE_URL="$TEST_BASE_URL" "$PROJECT_ROOT/scripts/test-validation.sh"
TEST_BASE_URL="$TEST_BASE_URL" TEST_SERVER_BINARY="$TEST_SERVER_BINARY" "$PROJECT_ROOT/scripts/test-access-modes.sh"
TEST_SERVER_BINARY="$TEST_SERVER_BINARY" "$PROJECT_ROOT/scripts/test-persistence.sh"
echo "All API regression tests passed."

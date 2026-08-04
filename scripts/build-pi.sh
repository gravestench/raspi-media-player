#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUTPUT=${OUTPUT:-$PROJECT_ROOT/dist/raspi-media-player-linux-arm64}
VERSION=${VERSION:-dev-pi}
COMMIT=${COMMIT:-$(git -C "$PROJECT_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)}
BUILT_AT=${BUILT_AT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}

mkdir -p "$(dirname -- "$OUTPUT")"
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
    -ldflags "-X main.version=$VERSION -X main.commit=$COMMIT -X main.builtAt=$BUILT_AT" \
    -o "$OUTPUT" "$PROJECT_ROOT/cmd/raspi-media-player"

echo "Built $OUTPUT"

#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VERSION=${1:-}

case "$VERSION" in
    ''|*[!A-Za-z0-9._-]*)
        echo "Usage: build-release.sh VERSION" >&2
        echo "VERSION may contain letters, numbers, dots, underscores, and hyphens." >&2
        exit 2
        ;;
esac

COMMIT=${COMMIT:-$(git -C "$PROJECT_ROOT" rev-parse --short=12 HEAD)}
BUILT_AT=${BUILT_AT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}
ARCHIVE_NAME=raspi-media-player_${VERSION}_linux_arm64
OUTPUT_DIR=${OUTPUT_DIR:-$PROJECT_ROOT/dist}
STAGE_ROOT=$(mktemp -d)
trap 'rm -rf "$STAGE_ROOT"' EXIT HUP INT TERM
BUNDLE=$STAGE_ROOT/$ARCHIVE_NAME

mkdir -p "$BUNDLE" "$OUTPUT_DIR"
OUTPUT="$BUNDLE/raspi-media-player-linux-arm64" \
    VERSION="$VERSION" COMMIT="$COMMIT" BUILT_AT="$BUILT_AT" \
    "$PROJECT_ROOT/scripts/build-pi.sh"

cp "$PROJECT_ROOT/deploy/raspi-media-player.init" "$BUNDLE/"
cp "$PROJECT_ROOT/deploy/raspi-media-player.default" "$BUNDLE/"
cp "$PROJECT_ROOT/scripts/install-dependencies.sh" "$BUNDLE/"
cp "$PROJECT_ROOT/scripts/service-manager.sh" "$BUNDLE/"
cp "$PROJECT_ROOT/scripts/diagnose.sh" "$BUNDLE/"
cp "$PROJECT_ROOT/scripts/soak.sh" "$BUNDLE/"
cp "$PROJECT_ROOT/README.md" "$BUNDLE/"
cp "$PROJECT_ROOT/RELEASE_NOTES.md" "$BUNDLE/"

tar -C "$STAGE_ROOT" -czf "$OUTPUT_DIR/$ARCHIVE_NAME.tar.gz" "$ARCHIVE_NAME"
echo "Built $OUTPUT_DIR/$ARCHIVE_NAME.tar.gz"

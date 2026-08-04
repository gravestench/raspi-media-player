#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TARGET=${TARGET:-dknuth@192.168.1.25}
REMOTE_STAGE=/tmp/raspi-media-player-deploy
BINARY=$PROJECT_ROOT/dist/raspi-media-player-linux-arm64

"$PROJECT_ROOT/scripts/build-pi.sh"

ssh "$TARGET" "mkdir -p '$REMOTE_STAGE'"
scp "$BINARY" \
    "$PROJECT_ROOT/deploy/raspi-media-player.init" \
    "$PROJECT_ROOT/deploy/raspi-media-player.default" \
    "$PROJECT_ROOT/scripts/service-manager.sh" \
    "$TARGET:$REMOTE_STAGE/"

ssh -t "$TARGET" "\
set -eu; \
sudo sh '$REMOTE_STAGE/service-manager.sh' upgrade '$REMOTE_STAGE'; \
sudo service raspi-media-player status; \
rm -rf '$REMOTE_STAGE'"

echo "Deployed to $TARGET"

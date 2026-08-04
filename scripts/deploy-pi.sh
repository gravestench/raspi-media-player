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
    "$TARGET:$REMOTE_STAGE/"

ssh -t "$TARGET" "\
set -eu; \
sudo service raspi-media-player stop 2>/dev/null || true; \
sudo install -m 0755 '$REMOTE_STAGE/raspi-media-player-linux-arm64' /usr/local/bin/raspi-media-player; \
sudo install -m 0755 '$REMOTE_STAGE/raspi-media-player.init' /etc/init.d/raspi-media-player; \
sudo install -m 0644 '$REMOTE_STAGE/raspi-media-player.default' /etc/default/raspi-media-player; \
id raspi-media-player >/dev/null 2>&1 || sudo useradd --system --home-dir /var/lib/raspi-media-player --create-home --shell /usr/sbin/nologin --groups audio raspi-media-player; \
sudo install -d -o raspi-media-player -g raspi-media-player -m 0750 /var/lib/raspi-media-player; \
sudo touch /var/log/raspi-media-player.log; \
sudo chown raspi-media-player:raspi-media-player /var/log/raspi-media-player.log; \
sudo chmod 0640 /var/log/raspi-media-player.log; \
sudo update-rc.d raspi-media-player defaults; \
sudo service raspi-media-player start; \
sudo service raspi-media-player status; \
rm -rf '$REMOTE_STAGE'"

echo "Deployed to $TARGET"

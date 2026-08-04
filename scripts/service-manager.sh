#!/bin/sh
set -eu

PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
NAME=raspi-media-player
SERVICE_USER=raspi-media-player
BINARY_PATH=/usr/local/bin/$NAME
INIT_PATH=/etc/init.d/$NAME
DEFAULTS_PATH=/etc/default/$NAME
MANAGER_PATH=/usr/local/sbin/raspi-media-player-service
DIAGNOSE_PATH=/usr/local/sbin/raspi-media-player-diagnose
SOAK_PATH=/usr/local/sbin/raspi-media-player-soak
DATA_DIR=/var/lib/$NAME
LOG_PATH=/var/log/$NAME.log

usage() {
    echo "Usage: service-manager.sh {install|upgrade|config-check|uninstall} [asset-directory]" >&2
}

if [ "$(id -u)" -ne 0 ]; then
    echo "Run service-manager.sh as root (normally with sudo)." >&2
    exit 1
fi

action=${1:-}
asset_dir=${2:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}
binary_asset=$asset_dir/raspi-media-player-linux-arm64
init_asset=$asset_dir/raspi-media-player.init
defaults_asset=$asset_dir/raspi-media-player.default
manager_asset=$asset_dir/service-manager.sh
diagnose_asset=$asset_dir/diagnose.sh
soak_asset=$asset_dir/soak.sh

install_assets() {
    [ -f "$binary_asset" ] || { echo "Missing binary: $binary_asset" >&2; exit 1; }
    [ -f "$init_asset" ] || { echo "Missing init script: $init_asset" >&2; exit 1; }
    [ -f "$defaults_asset" ] || { echo "Missing defaults file: $defaults_asset" >&2; exit 1; }
	[ -f "$manager_asset" ] || { echo "Missing service manager: $manager_asset" >&2; exit 1; }
	[ -f "$diagnose_asset" ] || { echo "Missing diagnostics script: $diagnose_asset" >&2; exit 1; }
	[ -f "$soak_asset" ] || { echo "Missing soak script: $soak_asset" >&2; exit 1; }

    if ! id "$SERVICE_USER" >/dev/null 2>&1; then
        useradd --system --home-dir "$DATA_DIR" --create-home --shell /usr/sbin/nologin --groups audio "$SERVICE_USER"
    fi
    install -d -o "$SERVICE_USER" -g "$SERVICE_USER" -m 0750 "$DATA_DIR"
    touch "$LOG_PATH"
    chown "$SERVICE_USER:$SERVICE_USER" "$LOG_PATH"
    chmod 0640 "$LOG_PATH"
    install -m 0755 "$binary_asset" "$BINARY_PATH"
    install -m 0755 "$init_asset" "$INIT_PATH"
	install -m 0755 "$manager_asset" "$MANAGER_PATH"
	install -m 0755 "$diagnose_asset" "$DIAGNOSE_PATH"
	install -m 0755 "$soak_asset" "$SOAK_PATH"
    install -d -m 0755 /usr/share/$NAME
    install -m 0644 "$defaults_asset" /usr/share/$NAME/raspi-media-player.default
    if [ ! -e "$DEFAULTS_PATH" ]; then
		install -m 0640 "$defaults_asset" "$DEFAULTS_PATH"
        echo "Installed initial configuration at $DEFAULTS_PATH"
    else
        echo "Preserved existing configuration at $DEFAULTS_PATH"
    fi
	chown root:root "$DEFAULTS_PATH"
	chmod 0640 "$DEFAULTS_PATH"
    update-rc.d "$NAME" defaults
}

config_check() {
    [ -x "$BINARY_PATH" ] || { echo "$BINARY_PATH is missing or not executable" >&2; exit 1; }
    [ -r "$DEFAULTS_PATH" ] || { echo "$DEFAULTS_PATH is missing or unreadable" >&2; exit 1; }
    # The file is administrator-controlled shell syntax, as expected by init.d.
    # shellcheck disable=SC1090
    . "$DEFAULTS_PATH"
    case "${RASPI_MEDIA_PLAYER_ACCESS_MODE:-open}" in open|accounts_optional|accounts_required) ;; *) echo "Invalid access mode" >&2; exit 1 ;; esac
    case "${RASPI_MEDIA_PLAYER_LOG_FORMAT:-json}" in json|text) ;; *) echo "Invalid log format" >&2; exit 1 ;; esac
    case "${RASPI_MEDIA_PLAYER_LOG_LEVEL:-info}" in debug|info|warn|error) ;; *) echo "Invalid log level" >&2; exit 1 ;; esac
    case "${RASPI_MEDIA_PLAYER_PLAYER_BACKEND:-mpv}" in mpv|fake) ;; *) echo "Invalid player backend" >&2; exit 1 ;; esac
    case "${RASPI_MEDIA_PLAYER_ADDR:-}" in *:*) ;; *) echo "Bind address must include a port" >&2; exit 1 ;; esac
    for value in "${RASPI_MEDIA_PLAYER_QUEUE_LIMIT:-}" "${RASPI_MEDIA_PLAYER_QUEUE_RATE:-}" "${RASPI_MEDIA_PLAYER_AUTH_RATE:-}" "${RASPI_MEDIA_PLAYER_SESSION_DAYS:-}"; do
        case "$value" in ''|*[!0-9]*|0) echo "Limits and lifetimes must be positive integers" >&2; exit 1 ;; esac
    done
    db_path=${RASPI_MEDIA_PLAYER_DB:-$DATA_DIR/player.sqlite}
    [ -d "$(dirname -- "$db_path")" ] || { echo "Database directory does not exist: $(dirname -- "$db_path")" >&2; exit 1; }
    if [ "${RASPI_MEDIA_PLAYER_PLAYER_ENABLED:-true}" = true ] && [ "${RASPI_MEDIA_PLAYER_PLAYER_BACKEND:-mpv}" = mpv ]; then
        [ -x "${RASPI_MEDIA_PLAYER_MPV_BINARY:-/usr/bin/mpv}" ] || { echo "Configured mpv binary is not executable" >&2; exit 1; }
    fi
    echo "Configuration is valid."
}

case "$action" in
    install)
        install_assets
        config_check
        service "$NAME" start
        ;;
    upgrade)
        service "$NAME" stop 2>/dev/null || true
        install_assets
        config_check
        service "$NAME" start
        ;;
    config-check)
        config_check
        ;;
    uninstall)
        service "$NAME" stop 2>/dev/null || true
        update-rc.d -f "$NAME" remove
		rm -f "$INIT_PATH" "$BINARY_PATH" "$MANAGER_PATH" "$DIAGNOSE_PATH" "$SOAK_PATH"
        echo "Removed the service and binary. Preserved $DEFAULTS_PATH, $DATA_DIR, $LOG_PATH, and user $SERVICE_USER."
        ;;
    *) usage; exit 2 ;;
esac

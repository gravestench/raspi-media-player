#!/bin/sh
set -eu

# Non-login SSH sessions do not consistently include administrative command
# directories, even when their tools are installed.
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export PATH

usage() {
    cat <<'EOF'
Usage: install-dependencies.sh [--check]

Install Raspberry Pi runtime and test dependencies on Debian/Raspberry Pi OS.
The script is idempotent and uses sudo automatically when not run as root.

  --check   Report missing packages and commands without changing the system.
EOF
}

MODE=install
case "${1:-}" in
    "") ;;
    --check) MODE=check ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
esac

if [ ! -r /etc/os-release ]; then
    echo "Cannot identify this operating system: /etc/os-release is missing." >&2
    exit 1
fi

# shellcheck disable=SC1091
. /etc/os-release
case "${ID:-}" in
    debian|raspbian) ;;
    *)
        echo "Unsupported operating system '${ID:-unknown}'; Debian or Raspberry Pi OS is required." >&2
        exit 1
        ;;
esac

if ! command -v dpkg-query >/dev/null 2>&1 || ! command -v apt-get >/dev/null 2>&1; then
    echo "This system does not provide the required dpkg/apt tools." >&2
    exit 1
fi

PACKAGES="
alsa-utils
ca-certificates
curl
ffmpeg
jq
lsb-base
mpv
passwd
procps
sqlite3
"

missing_packages=""
for package in $PACKAGES; do
    if ! dpkg-query -W -f='${Status}' "$package" 2>/dev/null | grep -q '^install ok installed$'; then
        missing_packages="$missing_packages $package"
    fi
done

if [ "$MODE" = check ]; then
    if [ -n "$missing_packages" ]; then
        echo "Missing packages:$missing_packages" >&2
        exit 1
    fi

    missing_commands=""
    for command_name in aplay curl ffmpeg jq mpv sqlite3 start-stop-daemon; do
        if ! command -v "$command_name" >/dev/null 2>&1; then
            missing_commands="$missing_commands $command_name"
        fi
    done
    if [ -n "$missing_commands" ]; then
        echo "Packages are installed, but commands are unavailable:$missing_commands" >&2
        exit 1
    fi

    echo "All Raspberry Pi dependencies are installed."
    exit 0
fi

if [ -z "$missing_packages" ]; then
    echo "All Raspberry Pi dependencies are already installed."
    exit 0
fi

if [ "$(id -u)" -eq 0 ]; then
    SUDO=""
elif command -v sudo >/dev/null 2>&1; then
    SUDO=sudo
else
    echo "Run this script as root or install sudo." >&2
    exit 1
fi

echo "Installing Raspberry Pi dependencies:$missing_packages"
$SUDO apt-get update
# SC2086 is intentional: apt-get requires the package list as separate words.
# shellcheck disable=SC2086
$SUDO apt-get install -y --no-install-recommends $missing_packages

echo "Dependency installation complete."
exec "$0" --check

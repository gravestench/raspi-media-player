#!/bin/sh
set -eu

BASE_URL=${BASE_URL:-http://127.0.0.1:8080}
SERVICE_NAME=${SERVICE_NAME:-raspi-media-player}

echo "== system =="
uname -a
if [ -r /etc/os-release ]; then sed -n '1,12p' /etc/os-release; fi
echo "== binary =="
if command -v raspi-media-player >/dev/null 2>&1; then command -v raspi-media-player; ls -l "$(command -v raspi-media-player)"; fi
echo "== service =="
service "$SERVICE_NAME" status 2>&1 || true
echo "== health and version =="
curl --silent --show-error --max-time 3 "$BASE_URL/api/v1/health/ready" || true
echo
curl --silent --show-error --max-time 3 "$BASE_URL/api/v1/version" || true
echo
echo "== queue summary =="
curl --silent --show-error --max-time 3 "$BASE_URL/api/v1/queue" | jq '{revision,item_count:(.items|length),playback:{status:.playback.status,paused:.playback.paused,buffering:.playback.buffering,volume:.playback.volume,error:.playback.error}}' || true
echo "== processes =="
ps -eo pid,ppid,user,%cpu,%mem,rss,etime,comm,args | awk 'NR == 1 || /raspi-media-player|[m]pv/'
echo "== recent redacted application logs =="
if [ -r /var/log/raspi-media-player.log ]; then tail -n 100 /var/log/raspi-media-player.log | sed -E 's#(https?://)[^/@[:space:]]+:[^/@[:space:]]+@#\1[redacted]@#g'; else echo "log unavailable to current user"; fi

#!/bin/sh
set -eu

BASE_URL=${BASE_URL:-http://127.0.0.1:8080}
DURATION_SECONDS=${DURATION_SECONDS:-3600}
INTERVAL_SECONDS=${INTERVAL_SECONDS:-10}

case "$DURATION_SECONDS:$INTERVAL_SECONDS" in *[!0-9:]*|0:*|*:0) echo "durations must be positive integers" >&2; exit 2 ;; esac
start=$(date +%s)
deadline=$((start + DURATION_SECONDS))
samples=0
max_rss=0
max_cpu=0
last_revision=""

while [ "$(date +%s)" -lt "$deadline" ]; do
    snapshot=$(curl --silent --fail --max-time 5 "$BASE_URL/api/v1/queue") || { echo "soak: queue API unavailable" >&2; exit 1; }
    printf '%s' "$snapshot" | jq -e '.revision >= 0 and (.items | type == "array") and (.playback.status | type == "string")' >/dev/null || { echo "soak: invalid queue snapshot" >&2; exit 1; }
    revision=$(printf '%s' "$snapshot" | jq -r .revision)
    case "$last_revision:$revision" in
        :*) ;;
        *) [ "$revision" -ge "$last_revision" ] || { echo "soak: queue revision moved backwards" >&2; exit 1; } ;;
    esac
    last_revision=$revision
    pid=$(pgrep -fo '^/usr/local/bin/raspi-media-player |raspi-media-player.*-addr' || true)
    if [ -n "$pid" ]; then
        values=$(ps -p "$pid" -o rss=,%cpu= | awk '{print $1, $2}')
        rss=$(printf '%s' "$values" | awk '{print $1+0}')
        cpu=$(printf '%s' "$values" | awk '{print $2+0}')
        [ "$rss" -le "$max_rss" ] || max_rss=$rss
        max_cpu=$(awk -v a="$max_cpu" -v b="$cpu" 'BEGIN { print (a>b?a:b) }')
    fi
    samples=$((samples + 1))
    sleep "$INTERVAL_SECONDS"
done

echo "soak passed: duration=${DURATION_SECONDS}s samples=$samples max_rss_kib=$max_rss max_cpu_percent=$max_cpu final_revision=$last_revision"

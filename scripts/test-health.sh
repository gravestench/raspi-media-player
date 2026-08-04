#!/bin/sh
set -eu

TEST_BASE_URL=${TEST_BASE_URL:-http://127.0.0.1:8080}

assert_json_value() {
    path=$1
    query=$2
    expected=$3
    actual=$(curl --silent --fail "$TEST_BASE_URL$path" | jq -r "$query")
    if [ "$actual" != "$expected" ]; then
        echo "$path: expected $query to be '$expected', got '$actual'" >&2
        exit 1
    fi
}

assert_json_value /api/v1/health/live .status ok
assert_json_value /api/v1/health/ready .status ready
assert_json_value /api/v1/version .version dev

request_id=$(curl --silent --dump-header - --output /dev/null \
    -H 'X-Request-ID: regression-request' \
    "$TEST_BASE_URL/api/v1/health/live" | tr -d '\r' | awk 'tolower($1) == "x-request-id:" { print $2 }')
if [ "$request_id" != "regression-request" ]; then
    echo "request ID was not echoed" >&2
    exit 1
fi

echo "health API: passed"

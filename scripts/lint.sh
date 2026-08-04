#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
REQUIRED_VERSION=$(sed -n '1p' "$PROJECT_ROOT/.golangci-lint-version")

if [ -x "$PROJECT_ROOT/bin/golangci-lint" ]; then
    LINTER=$PROJECT_ROOT/bin/golangci-lint
elif command -v golangci-lint >/dev/null 2>&1; then
    LINTER=$(command -v golangci-lint)
else
    echo "golangci-lint $REQUIRED_VERSION is required. See docs/development.md." >&2
    exit 1
fi

case "$($LINTER version 2>/dev/null)" in
    *"${REQUIRED_VERSION#v}"*) ;;
    *) echo "golangci-lint $REQUIRED_VERSION is required; found: $($LINTER version 2>/dev/null)" >&2; exit 1 ;;
esac

cd "$PROJECT_ROOT"
exec "$LINTER" run ./...

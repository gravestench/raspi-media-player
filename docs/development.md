# Development guide

## Toolchain

- Go 1.24 or newer
- `golangci-lint` at the version in `.golangci-lint-version`
- `curl` and `jq` for shell API tests
- SQLite CLI for operational inspection
- optional `mpv`, ALSA, and `yt-dlp` for real playback/search testing

Install platform dependencies appropriate to the development OS. The Pi helper
is Debian-specific and should not be run on macOS.

## Local server

```sh
go run ./cmd/raspi-media-player \
  -addr 127.0.0.1:8080 \
  -db /tmp/house-jukebox-dev.sqlite \
  -player-enabled=false \
  -metadata-enabled=false
```

Use a disposable database when testing the setup wizard. Set
`-setup-required=false` only for API-focused development that intentionally
skips first-run setup.

## Repository layout

| Path | Responsibility |
| --- | --- |
| `cmd/raspi-media-player` | process composition and flags |
| `internal/app` | HTTP API, middleware, embedded SPA |
| `internal/auth` | passwords and sessions |
| `internal/queue` | queue and playback snapshot persistence |
| `internal/playback` / `internal/player` | controller and mpv/fake backends |
| `internal/source` / `internal/youtube` | URL resolution and search |
| `internal/enrichment` | metadata providers/cache/images |
| `internal/library` | stations, favorites, playlists, history |
| `internal/autoqueue` | recommendation refill engine |
| `internal/database/migrations` | ordered embedded SQLite migrations |
| `deploy` / `scripts` | init.d, builds, deployment, diagnostics, tests |

## Verification

```sh
make lint
go test -race ./...
./scripts/test-all.sh
```

Install the pinned linter into the repository-local `bin/` directory with the
official installer:

```sh
curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b ./bin "$(cat .golangci-lint-version)"
```

The lint wrapper rejects other versions so local and CI results remain aligned.
`make check` runs linting and unit tests; `make test-api` runs the API regression
suite. GitHub CI repeats linting, formatting, race tests, and shell regression
tests on pushes and pull requests.

When frontend behavior changes, also run `node --check
internal/app/web/app.js` and execute the [browser smoke checklist](browser-smoke.md)
at desktop and phone widths.

## Database changes

Add the next zero-padded SQL file to `internal/database/migrations`. Migrations
must be forward-only, idempotent through `schema_migrations`, and safe inside one
transaction. Update the migration-count test and add feature regression tests.
Never edit a migration already shipped in a release.

## Logging changes

Use structured attributes and stable event messages. Never log passwords,
session/CSRF tokens, cookies, provider keys, or request bodies. Propagate request
contexts so request IDs remain correlated.

## Release flow

Pushes to `main` run CI and replace the rolling `edge` prerelease after tests
pass. Pushing a semantic tag such as `v0.2.0` builds a stable GitHub Release.
See [releases.md](releases.md).

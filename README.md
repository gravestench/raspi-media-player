# Raspberry Pi Household Jukebox

A self-hosted household jukebox written in Go. The Raspberry Pi owns playback;
phones and computers on the local network provide the shared remote control.

The project is under active development. See [milestones.md](milestones.md) for
the roadmap and current status.

## Requirements

- Go 1.24 or newer for development
- `curl` and `jq` for API regression tests
- Raspberry Pi playback uses `mpv`; YouTube URL support also uses the external
  `yt-dlp` executable installed by the dependency script

## Run locally

```sh
go run ./cmd/raspi-media-player -addr 127.0.0.1:8080 -db ./player.sqlite
```

Open <http://127.0.0.1:8080>. Configuration can also be supplied using
`RASPI_MEDIA_PLAYER_ADDR`, `RASPI_MEDIA_PLAYER_DB`,
`RASPI_MEDIA_PLAYER_LOG_FORMAT`, and `RASPI_MEDIA_PLAYER_LOG_LEVEL`.

## Verify

```sh
make check
make test-api
```

`scripts/test-all.sh` builds and launches an isolated server with a temporary
SQLite database, runs all API regression scripts, and cleans up afterward.
See [docs/testing.md](docs/testing.md) for coverage, dependencies, ports, and CI.
Operational diagnosis and common failures are covered in
[docs/troubleshooting.md](docs/troubleshooting.md). Release scope and known
limitations are in [RELEASE_NOTES.md](RELEASE_NOTES.md).

## API conventions

All APIs live under `/api/v1`. Successful responses and errors are JSON. Errors
have this shape:

```json
{"error":{"code":"stable_error_code","message":"human-readable message"}}
```

Every response includes `X-Request-ID`. A caller-supplied request ID is echoed
when valid; otherwise the service creates one. HTTP request completion is logged
as a structured event containing the request ID, method, path, status, duration,
response size, and remote address.

See [docs/logging.md](docs/logging.md) for the complete logging contract and
daemon log location.

The anonymous shared queue API is documented in
[docs/queue-api.md](docs/queue-api.md).

Optional local accounts, access modes, sessions, and CSRF behavior are documented
in [docs/authentication.md](docs/authentication.md).

Player configuration, queue advancement, and playback controls are documented in
[docs/playback.md](docs/playback.md).

Household stations, personal favorites and playlists, search, and playback
history are documented in [docs/library.md](docs/library.md).

The pluggable source boundary and the current YouTube feasibility decision are
documented in [docs/sources.md](docs/sources.md).

## Daemon deployment

The init.d service assets and Raspberry Pi deployment helper live in `deploy/`
and `scripts/`. Full install, upgrade, backup, and uninstall operations are
documented in [docs/operations.md](docs/operations.md).

The current integration target is a 64-bit Debian 12 Raspberry Pi. Build and
deploy to it with:

```sh
make build-pi
make deploy-pi
```

`scripts/deploy-pi.sh` defaults to `dknuth@192.168.1.25`; override it with the
`TARGET` environment variable. SSH and sudo authentication remain interactive.
No password is read from a project file or stored by the script. The deployed
service listens on port 8080, stores its database under
`/var/lib/raspi-media-player`, and writes structured logs to
`/var/log/raspi-media-player.log`.

Before the first deployment, copy `scripts/install-dependencies.sh` to the Pi
and run it there:

```sh
./install-dependencies.sh
```

It installs the Debian packages needed for playback, init.d operation, audio
diagnostics, SQLite inspection, and API regression testing. It is safe to run
again after upgrades. Use `./install-dependencies.sh --check` to verify the
machine without changing it. Go is intentionally not installed on the Pi; the
deployment script cross-compiles a static Linux/ARM64 binary on the development
machine.

# Raspberry Pi Household Jukebox

A self-hosted household jukebox written in Go. The Raspberry Pi owns playback;
phones and computers on the local network provide the shared remote control.

The project is under active development. See [milestones.md](milestones.md) for
the roadmap and current status.

## Requirements

- Go 1.24 or newer for development
- `curl` and `jq` for API regression tests
- Raspberry Pi OS dependencies will include `mpv` when playback is implemented

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

## Daemon deployment

Initial init.d assets live in `deploy/`. Automated installation and Raspberry Pi
validation are scheduled in Milestone 7; do not treat the current script as a
finished production installer yet.

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

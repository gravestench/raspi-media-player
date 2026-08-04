# House Jukebox documentation

This index separates household tasks, administrator operations, API contracts,
and implementation details. Commands assume the repository root unless a guide
says they run on the Raspberry Pi.

## Get started

- [Installation and first run](installation.md) — prerequisites, dependency
  installation, deployment, wizard steps, upgrades, and fresh resets.
- [Household user guide](user-guide.md) — queueing, playback, accounts,
  stations, playlists, discovery, voting, and auto-queue.
- [Configuration reference](configuration.md) — environment variables,
  command-line flags, Admin UI settings, precedence, and secrets.

## Operate the service

- [Administration](administration.md) — administrator accounts, roles, provider
  keys, voting, and personal dashboards.
- [Operations](operations.md) — init.d lifecycle, deploys, backups, restores,
  and uninstall behavior.
- [Troubleshooting](troubleshooting.md) — health checks, logs, audio, YouTube,
  metadata, database, and browser problems.
- [Structured logging](logging.md) — formats, fields, request IDs, and log
  locations.
- [Releases](releases.md) — stable tags, rolling edge builds, checksums, and
  upgrades.
- [Security model](security.md) — trust boundary, authentication, CSRF,
  encryption, permissions, and network exposure.

## Features

- [Playback](playback.md)
- [Queue API](queue-api.md)
- [Sources and YouTube](sources.md)
- [Stations, favorites, playlists, and history](library.md)
- [Artist metadata and discovery](enrichment.md)
- [Auto-queue](auto-queue.md)
- [Authentication and access modes](authentication.md)

## Build and integrate

- [Complete API reference](api.md)
- [Architecture and data flow](architecture.md)
- [Development guide](development.md)
- [Regression testing](testing.md)
- [Browser smoke checklist](browser-smoke.md)

The historical delivery plan remains in [`../milestones.md`](../milestones.md).


# HTTP API reference

The browser application uses the same `/api/v1` JSON API available to household
integrations. This document is a route inventory and shared contract; focused
payload details remain in the linked feature references and regression scripts.

## Conventions

- JSON responses use `Content-Type: application/json`.
- Errors use `{"error":{"code":"stable_code","message":"description"}}`.
- Every response includes `X-Request-ID`; a valid caller-supplied value is echoed.
- Queue snapshots include a monotonic `revision` and matching `ETag`.
- Stale queue mutations return `409 revision_conflict`; missing `If-Match` where
  required returns `428`.
- Cookie-authenticated mutations require `X-CSRF-Token` copied from the
  `jukebox_csrf` cookie.
- Before first-run setup, normal API routes return `503 setup_required`.

Example:

```sh
curl --fail --show-error http://jukebox.local:8080/api/v1/queue
```

## Health, build, setup, and events

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/api/v1/health/live` | process liveness |
| GET | `/api/v1/health/ready` | database readiness |
| GET | `/api/v1/version` | version, commit, and build time |
| GET | `/api/v1/setup/status` | install state, steps, and setup links |
| POST | `/api/v1/setup/complete` | create initial admin and settings |
| GET | `/api/v1/events` | Server-Sent Event stream of queue snapshots |

Setup completion accepts `username`, `password`, `password_confirmation`,
`access_mode`, and optional `lastfm_api_key`. It returns `201` with an
authenticated session and is rejected after installation.

SSE clients receive `event: queue` messages whose JSON data has the same shape
as `GET /api/v1/queue`. Send automatic reconnection using normal `EventSource`
behavior and fetch a fresh snapshot after a prolonged disconnect.

## Queue and playback

| Method | Route | Body / purpose |
| --- | --- | --- |
| GET | `/api/v1/queue` | complete queue/playback/vote snapshot |
| POST | `/api/v1/queue/items` | `url`, optional `display_name`, optional `title` |
| DELETE | `/api/v1/queue/items/{id}` | remove item; requires revision |
| PUT | `/api/v1/queue/order` | `item_ids`; requires revision |
| DELETE | `/api/v1/queue` | clear queue; requires revision |
| POST | `/api/v1/queue/skip` | cast vote or perform allowed skip |
| DELETE | `/api/v1/queue/skip` | withdraw current listener's vote |
| POST | `/api/v1/playback/pause` | pause current item |
| POST | `/api/v1/playback/resume` | resume current item |
| POST | `/api/v1/playback/stop` | stop playback |
| POST | `/api/v1/playback/seek` | `position_seconds` |
| PUT | `/api/v1/playback/volume` | `volume` from 0 through 100 |

URLs must be absolute HTTP(S), contain no embedded credentials, and be at most
2,048 characters. YouTube URLs are accepted and resolved during playback.
Duplicate active source URLs and full queues return explicit errors. See
[queue-api.md](queue-api.md) and [playback.md](playback.md).

## Auto-queue

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/api/v1/autoqueue` | enabled, available, depth, mode, and seeds |
| PUT | `/api/v1/autoqueue` | partially update `enabled`, `mode`, `artists`, `genres` |

Modes are `active_users`, `specific_seeds`, and `related_last`. Omitted fields
are unchanged. See [auto-queue.md](auto-queue.md).

## Authentication and sessions

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/api/v1/auth/usernames/{username}` | case-insensitive availability |
| POST | `/api/v1/auth/login` | login or signal account creation required |
| POST | `/api/v1/auth/signup` | create local account and session |
| GET | `/api/v1/auth/session` | current authenticated/anonymous state |
| POST | `/api/v1/auth/logout` | revoke current session |
| GET | `/api/v1/auth/sessions` | list current user's sessions |
| DELETE | `/api/v1/auth/sessions/{id}` | revoke one owned session |

Login accepts `username` and `password`. Signup also accepts
`password_confirmation`. See [authentication.md](authentication.md).

## Stations, favorites, playlists, and history

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/api/v1/stations?q=` | household/personal stations |
| POST | `/api/v1/stations` | create personal station with `name`, `stream_url` |
| DELETE | `/api/v1/stations/{id}` | delete owned station |
| PUT | `/api/v1/stations/{id}/favorite` | set `favorite` boolean |
| GET | `/api/v1/favorites?q=` | current user's favorite stations |
| GET | `/api/v1/playlists?q=` | current user's playlists and items |
| POST | `/api/v1/playlists` | create with `name` |
| DELETE | `/api/v1/playlists/{id}` | delete owned playlist |
| POST | `/api/v1/playlists/{id}/items` | add name/kind/source URL |
| DELETE | `/api/v1/playlists/{id}/items/{itemID}` | remove owned item |
| GET | `/api/v1/history?q=` | recent household playback history |
| GET | `/api/v1/library/search?q=` | combined station/playlist/history search |
| GET | `/api/v1/account` | private taste/history/favorites/playlists dashboard |
| PUT | `/api/v1/queue/items/{id}/like` | associate a queued track with the signed-in user's taste profile |

Personal mutations and account dashboard routes require authentication.

## Search, metadata, and discovery

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/api/v1/youtube/search?q=` | playable YouTube search results |
| GET | `/api/v1/enrichment?title=` | cached/pending artist-track enrichment |
| GET | `/api/v1/discovery?q=` | cached metadata matches |
| GET | `/api/v1/discovery?genre=` | Last.fm top genre artists/tracks |
| GET | `/api/v1/enrichment/images/{key}` | licensed cached image bytes |

Enrichment may return HTTP `202` with `status: pending`; poll with bounded
backoff. YouTube/discovery can return service-unavailable errors when disabled,
unconfigured, rate-limited, or unable to reach providers.

## Administrator API

Every route requires an administrator session.

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/api/v1/admin/settings` | categorized settings and help links |
| PUT | `/api/v1/admin/settings/{key}` | validate and save `value` |
| DELETE | `/api/v1/admin/settings/{key}` | remove/reset secret setting |
| POST | `/api/v1/admin/lastfm/test` | test submitted or stored key |
| GET | `/api/v1/admin/users` | list local users and roles |
| PUT | `/api/v1/admin/users/{id}/role` | set `admin` boolean |

Secret settings return configuration state, not plaintext. The API prevents
removing the last administrator.

## Executable regression examples

The shell scripts in `scripts/test-*.sh` are the most precise runnable request
examples. `scripts/test-all.sh` launches an isolated server and exercises every
API group, persistence, concurrency, setup, validation, and access modes.

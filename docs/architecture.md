# Architecture

## Overview

House Jukebox is a single Go process with an embedded HTML/CSS/JavaScript
frontend and a SQLite database. The Raspberry Pi is the authority for queue and
playback state. Browsers are remote controls, not audio players.

```text
Phones / desktops
       │ HTTP JSON + Server-Sent Events
       ▼
Go HTTP application ───── SQLite
       │                    queue, users, sessions,
       │                    history, library, settings,
       │                    enrichment cache, fair turns
       ▼
Playback controller ─── mpv IPC ─── ALSA ─── speaker
       │
       ├── source resolvers (direct audio, YouTube via yt-dlp)
       ├── metadata coordinator (Last.fm, MusicBrainz, Wikimedia)
       └── auto-queue engine (YouTube search + cached tastes)
```

## Process lifecycle

`cmd/raspi-media-player` loads configuration, opens SQLite, applies embedded
migrations transactionally, builds stores/providers/controllers, starts the HTTP
server, and runs background playback/auto-queue work. SIGINT/SIGTERM triggers a
bounded graceful HTTP shutdown and controller cleanup.

## HTTP application

`internal/app` owns routes and middleware. The middleware chain captures route
patterns, applies access-mode rules, verifies sessions and CSRF, blocks APIs
before setup, rate-limits mutations/authentication, attaches request IDs, and
emits structured completion logs.

Static frontend files under `internal/app/web` are embedded with `go:embed`.
There is no Node build step. The frontend is a hash-routed single-page app using
fetch, native DOM APIs, and `EventSource`.

## Queue and playback

`internal/queue` persists an ordered queue and singleton playback snapshot.
Mutations use an `If-Match` revision to reject stale reorder/remove operations.
The playback controller advances items, records history, retries failures, and
publishes snapshots. The `mpv` backend communicates over a Unix socket; tests
use a deterministic fake backend.

Queue events flow through SSE. A 30-second browser poll is a recovery path.
Queue DOM rows are only rebuilt when their visual item signature changes, so
progress updates do not flicker or restart artwork loads.

## Identity and authorization

`internal/auth` hashes passwords with Argon2id and stores hashed session/CSRF
tokens. Access modes are enforced independently from the optional-account UI.
Admin endpoints require an authenticated user whose `is_admin` flag is true.

## Metadata and discovery

The enrichment coordinator parses artist/title hints, reads the SQLite cache,
deduplicates in-flight work, queries configured providers, merges results, and
caches licensed images locally. The UI polls pending enrichments with bounded
backoff and discards stale responses using generation IDs.

## Auto-queue

The engine runs every 15 seconds. It computes how many queued items are needed,
selects preferences according to the configured strategy, searches YouTube,
and adds nonduplicate items as `Auto-queue`. Active-listener fairness is stored
in `auto_queue_user_turns`, allowing least-recently-selected rotation to survive
service restarts.

## Database migrations

SQL files in `internal/database/migrations` are embedded and applied in lexical
order. Each migration and its `schema_migrations` record share one transaction.
The database permits one open connection to make SQLite transaction behavior
predictable for this household workload.


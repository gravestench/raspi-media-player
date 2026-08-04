# Raspberry Pi Household Jukebox — Milestones

This document is the working implementation roadmap for a self-hosted household
jukebox. The Raspberry Pi runs the Go application as an `init.d` service and
plays audio through its attached speaker. Browsers on the local network act as
remote controls.

## Product principles

- Anonymous household members can view and add to the shared queue without an
  account.
- Accounts are optional unless the administrator explicitly enables required
  authentication.
- Accounts require only a username and password. Email addresses, password
  recovery, and external identity providers are out of scope.
- The shared queue and current playback state are authoritative on the server
  and update across connected browsers.
- Playback survives browser disconnects and the application recovers cleanly
  after a Raspberry Pi reboot.
- Direct audio and internet-radio streams are the first supported sources.
  Additional source types, including YouTube, are isolated behind source
  adapters and added later.
- Every milestone includes automated regression coverage before it is complete.

## Access modes

The server configuration supports these modes:

1. **Open (default):** anonymous users can browse and modify the queue. Users
   may optionally create accounts for favorites, playlists, and attribution.
2. **Accounts optional:** queue access remains anonymous, but selected actions
   may be restricted to signed-in users through configuration.
3. **Accounts required:** all application and queue mutations require a valid
   session. This is an optional deployment mode, not the default.

In open and accounts-optional modes, the login flow behaves as follows:

- An existing username plus the correct password creates a session.
- An existing username plus an incorrect password returns a generic invalid
  credentials error.
- A username that does not exist redirects/transitions to account creation.
- Account creation asks the user to enter the same password again.
- If the passwords match and the username remains available, the account is
  created and the user is logged in immediately.
- Users may cancel account creation and continue anonymously.
- Usernames are normalized consistently and uniqueness is enforced by the
  database. Passwords are stored only as strong salted hashes.

## Definition of done for every milestone

- [ ] The documented acceptance criteria are met.
- [ ] New API behavior has a shell-based regression test.
- [ ] Go unit/integration tests cover important logic and failure cases.
- [ ] Structured logs contain useful context without passwords, session tokens,
      or other secrets.
- [ ] `go test ./...` passes.
- [ ] The full API regression script passes against a clean database.
- [ ] Configuration and operator-facing behavior are documented.

---

## Milestone 0 — Project foundation

Goal: establish a small, reproducible Go service with conventions that later
milestones can extend safely.

- [x] Initialize the Go module and directory layout (`cmd`, `internal`,
      `web`, `scripts`, and deployment assets).
- [x] Add a single server binary with configuration from flags and/or an
      environment file.
- [x] Add graceful shutdown for `SIGTERM` and `SIGINT`.
- [x] Add `/api/v1/health/live` and `/api/v1/health/ready` endpoints.
- [x] Add version/build metadata to the binary and a version endpoint.
- [x] Establish JSON response and API error formats.
- [x] Add SQLite initialization, migrations, and a temporary test database
      workflow.
- [x] Add a Makefile with build, test, lint, and local-run targets.
- [x] Add a minimal static page served by the Go binary.
- [x] Document local development and Raspberry Pi prerequisites.

Acceptance criteria:

- A fresh checkout can build and start with one documented command.
- Readiness reports failure when required dependencies are unavailable.
- The server exits cleanly and stops accepting requests on termination.

## Milestone 1 — Structured logging and request observability

Goal: make every subsequent feature diagnosable on the Raspberry Pi.

- [x] Use Go structured logging (`log/slog`) throughout the application.
- [x] Support JSON logs for daemon operation and readable text logs for local
      development.
- [x] Add configurable log level.
- [x] Assign or accept a request ID and include it in response headers and all
      logs emitted for that request.
- [x] Log method, route, status, duration, response size, remote address, and
      authenticated username/user ID when available.
- [x] Add structured lifecycle and database events; require queue, source, and
      player milestones to add their own component events.
- [x] Redact passwords, cookies, authorization values, stream credentials, and
      session tokens.
- [x] Document expected log fields and examples for init.d operation.

Acceptance criteria:

- A request can be traced through the service using its request ID.
- Invalid input and internal failures are distinguishable in logs.
- Automated tests verify secret-bearing fields are never logged.

## Milestone 2 — Anonymous shared queue API

Goal: provide useful jukebox behavior without requiring login or a player.

- [x] Define queue item, source, submitter, and playback-state models.
- [x] Create APIs to list the queue and add a direct stream URL.
- [x] Create APIs to remove an item, reorder the queue, clear the queue, and
      skip the current item.
- [x] Allow anonymous queue submissions and attribute them as anonymous with an
      optional display name stored in the browser.
- [x] Validate URLs, supported schemes, item lengths, and request sizes.
- [x] Add optimistic concurrency or queue revision numbers to prevent clients
      from silently overwriting newer changes.
- [x] Persist the queue and playback state in SQLite.
- [x] Establish configurable queue limits and per-client rate limiting.
- [x] Return stable error codes for invalid URLs, duplicates, conflicts, queue
      limits, and unavailable sources.

Acceptance criteria:

- A new installation permits anonymous queue operations in open mode.
- Concurrent modifications do not corrupt or silently lose queue entries.
- Restarting the service preserves the queue.

## Milestone 3 — Optional local accounts and sessions

Goal: add identity without making accounts a prerequisite for household use.

- [x] Add configuration for open, accounts-optional, and accounts-required
      access modes.
- [x] Implement username availability lookup as part of the login/signup flow.
- [x] Implement username-and-password login.
- [x] For an unknown username, return a response the frontend uses to transition
      to password confirmation/account creation.
- [x] Create the account only after matching password confirmation, then issue a
      session immediately.
- [x] Hash passwords using a current memory-hard password hashing scheme with
      per-password salts and configurable cost.
- [x] Store revocable, expiring sessions in secure HTTP-only cookies.
- [x] Implement logout, current-session, and session-revocation endpoints.
- [x] Add CSRF protection for cookie-authenticated mutations.
- [x] Add login/signup throttling without blocking anonymous queue use.
- [x] Attribute new queue items to an account when signed in and preserve
      anonymous attribution otherwise.
- [x] Do not reveal account-sensitive details beyond the intentional unknown
      username transition required by this household-oriented UX.

Acceptance criteria:

- Anonymous users retain the configured queue permissions.
- Unknown-user login, password confirmation, account creation, and immediate
  login work as one continuous flow.
- Duplicate usernames and concurrent signup attempts are handled safely.
- Required mode rejects anonymous requests with consistent API errors.

## Milestone 4 — Raspberry Pi playback controller

Goal: play queued direct audio and radio streams through the attached speaker.

- [x] Integrate `mpv` as a supervised child process using its IPC interface.
- [x] Implement play, pause, resume, stop, skip, seek (when supported), and
      volume controls.
- [x] Advance automatically when a finite item ends.
- [x] Keep live radio streams playing until skipped or stopped.
- [x] Detect player crashes, invalid media, timeouts, and unreachable streams.
- [x] Mark failed items with a reason and continue according to configurable
      retry/skip policy.
- [x] Reconcile persisted queue state with actual player state after restart.
- [x] Publish current title, duration, position, buffering, volume, and errors.
- [x] Add a fake player implementation for deterministic integration tests.

Acceptance criteria:

- A direct audio URL and a radio stream can be queued from another device and
  heard through the Pi speaker.
- Playback continues when all browsers disconnect.
- A broken item cannot permanently stall the queue.

## Milestone 5 — Real-time household web interface

Goal: make the jukebox comfortable to operate from phones and computers.

- [x] Build a responsive now-playing and shared-queue interface.
- [x] Add direct stream/radio URL submission.
- [x] Add playback controls appropriate to the current access mode.
- [x] Push queue and playback changes using Server-Sent Events or WebSockets.
- [x] Reconnect automatically and reconcile missed revisions.
- [x] Show anonymous versus signed-in attribution.
- [x] Add the optional login flow, unknown-user creation transition, password
      confirmation, immediate login, and logout.
- [x] Keep anonymous use obvious; never force the login screen in open mode.
- [x] Add useful empty, loading, buffering, offline, and error states.
- [x] Meet basic keyboard, screen-reader, contrast, and touch-target needs.

Acceptance criteria:

- Multiple browsers see queue and playback changes without refreshing.
- The primary queue and playback tasks work well on a phone-sized screen.
- A user can dismiss account creation and return to anonymous use.

## Milestone 6 — Favorites, stations, and personal library

Goal: give optional accounts persistent personalization while retaining shared
playback.

- [x] Add a curated household station directory usable by anonymous users.
- [x] Add administrator-configured default stations, including KFJC.
- [x] Allow signed-in users to save personal stations and favorites.
- [x] Add named playlists composed of supported source references.
- [x] Add playback/queue history with configurable retention.
- [x] Add search and filtering across stations, favorites, playlists, and recent
      items.
- [x] Define ownership and visibility rules for personal and household items.

Acceptance criteria:

- Anonymous users can quickly play household stations.
- Signed-in users see their saved content on any household browser.
- Deleting a favorite never removes historical or active queue entries.

## Milestone 7 — init.d deployment and operations

Goal: install and operate the application as a conventional Raspberry Pi daemon.

- [x] Provide a POSIX-compatible `/etc/init.d/raspi-media-player` script with
      start, stop, restart, status, and reload where supported.
- [x] Include the correct LSB init metadata and dependency ordering.
- [x] Run under a dedicated unprivileged user and group.
- [x] Define paths for the binary, configuration, SQLite data, PID/runtime files,
      and logs without relying on an interactive home directory.
- [x] Provide an environment/configuration file containing access mode, bind
      address, database path, player settings, log format/level, and limits.
- [x] Ensure startup waits for required local facilities and handles networking
      becoming available later.
- [x] Ensure stop sends `SIGTERM`, waits a bounded period, and prevents orphaned
      player processes.
- [x] Add install, upgrade, uninstall, and configuration-check scripts.
- [x] Preserve the database and local configuration during upgrades/uninstall by
      default.
- [x] Add backup and restore documentation for SQLite and configuration.
- [x] Document enabling at boot on common Raspberry Pi OS versions.

Acceptance criteria:

- The service starts after boot without an interactive login.
- Start/stop/restart/status behave correctly and do not duplicate processes.
- A service restart preserves accounts, sessions as configured, queue, and
  library data.

## Milestone 8 — Complete API regression suite

Goal: make regressions easy to detect on a workstation or Raspberry Pi without
special test software.

- [x] Create `scripts/test-all.sh` as the single entry point.
- [x] Create focused shell scripts for health, anonymous queue behavior, access
      modes, account creation, login/logout, sessions, CSRF, queue controls,
      playback controls, stations, favorites, and playlists.
- [x] Use `curl` plus commonly available shell tools; clearly document any
      additional dependency such as `jq`.
- [x] Start the server with an isolated temporary database and fake player.
- [x] Allocate or accept a configurable test port and avoid disturbing a live
      installation.
- [x] Make scripts fail fast while reporting the request and assertion that
      failed, without printing secrets.
- [x] Cover success, validation, authorization, conflict, throttling, and
      recovery paths.
- [x] Test all configured access modes.
- [x] Add restart/persistence and concurrent-queue regression cases.
- [x] Ensure cleanup runs on success, failure, and interruption.
- [x] Add a CI workflow that runs formatting checks, `go vet`, Go tests, and the
      API regression suite.

Acceptance criteria:

- One command tests the entire API against a disposable environment.
- The suite is repeatable and leaves no processes or test databases behind.
- Each reported failure identifies the endpoint and expected behavior.

## Milestone 9 — Pluggable sources and YouTube feasibility

Goal: add source types without coupling queue and playback logic to a particular
provider.

- [x] Define a source resolver interface and normalized playable-source model.
- [x] Move direct URLs and radio streams behind source adapters.
- [x] Add metadata resolution, caching, timeouts, and cancellation.
- [x] Document the supported and maintainable YouTube playback options before
      selecting an implementation.
- [x] Keep any provider-specific executable or credentials optional and outside
      the core server.
- [x] Detect provider failures and return actionable, non-sensitive errors.
- [x] Add contract tests shared by all source adapters.
- [x] If an acceptable YouTube approach is selected, implement it as an optional
      adapter with explicit configuration and tests. (No acceptable approach was
      selected; YouTube URLs return `unsupported_source`.)

Acceptance criteria:

- Adding or disabling an adapter does not change the queue API.
- A provider outage does not interfere with direct radio playback.
- Deployment documentation states the operational and policy tradeoffs of each
  optional adapter.

## Milestone 10 — Hardening and household release

Goal: produce a dependable first release suitable for daily use.

- [ ] Audit authorization for every endpoint in all access modes.
- [ ] Add database migration upgrade and rollback/recovery tests.
- [ ] Add slow-client, malformed-request, large-request, and concurrency tests.
- [ ] Add browser smoke tests for anonymous queueing and optional signup/login.
- [ ] Verify clean installation and upgrade on a supported Raspberry Pi OS image.
- [ ] Measure idle memory/CPU and playback stability over an extended run.
- [ ] Add operator diagnostics that collect versions, health, player state, and
      recent redacted logs.
- [ ] Write release notes, known limitations, and troubleshooting documentation.
- [ ] Tag the first household-ready release.

Acceptance criteria:

- The service completes an extended playback soak test without queue corruption,
  runaway resource usage, or manual recovery.
- Installation, boot startup, playback, anonymous queueing, and optional account
  creation succeed from the release documentation alone.

---

## Deferred ideas

These are intentionally outside the first household-ready release unless a
milestone is revised:

- Multiple Raspberry Pi players or synchronized rooms.
- Remote access from outside the household network.
- Email addresses, email verification, or password recovery.
- OAuth/social login or third-party identity providers.
- Native mobile applications.
- Public internet hosting or multi-tenant operation.
- Complex moderation, billing, subscriptions, or digital-rights management.

## Current status

- **Milestone 0:** Complete and verified locally and on the Raspberry Pi.
- **Milestone 1:** Complete and verified on 2026-08-04.
- **Milestone 2:** Complete and verified on 2026-08-04.
- **Milestone 3:** Complete and verified on 2026-08-04.
- **Milestone 4:** Complete and verified locally and on the Raspberry Pi on
  2026-08-04, including finite HTTP audio, KFJC live radio, controls, and mpv
  crash recovery.
- **Milestone 5:** Complete and verified with automated tests plus desktop,
  390px-phone, multi-client SSE, anonymous, and account-flow browser testing on
  2026-08-04.
- **Milestone 6:** Complete and verified locally and on the Raspberry Pi on
  2026-08-04.
- **Milestone 7:** Complete and verified locally and on the Raspberry Pi on
  2026-08-04.
- **Milestone 8:** Complete and verified locally on 2026-08-04.
- **Milestone 9:** Complete and verified locally on 2026-08-04. The provider
  boundary is implemented; YouTube extraction is intentionally disabled based
  on the documented technical and platform-policy review.
- **Current milestone:** Milestone 10 — Hardening and household release
- **Next feature milestone:** Household release and ongoing maintenance
- **First playable target:** Completion of Milestone 4
- **First household-friendly target:** Completion of Milestone 5
- **First release target:** Completion of Milestone 10
- **Integration target:** Debian 12 ARM64 Raspberry Pi at `192.168.1.25`; initial
  API daemon deployment verified on 2026-08-04 without interrupting the existing
  VLC/KFJC playback process.

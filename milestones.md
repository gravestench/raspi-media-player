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
      adapter with explicit configuration and tests. (YouTube URLs are handled
      by an explicit adapter through mpv and the external `yt-dlp` executable.)

Acceptance criteria:

- Adding or disabling an adapter does not change the queue API.
- A provider outage does not interfere with direct radio playback.
- Deployment documentation states the operational and policy tradeoffs of each
  optional adapter.

## Milestone 10 — Hardening and household release

Goal: produce a dependable first release suitable for daily use.

- [x] Audit authorization for every endpoint in all access modes.
- [x] Add database migration upgrade and rollback/recovery tests.
- [x] Add slow-client, malformed-request, large-request, and concurrency tests.
- [x] Add browser smoke tests for anonymous queueing and optional signup/login.
- [x] Verify clean installation and upgrade on a supported Raspberry Pi OS image.
- [x] Measure idle memory/CPU and playback stability over an extended run.
- [x] Add operator diagnostics that collect versions, health, player state, and
      recent redacted logs.
- [x] Write release notes, known limitations, and troubleshooting documentation.
- [x] Tag the first household-ready release.

Acceptance criteria:

- The service completes an extended playback soak test without queue corruption,
  runaway resource usage, or manual recovery.
- Installation, boot startup, playback, anonymous queueing, and optional account
  creation succeed from the release documentation alone.

---

## Milestone 11 — Artist imagery and music discovery

Goal: turn stream and YouTube titles into useful, browsable artist context
without delaying or destabilizing playback.

- [x] Define normalized track hints and parse common `Artist - Title` radio and
      YouTube metadata, including removable official-video/audio suffixes.
- [x] Add a provider-neutral enrichment model and persistent cache schema for
      artist images, biographies, genres/tags, related artists, attribution,
      lookup status, and expiry.
- [x] Add a background enrichment coordinator with cancellation, bounded
      timeouts, retry/backoff, negative caching, and no impact on playback.
- [x] Add optional Last.fm configuration and provider support for artist info,
      top tags, and similar artists without storing or logging the API key.
- [x] Add keyless MusicBrainz identity/relationship fallback with the required
      descriptive User-Agent and an average maximum of one request per second.
- [x] Add Wikimedia Commons/Wikidata artist-image fallback with license and
      attribution fields; never hotlink an image without retaining its source.
- [x] Reject placeholder, tiny, unrelated, unsafe-scheme, or uncredited images
      and prefer cached local thumbnails where licensing permits.
- [x] Associate enrichment records with current playback and immutable history
      entries without changing queue ownership or deletion semantics.
- [x] Extend now-playing and history APIs with enrichment state, artist details,
      genres, related artists, image attribution, and graceful unavailable states.
- [x] Display artist imagery and expandable discovery details prominently in
      now-playing and accessibly from each recently played row on desktop and
      phone layouts.
- [x] Allow an operator to disable external metadata entirely and configure
      providers, cache lifetime, request budget, and contact/User-Agent details.
- [x] Add parser, provider-contract, cache, rate-limit, API, regression, and
      browser tests, including ambiguous titles and provider outages.
- [x] Document API-key setup, provider terms/attribution, privacy, cache cleanup,
      troubleshooting, and the limitations of title-derived identification.

Acceptance criteria:

- A recognizable radio or YouTube title gains artist context asynchronously
  while audio continues uninterrupted.
- Now-playing and recent-history views expose the same cached enrichment and
  make image/provider attribution easy to reach.
- Missing keys, ambiguous titles, rate limits, provider downtime, or bad images
  degrade to the original title rather than blocking playback or queue progress.
- Repeated plays use the persistent cache and remain inside provider request
  limits.

---

## Milestone 12 — Guided installation and administration

Goal: make a fresh appliance safe and understandable to configure entirely from
a phone or desktop browser.

- [x] Detect an uninstalled database and route browsers to a full-screen,
      one-question-at-a-time installation wizard.
- [x] Explain privacy, network exposure, playback, access modes, and optional
      external metadata before asking for configuration.
- [x] Create the first local account as an administrator and sign it in when
      installation completes.
- [x] Store runtime settings and encrypted/redacted secrets separately from
      deployment-only bootstrap settings.
- [x] Add administrator-only APIs and SPA routes for general, access, playback,
      metadata, retention, queue, and voting configuration.
- [x] Manage the Last.fm API key with masked reads, explicit replacement/removal,
      connection testing, and links to official setup documentation.
- [x] Add an administrator user list and allow admins to grant or revoke the
      admin role, while preventing removal of the final administrator.
- [x] Require a current admin session and CSRF protection for every admin route;
      never return or log stored secrets.
- [x] Make setup and administration keyboard-, screen-reader-, phone-, and
      desktop-accessible.
- [x] Document first boot, recovery, configuration precedence, secret storage,
      and administrator operations.

Acceptance criteria:

- A clean database cannot enter the player until an initial administrator is
  created, while an upgraded database remains usable.
- Non-admin and anonymous requests cannot read or mutate administrative state.
- A Last.fm key can be installed and tested without appearing in API responses,
  HTML, logs, or browser storage.

## Milestone 13 — Household queue voting

Goal: let active listeners democratically skip an unwanted item without making
single-listener households cumbersome.

- [x] Track recently active anonymous browser identities and signed-in users
      without creating permanent surveillance identifiers.
- [x] Add one skip vote per active identity per playback item and expose the
      current count, threshold, voters' own state, and expiry.
- [x] Skip immediately when only one listener is active; otherwise calculate a
      configurable quorum/percentage threshold.
- [x] Broadcast votes and threshold changes immediately over the existing event
      stream.
- [x] Expire votes after a configurable timeout and clear them on item change,
      stop, or successful skip.
- [x] Add admin settings for enabling voting, active-window length, vote timeout,
      quorum percentage, and optional administrator override.
- [x] Replace direct skip controls with clear vote/unvote affordances where the
      configured policy requires it.
- [x] Require the same vote policy to remove anonymous, auto-queued, or another
      user's queued item; retain immediate removal for owners and administrators.
- [x] Show per-item removal vote counts in the queue and broadcast vote changes
      without rebuilding unrelated queue state.
- [x] Add concurrency, reconnect, expiry, identity, and threshold tests.

Acceptance criteria:

- Every connected UI reflects a new or withdrawn vote without refreshing.
- Votes never carry over to another song and inactive clients stop affecting the
  threshold.
- A lone active listener can skip with one action.

## Milestone 14 — Personal listening dashboard

Goal: give each account a useful view of its listening and saved music.

- [x] Add an authenticated account route with profile and session-management
      basics.
- [x] Show recent submissions/listens, favorites, stations, and playlists in one
      responsive dashboard.
- [x] Derive a genre/tag breakdown from enriched listening history with clear
      empty and partial-data states.
- [x] Allow supported recent items, favorites, stations, and playlist entries to
      be queued directly from the account page.
- [x] Keep personal statistics private to the account except for existing shared
      queue attribution.
- [x] Add pagination/bounds so history aggregation remains inexpensive.

Acceptance criteria:

- A signed-in user can understand their recent listening tastes and requeue an
  item without leaving the dashboard.
- Anonymous and other-user requests cannot read a profile's private data.

## Milestone 15 — Integrated YouTube discovery

Goal: search and queue YouTube audio without leaving the jukebox.

- [x] Add a YouTube search adapter with bounded queries, safe-process execution,
      timeouts, and stable result models.
- [x] Add a debounced search UI with thumbnails, title/channel/duration, loading,
      empty, and failure states.
- [x] Queue a selected result through the existing validated YouTube source
      adapter and preserve submitter attribution.
- [x] Keep direct YouTube URL input available alongside search.
- [x] Add admin controls to enable search, cap results, and configure the search
      backend without exposing secrets.
- [x] Add adapter contract, injection, timeout, queueing, and browser tests.

Acceptance criteria:

- A household member can search, inspect results, and enqueue a video's audio in
  the same page.
- Search failure never disables URL, station, or direct-stream queueing.

## Milestone 16 — Track-level stream history and metadata polish

Goal: treat changing radio metadata as real listening events and make enrichment
useful at a glance.

- [x] Detect meaningful stream-title changes while one radio queue item remains
      active and debounce duplicate/noisy metadata.
- [x] Close the previous track-history segment and create a new immutable segment
      without advancing or rewriting the shared queue item.
- [x] Enrich each detected track asynchronously and retain its normalized raw
      title, timestamps, station/source, and attribution.
- [x] Display genre tags compactly in now-playing and history views.
- [x] Add tests for repeated metadata, rapid changes, missing metadata, restarts,
      and stream-to-finite-item transitions.

Acceptance criteria:

- Two successive songs on a radio stream appear as two recent-history entries
  even though playback never stopped.
- Metadata noise cannot flood history and metadata processing cannot interrupt
  playback.

## Milestone 17 — Responsive single-page player redesign

Goal: make the player, setup, admin, discovery, and account experiences feel
like one fast, expressive household appliance.

- [x] Establish client-side routes and shared application state without breaking
      deep links, refreshes, anonymous use, or progressive loading.
- [x] Redesign now-playing information into a compact hierarchy with artwork,
      title, artist, genres, attribution, progress, and primary controls.
- [x] Make queue, search, library, account, and admin destinations easy to reach
      with phone bottom navigation and space-efficient desktop navigation.
- [x] Add purposeful CSS3 transitions, artwork treatments, live vote feedback,
      loading skeletons, and reduced-motion equivalents.
- [x] Remove duplicated labels, oversized empty regions, and visually noisy
      metadata while retaining details behind accessible disclosure controls.
- [x] Verify keyboard navigation, focus management, contrast, landmarks, touch
      targets, screen-reader names, and 320px through wide-desktop layouts.
- [x] Add browser smoke tests for anonymous, account, admin, setup, search,
      voting, offline/reconnect, and reduced-motion flows.

Acceptance criteria:

- The primary now-playing and queue actions remain readable and reachable on a
  phone without wasted space or horizontal scrolling.
- Desktop uses available space without turning metadata into an unstructured
  wall of content.
- Animation communicates state and respects `prefers-reduced-motion`.

---

## Milestone 20 — Unified playback metadata and personal likes

Goal: show the same canonical enriched identity everywhere and let listeners
claim recommendations they enjoy as part of their personal taste profile.

- [x] Prefer the canonical queued title for YouTube Now Playing enrichment while
      retaining changing player metadata for live radio streams.
- [x] Add an authenticated, idempotent track-like API and persistent storage.
- [x] Put the like action in the Now Playing controls without interrupting or
      duplicating playback.
- [x] Display liked tracks on the account dashboard and include them in genre
      counts and active-listener auto-queue weighting.
- [x] Cover likes with unit, API, migration, and full regression tests.

Acceptance criteria:

- Now Playing and its matching queue row resolve metadata from the same title.
- A signed-in listener can associate somebody else's or auto-queue's current
  track with their profile in one action.
- Repeated likes do not create duplicates, and anonymous visitors are prompted
  by the existing authentication boundary.

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
  boundary is implemented; YouTube queue input is enabled through mpv/yt-dlp.
- **Milestone 10:** Complete and verified locally and on the Raspberry Pi on
  2026-08-04, including diagnostics, live browser smoke testing, and playback
  resource observation.
- **Milestone 11:** Complete and verified locally and on the Raspberry Pi on
  2026-08-04, including live multi-provider enrichment, attributed local image
  caching, API/browser validation, and uninterrupted playback.
- **Milestones 12–17:** Complete and verified locally and on the Raspberry Pi on
  2026-08-04: guided installation/admin, encrypted configuration, role
  management, live skip voting, personal dashboards, integrated YouTube search,
  track-level stream history, and the responsive SPA redesign.
- **Milestone 18:** Active-listener auto-queue, weighted artist/genre selection,
  configurable queue depth, and unified search/URL entry.
- **Milestone 19:** Three-mode auto-queue strategy controls: persistent fair
  rotation across active listeners, explicit artist/genre seeds, and cached
  related-artist/genre continuation from the last queued item.
- **Milestone 20:** Unified Now Playing enrichment plus persistent personal
  likes that feed the account dashboard and recommendation profile.
- **Current milestone:** Household feedback and ongoing maintenance
- **First playable target:** Completion of Milestone 4
- **First household-friendly target:** Completion of Milestone 5
- **First release target:** Completion of Milestone 10
- **Integration target:** Debian 12 ARM64 Raspberry Pi; the init.d-managed API
  and mpv playback daemon are deployed and verified.

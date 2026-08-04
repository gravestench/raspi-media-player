# Guided installation and administration

## First boot

A fresh production database exposes only health/version APIs and the static
installer until setup is complete. Open the Pi's URL in a phone or desktop
browser and follow the full-screen steps. The wizard chooses household access,
creates the first local administrator, and optionally accepts a Last.fm API key.
No email address is requested.

Databases upgraded from an earlier release are marked installed automatically;
the oldest existing account is promoted so the household is not locked out.

The installer is transactionally single-use. Completing it creates an admin
session immediately. Losing every administrator is prevented by the API; keep a
database backup because there is intentionally no email recovery flow.

## Administrator area

Signed-in administrators see an **Admin** destination. It contains access,
queue, playback, library retention, metadata, skip-voting, and YouTube-search
settings plus account role management. Anonymous users and ordinary accounts
receive `401` or `403` from every `/api/v1/admin/*` endpoint.

Most low-level settings apply after restarting the daemon. Values stored by the
admin UI override `/etc/default/raspi-media-player` on the next start. Deployment
paths, the bind address, database location, logging destination, and encryption
bootstrap key stay operator-controlled because changing them from the running
web process would be unsafe.

## Secrets and Last.fm

The service manager generates `RASPI_MEDIA_PLAYER_SETTINGS_SECRET_KEY` in
`/etc/default/raspi-media-player` when it is absent. API keys saved in the UI are
AES-GCM encrypted before SQLite storage. Admin reads expose only a `configured`
boolean, never the key. Removing a key stores an explicit empty override so an
older environment value does not silently return after restart.

Create a Last.fm API key at <https://www.last.fm/api/account/create> and review
their terms at <https://www.last.fm/api/tos>. The Admin page can test either a
newly entered key or the stored key. Test failures are deliberately generic and
structured logs never contain the submitted value.

Back up the settings encryption key separately from the SQLite database. A
database copied without its key remains usable, but encrypted provider secrets
must be replaced.

## Household voting

Each browser receives a random, year-lived household-listener cookie. Signed-in
users vote by account identity. These identifiers are used only to count recent
activity and one vote per skip/removal target; activity and votes live in memory
and expire automatically.

The admin can enable voting, set the active-listener window, vote timeout, and
required percentage. One active listener always has a threshold of one. Votes
to skip or to remove another listener's, anonymous, or auto-queued item. Skip
votes clear on track changes; removal votes remain scoped to their queue item.
All votes are delivered in queue/SSE snapshots for immediate UI feedback.
Administrators retain immediate skip and removal overrides, and signed-in users
can always remove their own submissions directly.

## YouTube discovery and personal dashboards

YouTube search uses the installed `yt-dlp` executable with argument-safe process
execution and a bounded timeout. It does not use a shell. Search may be disabled
or result counts capped in Admin; direct YouTube URL queueing remains available.

Account dashboards are private to the current session. They combine that user's
submitted listening history, favorites, playlists, and genre counts from cached
enrichment. Any listed source can be queued from the dashboard.

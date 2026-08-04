# v0.1.0 — Household release

The first household-ready release provides anonymous shared queueing, optional
username/password accounts, personal stations/favorites/playlists, responsive
real-time controls, persistent playback history, structured logs, supervised
mpv playback, init.d operations, backups, diagnostics, and a complete disposable
API regression suite.

Validated target: Debian 12 ARM64 Raspberry Pi with an attached ALSA speaker.

Known limitations:

- Direct HTTP(S) audio, internet-radio streams, and YouTube URLs are supported.
  YouTube playback is best-effort and requires the external `yt-dlp` executable.
- One Raspberry Pi/output zone is supported.
- The service is designed for a trusted household LAN, not public internet
  exposure. TLS and password recovery are not included.
- Playlists contain direct source references and do not import third-party
  provider playlists.
- Uninstall preserves configuration, database, logs, and the service user.

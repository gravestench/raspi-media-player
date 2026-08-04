# v0.1.0 — Household release

The first household-ready release provides anonymous shared queueing, optional
username/password accounts, personal stations/favorites/playlists, responsive
real-time controls, persistent playback history, structured logs, supervised
mpv playback, init.d operations, backups, diagnostics, and a complete disposable
API regression suite.

Validated target: Debian 12 ARM64 Raspberry Pi with an attached ALSA speaker.

Known limitations:

- Direct HTTP(S) audio and internet-radio streams are supported. YouTube is
  intentionally unsupported after the documented platform-policy review.
- One Raspberry Pi/output zone is supported.
- The service is designed for a trusted household LAN, not public internet
  exposure. TLS and password recovery are not included.
- Playlists contain direct source references and do not import third-party
  provider playlists.
- Uninstall preserves configuration, database, logs, and the service user.

# Security model

House Jukebox is designed for a trusted household LAN. It is not hardened as a
public multi-tenant internet service.

## Network boundary

The supplied Pi configuration listens on all interfaces so household devices
can connect. Use router/firewall rules to limit port 8080 to trusted LANs. For
remote access, prefer a VPN. If placing the app behind HTTPS, enable secure
cookies and configure the reverse proxy to preserve sensible client addresses.

## Accounts and passwords

Accounts require only username/password. Passwords are Argon2id hashes with
configurable memory and iteration costs. There is intentionally no email
recovery. Back up the database and maintain at least one administrator account.

Session tokens are random and stored as hashes. Cookies are HttpOnly and
SameSite; mutation requests from authenticated sessions require a matching CSRF
token. Login/signup endpoints and anonymous queue mutations have independent
per-client rate limits.

## Access modes

- `open`: household playback/queue mutations may be anonymous.
- `accounts_optional`: preserves open listening while enabling local accounts
  and reserving selected personal behavior for them.
- `accounts_required`: protected playback/queue/auto-queue mutations require a
  valid account.

Admin APIs always require an administrator regardless of household mode.

## Provider secrets

Secret application settings are AES-GCM encrypted in SQLite with a key from
`RASPI_MEDIA_PLAYER_SETTINGS_SECRET_KEY`. API responses expose only a configured
flag. Structured logs avoid request bodies and provider key values.

Protect both `/etc/default/raspi-media-player` and database backups. Possession
of the database plus encryption key reveals configured provider secrets.

## Process permissions

The service runs as the dedicated `raspi-media-player` account, which belongs to
the audio group. The data directory is mode `0750`; the log is `0640`; the
defaults file is root-owned. The service manager needs root only for install,
configuration, and lifecycle operations. The web process does not edit its
operator-owned defaults file.

## External content

URLs are validated as HTTP(S). `yt-dlp` and `mpv` are executed with explicit
argument vectors rather than shell interpolation. Metadata text is placed into
the DOM with `textContent`; generated links use bounded application data. Keep
OS packages, `mpv`, and `yt-dlp` current because they process untrusted network
media and metadata.

## Reporting

Do not include passwords, provider keys, session cookies, defaults files, or
private LAN/SSH details in bug reports. Include the application version,
request ID, redacted structured log event, OS version, and reproduction steps.


# Local accounts and access modes

Accounts are local to the household jukebox and require only a username and
password. No email address or recovery workflow exists. Anonymous access remains
the default.

## Access modes

- `open` (default): anonymous queue access with optional accounts.
- `accounts_optional`: anonymous household queue/playback access plus optional
  accounts for personal library and dashboard features.
- `accounts_required`: anonymous users may view the queue but must sign in for
  queue mutations.

Choose a mode during guided setup, through Admin, or with
`RASPI_MEDIA_PLAYER_ACCESS_MODE`. Invalid modes stop the service at startup.

## Login and account creation

`POST /api/v1/auth/login` accepts `username` and `password`. An unknown username
returns `status: account_creation_required` and the normalized username. The UI
then asks for `password_confirmation` and sends all three fields to
`POST /api/v1/auth/signup`. Matching passwords create the account and immediately
return an authenticated session. Existing usernames with a wrong password return
the generic `invalid_credentials` error.

Usernames are 2–32 letters, numbers, hyphens, or underscores and are unique
case-insensitively. Passwords are 8–256 characters. Passwords are stored as
salted Argon2id hashes; memory and iteration costs are configurable.

`GET /api/v1/auth/usernames/{username}` reports availability. This deliberate
account enumeration supports the requested household signup flow.

## Sessions and CSRF

The opaque session token is stored hashed in SQLite and delivered in the
HTTP-only, SameSite=Strict `jukebox_session` cookie. The CSRF token is stored in
the readable, SameSite=Strict `jukebox_csrf` cookie so the frontend can copy it
to `X-CSRF-Token`. The server stores only its hash. HTTPS deployments should set
`RASPI_MEDIA_PLAYER_SECURE_COOKIE=true`; local HTTP deployments must leave it
false.

- `GET /api/v1/auth/session` returns the current identity or anonymous status.
- `POST /api/v1/auth/logout` revokes the current session.
- `GET /api/v1/auth/sessions` lists the user's active sessions.
- `DELETE /api/v1/auth/sessions/{id}` revokes one of the user's sessions.

All cookie-authenticated POST, PUT, PATCH, and DELETE requests except login and
signup require `X-CSRF-Token`. Anonymous mutations in an access mode that allows
them do not require CSRF because they carry no ambient authentication authority.
Login and signup attempts are throttled independently of anonymous queue use.

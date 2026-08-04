# Stations, favorites, playlists, and history

The library is optional personalization layered over the shared household
queue. Anonymous visitors can list, search, and queue curated household
stations. The initial database migration installs KFJC 89.7 FM as the first
household station.

Signed-in users can save private stations, favorite any station visible to
them, and create playlists of direct audio or radio stream URLs. Personal
stations, favorites, and playlists are visible only to their owner. Household
stations and playback history are visible to everyone on the local jukebox.
Deleting a favorite only deletes the saved association; it does not alter the
station, queue, or history.

## API

- `GET /api/v1/stations?q=` lists visible stations; `POST` creates a personal
  station.
- `PUT /api/v1/stations/{id}/favorite` changes the current user's favorite.
- `GET /api/v1/favorites` lists the current user's favorites.
- `GET|POST /api/v1/playlists` lists or creates personal playlists.
- `POST /api/v1/playlists/{id}/items` adds a supported source and
  `DELETE /api/v1/playlists/{id}/items/{itemID}` removes it.
- `GET /api/v1/history?q=` lists recent playback attempts.
- `GET /api/v1/library/search?q=` searches all library categories visible to
  the caller.

Cookie-authenticated mutations require the CSRF token described in
`authentication.md`. Source URLs accept HTTP and HTTPS direct streams, using
the same validation as queue submissions.

## Retention and defaults

`RASPI_MEDIA_PLAYER_HISTORY_DAYS` (or `-history-days`) controls history
retention and defaults to 90 days. A value of `0` keeps history indefinitely.
Pruning occurs as playback finishes.

Household stations are deployment-owned seed data. Additional defaults should
be introduced through a migration so clean installations and upgrades receive
the same stable station IDs.

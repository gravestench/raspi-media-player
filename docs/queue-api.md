# Anonymous queue API

The shared queue is available without authentication in the default open mode.
All responses are JSON. A queue snapshot contains a monotonic `revision`, ordered
`items`, and `playback` state. The same revision is returned as an `ETag`.

## Endpoints

- `GET /api/v1/queue` returns the current snapshot.
- `POST /api/v1/queue/items` adds a direct HTTP(S) stream. Its JSON body is
  `{"url":"https://…","display_name":"optional name"}`.
- `DELETE /api/v1/queue/items/{id}` removes an item.
- `PUT /api/v1/queue/order` reorders all items with
  `{"item_ids":["id-2","id-1"]}`.
- `DELETE /api/v1/queue` clears the queue.
- `POST /api/v1/queue/skip` removes the current (first) item until the player
  milestone owns actual playback advancement.

All mutations except additive submission require `If-Match: "<revision>"`.
Stale revisions receive `409 revision_conflict`; callers should fetch the latest
snapshot and ask the user to retry. Missing revisions receive HTTP 428.

URLs must be absolute HTTP or HTTPS URLs, may not contain credentials, and are
limited to 2,048 characters. Anonymous display names are optional and limited to
64 characters. Duplicate active URLs are rejected. Queue length and per-client
submission rate are controlled by `RASPI_MEDIA_PLAYER_QUEUE_LIMIT` and
`RASPI_MEDIA_PLAYER_QUEUE_RATE`.

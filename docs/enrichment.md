# Artist metadata enrichment

Artist enrichment runs asynchronously and never delays playback. The player
parses common `Artist - Title` metadata from radio and YouTube titles, checks a
persistent cache, and asks configured providers only when a cached entry is
missing or expired. Failed and not-found lookups are negatively cached.

## Last.fm

Create a Last.fm API application and put only its API key in the Pi-local file:

```sh
RASPI_MEDIA_PLAYER_LASTFM_API_KEY=your-api-key
```

The application uses read-only `artist.getInfo`; its shared secret is not
needed and should not be installed. `/etc/default/raspi-media-player` is mode
`0640`, deployment upgrades preserve it, the key is inherited through the
daemon environment instead of appearing in process arguments, and provider
errors/logs omit request URLs and credentials.

Restart the service after changing the key. Leaving the value empty disables
external enrichment and returns a graceful `disabled` state.

The provider currently supplies the corrected artist name, biography, tags,
similar artists, artist page, and the best image URL returned by Last.fm. The
now-playing panel loads it automatically; recent-history rows expose an
`Artist info` action. Images link back to their provider attribution source.

Last.fm data can be incomplete or ambiguous, especially when a stream title is
not formatted as artist and track. The original playback title always remains
visible.

## MusicBrainz and Wikimedia fallback

MusicBrainz corrects artist identity and contributes tags and artist
relationships. Requests carry `RASPI_MEDIA_PLAYER_METADATA_USER_AGENT` and are
serialized to the service's required average maximum of one request per second.
Wikidata/Wikimedia Commons supplies a fallback artist photo only when the search
result describes a musician and Commons returns a source page plus non-empty
credit/license metadata.

Licensed Wikimedia thumbnails are validated as HTTPS PNG/JPEG images, limited
to 5 MiB, rejected below 120×120, and cached under
`RASPI_MEDIA_PLAYER_METADATA_IMAGE_DIR`. The browser receives a same-origin
image URL while the original Commons page and attribution remain in the API.
Last.fm images are not copied locally because their reuse license is not
provided by this API response.

Set `RASPI_MEDIA_PLAYER_METADATA_ENABLED=false` to disable all external
metadata requests. `RASPI_MEDIA_PLAYER_METADATA_CACHE_DAYS` defaults to seven;
expired records are pruned at startup, provider misses are negatively cached,
licensed thumbnail files older than that lifetime are removed, and repeated
playback titles reuse SQLite cache entries. `RASPI_MEDIA_PLAYER_METADATA_MAX_INFLIGHT`
(default 2) bounds concurrent lookup jobs. Lookups send the
parsed artist/title and the configured application User-Agent to providers; no
household usernames, queue submitters, or Last.fm shared secret are sent.

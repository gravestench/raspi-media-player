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
visible. MusicBrainz identity matching and Wikimedia-licensed image fallback
remain planned later in Milestone 11.

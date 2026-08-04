# Source adapters and YouTube feasibility

Queue and playback code depend on a source resolver rather than on a provider.
An adapter classifies an original user URL and resolves it into a normalized
playable URL and optional title. The registry applies bounded resolution
timeouts, cancellation, and an in-memory metadata/result cache. Adapter errors
mark only that queue item as failed, so a provider outage cannot stop direct
radio playback.

The built-in `direct` adapter accepts credential-free absolute HTTP and HTTPS
audio or radio URLs. Queue API clients still submit the same `{ "url": "…" }`
body; adding or removing adapters does not change that contract. Provider
executables and credentials must remain optional and outside the core binary.

## YouTube playback

YouTube watch and short URLs are accepted as `youtube` queue sources. The
adapter preserves the original URL and hands it to mpv; mpv's `ytdl_hook` uses
the separately installed `yt-dlp` executable to resolve the playable audio URL.
This avoids persisting short-lived provider URLs. Extraction remains inherently
best-effort because YouTube delivery and `yt-dlp` behavior can change
independently of this application. See the
[official yt-dlp README](https://github.com/yt-dlp/yt-dlp/blob/master/README.md).

YouTube's current developer guidance restricts audio separation and background
play for API clients, and its general terms restrict downloading or using
content except when the service, rights holders, or applicable law permit it.
Household operators are responsible for deciding whether their intended use and
content are permitted. See YouTube's
[developer policy guide](https://developers.google.com/youtube/terms/developer-policies-guide),
[API developer policies](https://developers.google.com/youtube/terms/developer-policies),
and [Terms of Service](https://www.youtube.com/t/terms).

`yt-dlp` is an external runtime dependency rather than part of the Go binary.
Provider failures mark only the affected queue item and do not interfere with
direct streams such as KFJC.

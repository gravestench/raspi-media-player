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

## YouTube decision

YouTube playback is deliberately **not enabled** in this release. Technically,
an optional `yt-dlp` adapter could emit machine-readable metadata using its
documented `--dump-single-json` mode and hand a temporary media URL to mpv.
That implementation would be brittle because extraction behavior and YouTube
delivery change independently of this application. See the
[official yt-dlp README](https://github.com/yt-dlp/yt-dlp/blob/master/README.md).

More importantly, YouTube's current developer guidance says API clients must
not separate audio tracks or enable background play, and its general terms
restrict downloading or using content except when the service, rights holders,
or applicable law permit it. A headless living-room audio extractor conflicts
with those stated platform rules. See YouTube's
[developer policy guide](https://developers.google.com/youtube/terms/developer-policies-guide),
[API developer policies](https://developers.google.com/youtube/terms/developer-policies),
and [Terms of Service](https://www.youtube.com/t/terms).

Accordingly, this project keeps the adapter boundary ready but does not install,
invoke, or endorse a YouTube extractor. A future adapter should be considered
only if an officially supported living-room/background-audio path or explicit
permission makes the use case acceptable. Direct streams such as KFJC remain
fully supported regardless of that decision.

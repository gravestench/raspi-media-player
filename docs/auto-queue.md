# Auto-queue

Auto-queue is an optional household recommendation loop. It is disabled by
default. Anyone allowed to control playback can turn it on or off from the Now
Playing screen. An administrator configures its depth and listener window
without restarting the service.

The service checks the queue every 15 seconds and adds only enough tracks to
maintain the configured number of queued tracks behind the current item. It
never removes or reorders manual selections. Automatically selected items are
shown as added by `Auto-queue`.

## Active listeners and taste weighting

Only signed-in users with a non-revoked session seen within the configured
active-session window influence a refill. The player checks in every 30 seconds
while its page is open. Anonymous browsers can still control and add to the
queue, but cannot supply a personal recommendation profile.

The engine reads each active user's recent personal playback history and counts
artists. When cached enrichment is available, it also counts the genres of
those artists. Counts are normalized per user before being merged, preventing a
single account with a much longer history from completely overwhelming another
active listener. A weighted random artist or genre is then selected.

Artist choices search YouTube and randomly select from the returned playable
tracks. Genre choices use a random Last.fm tag track when an API key is
configured, then search YouTube for that exact artist and track. Without a
Last.fm key they fall back to a genre search. Duplicate URLs are skipped.

## Settings

- `Auto-queue`: enables or disables refilling.
- `Tracks kept ahead`: desired count of queued automatic/manual tracks behind
  the currently playing item, from 1 through 20.
- `Active session window`: number of seconds, from 30 through 3600, during
  which a signed-in browser affects recommendations.

Integrated YouTube search must also be enabled. Auto-queue pauses when there
are no active signed-in users, no personal history, or no playable search
results.

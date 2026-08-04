# Auto-queue

Auto-queue is an optional household recommendation loop. It is disabled by
default. Anyone allowed to control playback can turn it on or off from the Now
Playing screen. An administrator configures its depth and listener window
without restarting the service.

The service checks the queue every 15 seconds and adds only enough tracks to
maintain the configured number of queued tracks behind the current item. It
never removes or reorders manual selections. Automatically selected items are
shown as added by `Auto-queue`.

## Recommendation strategies

The Player screen offers three strategies.

### Active listeners · fair rotation

Only signed-in users with a non-revoked session seen within the configured
active-session window influence a refill. The player checks in every 30 seconds
while its page is open. Anonymous browsers can still control and add to the
queue, but cannot supply a personal recommendation profile.

The engine selects the active user whose last successful auto-queue turn is
oldest. Users with no previous turn tie and one is selected randomly. After a
successful addition, the turn timestamp is persisted in SQLite, so everyone is
represented across repeated refills and service restarts. A selected user's
recent personal history and explicitly liked tracks become the weighted
artist/genre pool for that turn.

### Specific artists or genres

Enter comma-separated artists and genres directly below Now Playing. Each seed
has equal initial weight. Artist seeds become music searches; genre seeds use a
random Last.fm tag track when a key is configured and otherwise fall back to a
genre music search.

### Related to the last queued item

The engine parses the final queued item's artist/title and reads its cached
genres and related artists. These become the next weighted pool, producing a
continuation of the queue's current direction. If metadata has not finished or
the title cannot be parsed, auto-queue waits rather than guessing.

Artist choices search YouTube and randomly select from the returned playable
tracks. Genre choices use a random Last.fm tag track when an API key is
configured, then search YouTube for that exact artist and track. Without a
Last.fm key they fall back to a genre search. Duplicate URLs are skipped.

## Settings

- `Auto-queue`: enables or disables refilling.
- `Recommendation strategy`: active users, explicit seeds, or related last item.
- `Seed artists` / `Seed genres`: comma-separated values used by the explicit
  strategy (up to 1,000 characters each).
- `Tracks kept ahead`: desired count of queued automatic/manual tracks behind
  the currently playing item, from 1 through 20.
- `Active session window`: number of seconds, from 30 through 3600, during
  which a signed-in browser affects recommendations.

Integrated YouTube search must also be enabled. Active-listener mode pauses when
there are no active signed-in users or no usable personal history. Explicit
mode pauses with no seeds. Related mode pauses with no parseable/cached context.
Every mode pauses when searches return no playable results.

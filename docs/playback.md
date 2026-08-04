# Raspberry Pi playback

The production player is a supervised `mpv` child process controlled through a
private Unix-domain JSON IPC socket. Browsers never communicate with `mpv`
directly. The coordinator reconciles SQLite queue state with player state after
startup and continues playback without an open browser.

The Raspberry Pi deployment enables playback and uses the analog headphone
device by default:

```text
RASPI_MEDIA_PLAYER_PLAYER_ENABLED=true
RASPI_MEDIA_PLAYER_PLAYER_BACKEND=mpv
RASPI_MEDIA_PLAYER_AUDIO_DEVICE=alsa/plughw:CARD=Headphones,DEV=0
```

Set the audio device to `auto` or another value shown by `mpv --audio-device=help`
when using HDMI, USB, or another sound card. Streaming uses a configurable
20-second cache by default. The test suite uses the deterministic `fake` backend
and never emits sound.

## Behavior

- The first playable queued item loads automatically.
- Finite media advances on end-of-file.
- Live radio continues until stopped or skipped.
- Media failures remain in the queue with `status: failed` and an error reason;
  playback proceeds to the next queued item after the configured retry count.
- If `mpv` crashes, the driver records the failure, restarts with backoff, and
  the coordinator reloads the current item.
- Persisted `current` state is reconciled to queued state after service restart.

The queue snapshot publishes status, current item ID, title, position, duration,
pause/buffering state, volume, and player errors.

`RASPI_MEDIA_PLAYER_PLAYER_RETRIES` controls retries per item before it is marked
failed; the Raspberry Pi deployment defaults to one retry. Set it to zero to
skip directly to failure handling.

## Control endpoints

- `POST /api/v1/playback/pause`
- `POST /api/v1/playback/resume`
- `POST /api/v1/playback/stop`
- `POST /api/v1/playback/seek` with `{"position_seconds": 30}`
- `PUT /api/v1/playback/volume` with `{"volume": 50}`

Queue skip remains `POST /api/v1/queue/skip`. Cookie-authenticated requests need
the usual CSRF header. Anonymous controls remain available in open mode.

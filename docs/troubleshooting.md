# Troubleshooting

Run `sudo raspi-media-player-service config-check` first, then
`sudo service raspi-media-player status`. `scripts/diagnose.sh` produces a
compact support report (`sudo raspi-media-player-diagnose`) with OS/binary
versions, health, queue and player state,
process resource use, and recent logs. It omits cookies, passwords, session
tokens, and source URLs from the queue summary and redacts URL credentials in
logs.

## No sound

Confirm the web UI says `playing`, volume is nonzero, and `mpv` is present.
Run `aplay -l`, compare the desired ALSA device with
`RASPI_MEDIA_PLAYER_AUDIO_DEVICE`, and inspect the application log for mpv
startup errors. On the validated Pi the analog output is
`alsa/plughw:CARD=Headphones,DEV=0`.

## Stream fails or stalls

Try KFJC from the household station directory. If it works, the other URL is
unreachable or not a direct media stream. Failed provider items do not block
later direct streams. YouTube URLs are intentionally unsupported; see
`sources.md`.

## Service will not start

Check configuration, ownership of `/var/lib/raspi-media-player`, the configured
mpv path, and whether port 8080 is already in use. The service log is
`/var/log/raspi-media-player.log`. Restore the last SQLite backup using the
procedure in `operations.md` if migration/database errors persist.

## Browser looks stale

The interface reconnects its Server-Sent Events stream automatically. Reload
once, then verify `/api/v1/health/ready` from the same device and confirm local
firewall/Wi-Fi isolation is not blocking the Pi.

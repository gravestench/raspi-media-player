# Installation and first run

## Supported deployment

The production scripts target a 64-bit ARM Raspberry Pi running Debian 12 or
Raspberry Pi OS. The service uses `mpv` for audio output, SQLite for durable
state, and a SysV init script that also works through systemd's compatibility
layer.

You need:

- a Raspberry Pi reachable over SSH;
- an audio device visible to ALSA;
- an always-on speaker connected to that device;
- a development machine with Go 1.24+, `make`, `ssh`, and `scp`;
- internet access from the Pi for network radio, YouTube, and metadata.

## 1. Install Pi dependencies

Copy `scripts/install-dependencies.sh` to the Pi and run:

```sh
chmod +x install-dependencies.sh
sudo ./install-dependencies.sh
./install-dependencies.sh --check
```

The script installs `mpv`, `yt-dlp`, ALSA tools, SQLite tooling, `curl`, `jq`,
and service utilities. It is idempotent and can be run again after OS upgrades.
Go is intentionally not installed on the Pi.

## 2. Check the audio device

On the Pi:

```sh
aplay -l
speaker-test -t sine -f 440 -c 2
```

Stop the tone with Ctrl-C. Record the intended ALSA/mpv device in
`RASPI_MEDIA_PLAYER_AUDIO_DEVICE`; the supplied default is an example and may
not match USB DACs, HDMI, or newer Pi models.

## 3. Deploy the application

From the repository on the development machine:

```sh
TARGET=user@jukebox.local make deploy-pi
```

This cross-compiles a Linux/ARM64 binary, uploads a temporary bundle, installs
the binary and init.d assets, preserves existing configuration, validates it,
and starts the service. SSH and sudo authentication remain interactive. The
destination is supplied at runtime and is never written to the repository.

The important installed paths are:

| Path | Purpose |
| --- | --- |
| `/usr/local/bin/raspi-media-player` | application binary |
| `/etc/init.d/raspi-media-player` | init.d service |
| `/etc/default/raspi-media-player` | operator environment/configuration |
| `/var/lib/raspi-media-player/player.sqlite` | SQLite database |
| `/var/lib/raspi-media-player/artist-images` | cached metadata images |
| `/var/log/raspi-media-player.log` | structured daemon log |

## 4. Complete guided setup

Open `http://jukebox.local:8080` (or the Pi's LAN address). A new database shows
five full-screen steps:

1. **Welcome** explains the local household model.
2. **Household access** chooses open, accounts-optional, or accounts-required.
3. **Administrator** creates the first username/password account. No email is
   requested.
4. **Metadata** optionally accepts a Last.fm API key.
5. **Review** confirms and completes the one-time installation.

![Household access step in the guided installer](images/setup-access.png)

The first account is an administrator and receives a session immediately.
Installation completion is transactional and cannot be repeated through the
browser after it succeeds.

## Last.fm setup

Last.fm is optional. Without a key, core playback and queueing work, while some
genre discovery and recommendation depth is reduced. Create a key at
<https://www.last.fm/api/account/create>. Only the API key is required; the
shared secret is not used by this application.

Keys submitted through setup or Admin are encrypted before SQLite storage. The
encryption bootstrap key remains in `/etc/default/raspi-media-player`.

## Verify the installation

```sh
curl http://jukebox.local:8080/api/v1/health/live
curl http://jukebox.local:8080/api/v1/health/ready
ssh user@jukebox.local 'sudo service raspi-media-player status'
```

Both health endpoints should return `{"status":"ok"}` / `ready`, and the
service should be active.

## Upgrade

Run the same deployment command. Upgrades preserve the configuration and
database and apply embedded, transactional SQLite migrations on startup:

```sh
TARGET=user@jukebox.local make deploy-pi
```

Take a backup first for significant upgrades. See [operations.md](operations.md).

## Start over while preserving configuration

This is destructive. Stop the service, remove the configured SQLite database
and its `-wal`/`-shm` sidecars, then deploy again. Keep
`/etc/default/raspi-media-player`, ensure
`RASPI_MEDIA_PLAYER_SETUP_REQUIRED=true`, and verify
`GET /api/v1/setup/status` returns `"installed":false`. Never guess a database
path—read it from the preserved configuration first.


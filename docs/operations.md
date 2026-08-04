# Raspberry Pi service operations

The application runs as the dedicated `raspi-media-player` user through the
SysV init script `/etc/init.d/raspi-media-player`. On current Raspberry Pi OS,
systemd's SysV compatibility generator manages the same service.

## Install and upgrade

Install the dependencies once with `scripts/install-dependencies.sh`, then use
`make deploy-pi` from the development machine. Deployment cross-compiles the
ARM64 binary, uploads a staging bundle, and runs the service manager's
`upgrade` action. Existing `/etc/default/raspi-media-player` and SQLite data are
preserved. A new sample defaults file is installed under
`/usr/share/raspi-media-player/` for comparison.

For a bundle already copied onto the Pi:

```sh
sudo ./service-manager.sh install /path/to/bundle
sudo ./service-manager.sh upgrade /path/to/bundle
sudo raspi-media-player-service config-check
```

Use `sudo service raspi-media-player start|stop|restart|status`. Installation
runs `update-rc.d raspi-media-player defaults`, enabling startup in normal
multi-user runlevels on Debian and Raspberry Pi OS. The init metadata orders
startup after networking, remote filesystems, and logging. Network streams are
retried/skipped by the playback controller when connectivity is late or
temporarily unavailable; the web service itself remains available locally.

## Configuration

Edit `/etc/default/raspi-media-player`, then run the configuration check and
restart. The supplied file documents all runtime values: bind and database
paths, structured logging, access mode and rate limits, session/password
settings, mpv/audio settings, retry policy, and history retention.

```sh
sudo raspi-media-player-service config-check
sudo service raspi-media-player restart
```

## Backup and restore

Create a consistent online SQLite backup and separately copy the configuration:

```sh
sudo install -d -m 0700 /var/backups/raspi-media-player
sudo sqlite3 /var/lib/raspi-media-player/player.sqlite ".backup '/var/backups/raspi-media-player/player.sqlite'"
sudo cp -p /etc/default/raspi-media-player /var/backups/raspi-media-player/default
```

To restore, stop the service, preserve the current database as a rollback copy,
install the backup with service ownership, restore the configuration, and start
the service. Never copy a live SQLite database with plain `cp`.

```sh
sudo service raspi-media-player stop
sudo cp -p /var/lib/raspi-media-player/player.sqlite /var/lib/raspi-media-player/player.sqlite.before-restore
sudo install -o raspi-media-player -g raspi-media-player -m 0600 /var/backups/raspi-media-player/player.sqlite /var/lib/raspi-media-player/player.sqlite
sudo install -m 0644 /var/backups/raspi-media-player/default /etc/default/raspi-media-player
sudo service raspi-media-player start
```

## Uninstall

`sudo raspi-media-player-service uninstall` stops and disables the daemon and removes
the executable and init script. It deliberately preserves the database,
configuration, logs, and service account so a reinstall or manual recovery is
possible. Remove those preserved paths manually only after taking a backup.

## Fresh installation with preserved configuration

To return to the guided wizard while keeping operator configuration, first read
the actual database and image-cache paths from `/etc/default/raspi-media-player`.
Stop/uninstall the service, remove that database plus its exact `-wal` and `-shm`
sidecars, remove generated image cache contents, and deploy again. Do not remove
the defaults file or settings encryption key. Confirm
`RASPI_MEDIA_PLAYER_SETUP_REQUIRED=true` before restart.

After reinstalling, verify:

```sh
curl http://jukebox.local:8080/api/v1/setup/status
curl -o /dev/null -w '%{http_code}\n' http://jukebox.local:8080/api/v1/queue
```

Setup status must report `"installed":false`; the normal queue API must return
503 until the wizard creates the new administrator. This procedure permanently
erases accounts, sessions, queue, history, library, settings overrides, and
cached fair-rotation turns because all are stored in SQLite.

# Releases

GitHub Actions publishes two installable ARM64 release channels. Both run the
full test suite before publishing and contain the binary, init.d assets,
dependency installer, service manager, diagnostics, and documentation.

## Stable

Push a semantic version tag to publish a stable GitHub Release:

```sh
git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

The `Release` workflow embeds the tag, commit, and build time in the binary,
creates an archive and SHA-256 checksum file, generates release notes, and marks
the release as the latest stable version. Download it from the repository's
Releases page or `/releases/latest`.

## Bleeding edge

Every push to `main` runs the `Edge release` workflow. A successful run replaces
the rolling `edge` prerelease at `/releases/tag/edge`. The version includes the
build date and commit hash. Edge is explicitly a prerelease and never becomes
the latest stable release.

The rolling edge workflow moves the `edge` tag and replaces its assets. GitHub's
immutable releases setting must remain disabled for this one rolling release.
Stable version tags are never moved or replaced.

## Install a downloaded bundle

Verify the archive against `checksums.txt`, extract it, then install dependencies
and the service on a 64-bit Raspberry Pi:

```sh
sha256sum -c checksums.txt
tar -xzf raspi-media-player_VERSION_linux_arm64.tar.gz
cd raspi-media-player_VERSION_linux_arm64
sudo ./install-dependencies.sh
sudo ./service-manager.sh install "$PWD"
```

For an existing installation, use `upgrade` instead of `install`. Configuration
and the SQLite database are preserved.

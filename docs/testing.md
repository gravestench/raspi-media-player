# Regression testing

Run `make check` for Go tests and vetting, and `make test-api` for the complete
black-box API suite. The latter builds the current server, starts isolated
instances on loopback with temporary SQLite databases and the fake player, and
removes every process and temporary file on exit.

The focused scripts cover health/version, anonymous and concurrent queue
operations, accounts and sessions, CSRF, playback controls, stations,
favorites, playlists, search/history, malformed and oversized requests, all
three access modes, and restart persistence. They use POSIX shell, `curl`, and
`jq`; no credentials or live installation data are used or printed.

`TEST_PORT` changes the primary port (default `18080`). The auxiliary access
mode and persistence servers use `TEST_OPTIONAL_PORT`, `TEST_REQUIRED_PORT`,
and `TEST_PERSIST_PORT`, defaulting to `18081` through `18083`. Override these
when another local service owns those ports.

GitHub Actions runs formatting, vet, race-enabled Go tests, and this full API
suite on pushes and pull requests.

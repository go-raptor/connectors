# Changelog

Each connector is its own module, versioned by its own tag (`pgx/vX.Y.Z`, `bun/postgres/vX.Y.Z`).

## pgx — Unreleased (v1.2.0)

### Fixed

- The connection URL is built with `net/url`, so passwords containing `@ : / ? #`, spaces or quotes no longer break it.
- A Unix-socket `host` (e.g. `/var/run/postgresql`) is passed as a `host` query parameter instead of producing an invalid URL.
- A config field of the wrong type returns an error instead of panicking.
- An empty password is omitted from the URL, so `PGPASSWORD` and `~/.pgpass` apply.

### Added

- `sslmode` comes from Raptor's `database.ssl_mode` (raptor/v4 v4.4.0+, default `prefer`). When the config has no `SSLMode` field (older Raptor) or it is empty, the connector keeps `sslmode=disable`.

## bun/postgres — Unreleased (v1.2.0)

### Fixed

- The connection string is built as a `net/url` URL instead of a keyword/value DSN, so passwords containing spaces, quotes or other special characters no longer break it.
- A Unix-socket `host` (e.g. `/var/run/postgresql`) keeps working.
- A config field of the wrong type returns an error instead of panicking.
- An empty password is omitted from the URL, so `PGPASSWORD` and `~/.pgpass` apply.

### Added

- `sslmode` comes from Raptor's `database.ssl_mode` (raptor/v4 v4.4.0+, default `prefer`). When the config has no `SSLMode` field (older Raptor) or it is empty, the connector keeps `sslmode=disable`.

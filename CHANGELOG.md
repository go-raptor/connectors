# Changelog

Each connector is its own module, versioned by its own tag (`pgx/vX.Y.Z`, `bun/postgres/vX.Y.Z`).

## pgx — Unreleased (v1.2.0)

### Upgrading

- **If you percent-encoded credentials to work around the old DSN** (e.g. `DATABASE_PASSWORD=p%2Fss`), store the raw value now (`p/ss`). Every field is escaped for you, so an already-encoded value would be sent literally and authentication would fail. The same applies to a percent-encoded `username` or `name`.

### Fixed

- The connection URL is built with `net/url`, so passwords containing `@ : / ? #`, spaces or quotes no longer break it.
- A Unix-socket `host` (e.g. `/var/run/postgresql`) is passed as a `host` query parameter instead of producing an invalid URL.
- IPv6 hosts work bare (`::1`) or bracketed (`[::1]`, the form earlier releases required).
- A config field of the wrong type returns an error instead of panicking.
- An empty password is omitted from the URL, so `PGPASSWORD` and `~/.pgpass` apply.

### Added

- `sslmode` comes from Raptor's `database.ssl_mode` (raptor/v4 v4.4.0+, default `prefer`). When the config has no `SSLMode` field (older Raptor) or it is empty, the connector keeps `sslmode=disable`.

## bun/postgres — Unreleased (v1.2.0)

### Upgrading

- **If you backslash-escaped or quoted credentials to work around the old keyword/value DSN** (e.g. a password written as `p\ w`), store the raw value now (`p w`). Every field is escaped for you, so escape characters would be sent literally and authentication would fail.

### Fixed

- The connection string is built as a `net/url` URL instead of a keyword/value DSN, so passwords containing spaces, quotes or other special characters no longer break it.
- A Unix-socket `host` (e.g. `/var/run/postgresql`) keeps working.
- IPv6 hosts work bare (`::1`) or bracketed (`[::1]`).
- A config field of the wrong type returns an error instead of panicking.
- An empty password is omitted from the URL, so `PGPASSWORD` and `~/.pgpass` apply.

### Added

- `sslmode` comes from Raptor's `database.ssl_mode` (raptor/v4 v4.4.0+, default `prefer`). When the config has no `SSLMode` field (older Raptor) or it is empty, the connector keeps `sslmode=disable`.

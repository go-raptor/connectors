# Changelog

Each connector is its own module, versioned by its own tag (`pgx/vX.Y.Z`, `bun/postgres/vX.Y.Z`).

## pgx — Unreleased

### Added

- `Close`: Raptor's `Shutdown` now closes the pool, after in-flight requests drain and services stop. Before, every connection was dropped at process exit.
- `WithPoolConfig(func(*pgxpool.Config))` adjusts the pool before it opens: `MaxConns`, lifetimes, the connect timeout, or server settings such as `statement_timeout` through `ConnConfig.RuntimeParams`.

### Fixed

- An unreachable database blocked startup until the OS gave up on the TCP handshake, about two minutes on Linux. Connection attempts and the startup ping now time out after 10 seconds; `PGCONNECT_TIMEOUT` still overrides it.
- A failed `Init` closes the pool it opened instead of leaking it.

### Security

- Requires `golang.org/x/text` v0.42.0: v0.38.0 has an infinite loop on invalid input (GO-2026-5970), reachable from `stdlib.OpenDBFromPool`.

### Changed

- Requires pgx v5.11.0 and goose v3.28.0.

## bun/postgres — Unreleased

### Added

- `Close`: Raptor's `Shutdown` now closes the pool, after in-flight requests drain and services stop. Before, every connection was dropped at process exit.
- `WithPoolConfig(func(*pgxpool.Config))` adjusts the pool before it opens: `MaxConns`, lifetimes, the connect timeout, or server settings such as `statement_timeout` through `ConnConfig.RuntimeParams`.

### Fixed

- An unreachable database blocked startup until the OS gave up on the TCP handshake, about two minutes on Linux. Connection attempts and the startup ping now time out after 10 seconds; `PGCONNECT_TIMEOUT` still overrides it.
- A failed `Init` closes the pool it opened instead of leaking it.

### Security

- Requires `golang.org/x/text` v0.42.0: v0.38.0 has an infinite loop on invalid input (GO-2026-5970), reachable from `stdlib.OpenDBFromPool`.

### Changed

- Requires pgx v5.11.0 and goose v3.28.0.

## sqlite — Unreleased

### Upgrading

- Transactions begin `IMMEDIATE`: `BeginTx` takes the write lock at once and waits for it under the 5-second busy timeout. Explicit transactions, read-only ones included, now run one at a time. Queries outside a transaction are unaffected.

### Added

- `Close`: Raptor's `Shutdown` now closes the database, which checkpoints the WAL into the main file.

### Fixed

- A transaction that read before another one committed failed its write at once with `database is locked (517)`, which the busy timeout couldn't prevent. With `IMMEDIATE` transactions the second writer waits instead.
- Every `:memory:` connector shared one process-wide database, so two test apps in one binary saw each other's tables. Each connector now gets its own.
- A `database.name` with its own query string (`file:app.db?mode=rwc`) produced an invalid DSN. The connector's settings are now appended to it.
- A pointer to the database config failed with "config must be a struct".

### Changed

- Requires modernc.org/sqlite v1.60.1 and goose v3.28.0.

## bun/sqlite — Unreleased

### Upgrading

- Transactions begin `IMMEDIATE`: `BeginTx` takes the write lock at once and waits for it under the 5-second busy timeout. Explicit transactions, read-only ones included, now run one at a time. Queries outside a transaction are unaffected.

### Added

- `Close`: Raptor's `Shutdown` now closes the database, which checkpoints the WAL into the main file.

### Fixed

- A transaction that read before another one committed failed its write at once with `database is locked (517)`, which the busy timeout couldn't prevent. With `IMMEDIATE` transactions the second writer waits instead.
- Every `:memory:` connector shared one process-wide database, so two test apps in one binary saw each other's tables. Each connector now gets its own.
- A `database.name` with its own query string (`file:app.db?mode=rwc`) produced an invalid DSN. The connector's settings are now appended to it.
- A pointer to the database config failed with "config must be a struct".

### Changed

- Requires modernc.org/sqlite v1.60.1 and goose v3.28.0.

## pgx — v1.2.0 (2026-09-25)

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

## bun/postgres — v1.2.0 (2026-09-25)

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

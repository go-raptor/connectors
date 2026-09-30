# Changelog

Each connector is its own module, versioned by its own tag (`pgx/vX.Y.Z`, `bun/postgres/vX.Y.Z`).

## pgx — Unreleased

### Added

- `Ping(ctx context.Context) error` checks that the database is reachable. Raptor's readiness check (`Health.Ready`, raptor v4.6.0+) calls it; before `Init` it returns an error. With every connection in use it reports success rather than waiting for one, so a busy app isn't taken out of rotation.

## bun/postgres — Unreleased

### Added

- `Ping(ctx context.Context) error` checks that the database is reachable. Raptor's readiness check (`Health.Ready`, raptor v4.6.0+) calls it; before `Init` it returns an error. With every connection in use it reports success rather than waiting for one, so a busy app isn't taken out of rotation.

## sqlite — Unreleased

### Added

- `Ping(ctx context.Context) error` checks that the database is reachable. Raptor's readiness check (`Health.Ready`, raptor v4.6.0+) calls it; before `Init` it returns an error. With every connection in use it reports success rather than waiting for one.

## bun/sqlite — Unreleased

### Added

- `Ping(ctx context.Context) error` checks that the database is reachable. Raptor's readiness check (`Health.Ready`, raptor v4.6.0+) calls it; before `Init` it returns an error. With every connection in use it reports success rather than waiting for one.

## pgx — v1.3.0 (2026-09-30)

### Upgrading

- Every new pool connection, not only the one at startup, now gives up after 10 seconds. A database that takes longer to accept connections, such as a serverless one resuming from a pause, needs a larger `ConnectTimeout` through `WithPoolConfig` or `PGCONNECT_TIMEOUT`.
- `Shutdown` now closes the pool and waits up to 5 seconds for connections still in use; after that `Close` returns an error naming how many were not released.

### Added

- `Close`: Raptor's `Shutdown` now closes the pool, after in-flight requests drain and services stop, waiting up to 5 seconds for connections still in use. Before, every connection was dropped at process exit.
- `WithPoolConfig(func(*pgxpool.Config))` adjusts the pool before it opens: `MaxConns`, lifetimes, the connect timeout, or server settings such as `statement_timeout` through `ConnConfig.RuntimeParams`.

### Fixed

- An unreachable database blocked startup until the OS gave up on the TCP handshake, about two minutes on Linux. Connection attempts and the startup ping now time out after 10 seconds, with an error naming the address; `PGCONNECT_TIMEOUT` still overrides it.
- A failed `Init` closes the pool it opened instead of leaking it.

### Security

- Requires `golang.org/x/text` v0.42.0: v0.38.0 has an infinite loop on invalid input (GO-2026-5970), reachable from `stdlib.OpenDBFromPool`.

### Changed

- Requires pgx v5.11.0 and goose v3.28.0.

## bun/postgres — v1.3.0 (2026-09-30)

### Upgrading

- Every new pool connection, not only the one at startup, now gives up after 10 seconds. A database that takes longer to accept connections, such as a serverless one resuming from a pause, needs a larger `ConnectTimeout` through `WithPoolConfig` or `PGCONNECT_TIMEOUT`.
- `Shutdown` now closes the pool and waits up to 5 seconds for connections still in use; after that `Close` returns an error naming how many were not released.

### Added

- `Close`: Raptor's `Shutdown` now closes the pool, after in-flight requests drain and services stop, waiting up to 5 seconds for connections still in use. Before, every connection was dropped at process exit.
- `WithPoolConfig(func(*pgxpool.Config))` adjusts the pool before it opens: `MaxConns`, lifetimes, the connect timeout, or server settings such as `statement_timeout` through `ConnConfig.RuntimeParams`.

### Fixed

- An unreachable database blocked startup until the OS gave up on the TCP handshake, about two minutes on Linux. Connection attempts and the startup ping now time out after 10 seconds, with an error naming the address; `PGCONNECT_TIMEOUT` still overrides it.
- A failed `Init` closes the pool it opened instead of leaking it.

### Security

- Requires `golang.org/x/text` v0.42.0: v0.38.0 has an infinite loop on invalid input (GO-2026-5970), reachable from `stdlib.OpenDBFromPool`.

### Changed

- Requires pgx v5.11.0 and goose v3.28.0.

## sqlite — v1.1.0 (2026-09-30)

### Upgrading

- Transactions begin `IMMEDIATE`: a `BeginTx` without `ReadOnly` takes the write lock at once, waiting for it under the 5-second busy timeout. While such a transaction is open, every other write, inside a transaction or not, waits up to 5 seconds and then fails with `database is locked`, so a long transaction that only reads now stalls writers. Declare read-only work with `&sql.TxOptions{ReadOnly: true}` to keep it concurrent, or put `_txlock=deferred` in `database.name` to opt out.

### Added

- `Close`: Raptor's `Shutdown` now closes the database, which checkpoints the WAL into the main file.

### Fixed

- A transaction that read before another one committed failed its write at once with `database is locked (517)`, which the busy timeout couldn't prevent. With `IMMEDIATE` transactions the second writer waits instead.
- Every `:memory:` connector shared one process-wide database, so two test apps in one binary saw each other's tables. Each connector now gets its own.
- A `database.name` with its own query string (`file:app.db?mode=rwc`) produced an invalid DSN. The connector's settings are now appended to it.
- A pointer to the database config failed with "config must be a struct".

### Changed

- Requires modernc.org/sqlite v1.60.1 and goose v3.28.0.

## bun/sqlite — v1.1.0 (2026-09-30)

### Upgrading

- Transactions begin `IMMEDIATE`: a `BeginTx` without `ReadOnly` takes the write lock at once, waiting for it under the 5-second busy timeout. While such a transaction is open, every other write, inside a transaction or not, waits up to 5 seconds and then fails with `database is locked`, so a long transaction that only reads now stalls writers. Declare read-only work with `&sql.TxOptions{ReadOnly: true}` to keep it concurrent, or put `_txlock=deferred` in `database.name` to opt out.

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

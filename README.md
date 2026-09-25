![Raptor](https://static.husak.me/img/raptor/logo.png)

# Raptor connectors

Database connectors for the [Raptor](https://github.com/go-raptor/raptor) web framework. Each connector manages the connection pool and exposes a [Goose](https://github.com/pressly/goose) migrator; register one through `Components.DatabaseConnector`. Each is its own Go module.

| Module | Provides | Install |
| --- | --- | --- |
| `pgx` | `*pgxpool.Pool` (PostgreSQL via pgx) | `go get github.com/go-raptor/connectors/pgx` |
| `bun/postgres` | `*bun.DB` (PostgreSQL via the Bun ORM) | `go get github.com/go-raptor/connectors/bun/postgres` |
| `sqlite` | `*sql.DB` (SQLite, pure Go) | `go get github.com/go-raptor/connectors/sqlite` |
| `bun/sqlite` | `*bun.DB` (SQLite via the Bun ORM) | `go get github.com/go-raptor/connectors/bun/sqlite` |
| `goosemigrator` | The Goose-backed migrator the connectors above use | `go get github.com/go-raptor/connectors/goosemigrator` |

```go
import "github.com/go-raptor/connectors/bun/postgres"

func New() *raptor.Components {
	return &raptor.Components{
		DatabaseConnector: postgres.NewPostgresConnector(db.MigrationsFS()),
		// ...
	}
}
```

## Postgres configuration

Both Postgres connectors read Raptor's `database:` section:

```yaml
database:
  host: localhost
  port: 5432
  username: myapp
  name: myapp
  ssl_mode: prefer
```

- `password` comes from `DATABASE_PASSWORD`; keep it out of tracked files. An empty password is left out of the connection URL, so `PGPASSWORD` and `~/.pgpass` still apply.
- Every field is URL-escaped, so passwords may contain any character, `@ : / ? #`, spaces and quotes included.
- A `host` starting with `/` is a Unix-socket directory, e.g. `/var/run/postgresql`. IPv6 addresses such as `::1` work as-is.
- `ssl_mode` (`DATABASE_SSL_MODE`) is libpq's `sslmode`: `disable`, `prefer`, `require`, `verify-ca` or `verify-full`. Raptor v4.4.0+ defaults it to `prefer`. With an older Raptor, which has no such setting, the connectors keep `disable`, their previous hard-coded value. Use `verify-full` for managed databases.

## SQLite configuration

The SQLite connectors read only `database.name`, the path of the database file.

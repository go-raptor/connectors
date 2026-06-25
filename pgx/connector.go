package pgx

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"reflect"

	"github.com/go-raptor/connectors"
	"github.com/go-raptor/connectors/goosemigrator"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

type PgxConnector struct {
	config       interface{}
	pool         *pgxpool.Pool
	sqlDB        *sql.DB
	migrationsFS fs.FS
	migrator     connectors.Migrator
}

// NewPgxConnector returns a Postgres connector backed by pgx. The
// migrationsFS argument should be an fs.FS rooted at the directory holding
// the migration files (typically `fs.Sub(embedFS, "db/migrations")`). Pass
// nil if no SQL migrations are embedded; Go migrations registered via
// goose.AddMigration* still work.
func NewPgxConnector(migrationsFS fs.FS) connectors.DatabaseConnector {
	return &PgxConnector{
		migrationsFS: migrationsFS,
	}
}

func (c *PgxConnector) SetConfig(config interface{}) {
	c.config = config
}

func (c *PgxConnector) Conn() any {
	return c.pool
}

func (c *PgxConnector) Migrator() connectors.Migrator {
	return c.migrator
}

func (c *PgxConnector) Init() error {
	val := reflect.ValueOf(c.config)
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("config must be a struct")
	}

	hostField := val.FieldByName("Host")
	portField := val.FieldByName("Port")
	userField := val.FieldByName("Username")
	passwordField := val.FieldByName("Password")
	nameField := val.FieldByName("Name")

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		userField.Interface().(string),
		passwordField.Interface().(string),
		hostField.Interface().(string),
		portField.Interface().(int),
		nameField.Interface().(string),
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("failed to parse connection string: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	c.pool = pool
	c.sqlDB = stdlib.OpenDBFromPool(pool)

	// Postgres uses goose's advisory-lock session locker so concurrent app
	// instances don't run migrations simultaneously.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("failed to build session locker: %w", err)
	}

	migrator, err := goosemigrator.New(goose.DialectPostgres, c.sqlDB, c.migrationsFS, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("failed to build migrator: %w", err)
	}
	c.migrator = migrator

	return nil
}

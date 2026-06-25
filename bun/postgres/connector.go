package postgres

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
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

type PostgresConnector struct {
	config       any
	pool         *pgxpool.Pool
	sqlDB        *sql.DB
	conn         *bun.DB
	migrationsFS fs.FS
	migrator     connectors.Migrator
}

// NewPostgresConnector returns a Postgres connector backed by Bun. The
// migrationsFS argument should be an fs.FS rooted at the directory holding
// the migration files (typically `fs.Sub(embedFS, "db/migrations")`). Pass
// nil if no SQL migrations are embedded; Go migrations registered via
// goose.AddMigration* still work.
//
// Go migrations run against a raw *sql.Tx, not *bun.DB. To use Bun's query
// builder inside a Go migration, wrap the transaction:
//
//	bunTx := bun.NewTx(tx, pgdialect.New())
func NewPostgresConnector(migrationsFS fs.FS) connectors.DatabaseConnector {
	return &PostgresConnector{
		migrationsFS: migrationsFS,
	}
}

func (c *PostgresConnector) SetConfig(config any) {
	c.config = config
}

func (c *PostgresConnector) Conn() any {
	return c.conn
}

func (c *PostgresConnector) Migrator() connectors.Migrator {
	return c.migrator
}

func (c *PostgresConnector) Init() error {
	val := reflect.ValueOf(c.config)
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("input is not a struct")
	}

	hostField := val.FieldByName("Host")
	portField := val.FieldByName("Port")
	userField := val.FieldByName("Username")
	passwordField := val.FieldByName("Password")
	nameField := val.FieldByName("Name")

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable",
		hostField.Interface().(string),
		userField.Interface().(string),
		passwordField.Interface().(string),
		nameField.Interface().(string),
		portField.Interface().(int),
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("failed to parse DSN: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	c.pool = pool
	c.sqlDB = stdlib.OpenDBFromPool(pool)
	c.conn = bun.NewDB(c.sqlDB, pgdialect.New())

	if err := c.conn.PingContext(context.Background()); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

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

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"time"

	"github.com/go-raptor/connectors"
	"github.com/go-raptor/connectors/goosemigrator"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// defaultConnectTimeout bounds each connection attempt and the startup
// ping, so an unreachable database fails startup in seconds instead of
// after the OS gives up on the TCP handshake (about two minutes on Linux).
// PGCONNECT_TIMEOUT and WithPoolConfig override it.
const defaultConnectTimeout = 10 * time.Second

type PostgresConnector struct {
	config        any
	pool          *pgxpool.Pool
	sqlDB         *sql.DB
	conn          *bun.DB
	migrationsFS  fs.FS
	migrator      connectors.Migrator
	configurePool []func(*pgxpool.Config)
}

// Option configures a PostgresConnector.
type Option func(*PostgresConnector)

// WithPoolConfig adjusts the pool configuration built from Raptor's
// database config before the pool opens: MaxConns and the connection
// lifetimes, ConnConfig.ConnectTimeout, or server settings such as
// statement_timeout through ConnConfig.RuntimeParams. Several options
// apply in order.
func WithPoolConfig(configure func(*pgxpool.Config)) Option {
	return func(c *PostgresConnector) {
		c.configurePool = append(c.configurePool, configure)
	}
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
func NewPostgresConnector(migrationsFS fs.FS, opts ...Option) connectors.DatabaseConnector {
	c := &PostgresConnector{migrationsFS: migrationsFS}
	for _, opt := range opts {
		opt(c)
	}
	return c
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
	poolConfig, err := c.poolConfig()
	if err != nil {
		return err
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	c.pool = pool
	c.sqlDB = stdlib.OpenDBFromPool(pool)
	c.conn = bun.NewDB(c.sqlDB, pgdialect.New())

	if err := ping(c.pool, poolConfig.ConnConfig.ConnectTimeout); err != nil {
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

// poolConfig turns the Raptor database config into a pool config, with
// the default connect timeout and the WithPoolConfig options applied.
func (c *PostgresConnector) poolConfig() (*pgxpool.Config, error) {
	dsn, err := connString(c.config)
	if err != nil {
		return nil, err
	}
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}
	if poolConfig.ConnConfig.ConnectTimeout == 0 {
		poolConfig.ConnConfig.ConnectTimeout = defaultConnectTimeout
	}
	for _, configure := range c.configurePool {
		configure(poolConfig)
	}
	return poolConfig, nil
}

// ping checks that the database is reachable, bounded by timeout when one
// is set.
func ping(pool *pgxpool.Pool, timeout time.Duration) error {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return pool.Ping(ctx)
}

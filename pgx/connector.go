package pgx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/go-raptor/connectors"
	"github.com/go-raptor/connectors/goosemigrator"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// defaultConnectTimeout bounds each connection attempt and the startup
// ping, so an unreachable database fails startup in seconds instead of
// after the OS gives up on the TCP handshake (about two minutes on Linux).
// PGCONNECT_TIMEOUT and WithPoolConfig override it.
const defaultConnectTimeout = 10 * time.Second

type PgxConnector struct {
	config        interface{}
	pool          *pgxpool.Pool
	sqlDB         *sql.DB
	migrationsFS  fs.FS
	migrator      connectors.Migrator
	configurePool []func(*pgxpool.Config)
}

var _ io.Closer = (*PgxConnector)(nil)

// Option configures a PgxConnector.
type Option func(*PgxConnector)

// WithPoolConfig adjusts the pool configuration built from Raptor's
// database config before the pool opens: MaxConns and the connection
// lifetimes, ConnConfig.ConnectTimeout, or server settings such as
// statement_timeout through ConnConfig.RuntimeParams. Several options
// apply in order.
func WithPoolConfig(configure func(*pgxpool.Config)) Option {
	return func(c *PgxConnector) {
		c.configurePool = append(c.configurePool, configure)
	}
}

// NewPgxConnector returns a Postgres connector backed by pgx. The
// migrationsFS argument should be an fs.FS rooted at the directory holding
// the migration files (typically `fs.Sub(embedFS, "db/migrations")`). Pass
// nil if no SQL migrations are embedded; Go migrations registered via
// goose.AddMigration* still work.
func NewPgxConnector(migrationsFS fs.FS, opts ...Option) connectors.DatabaseConnector {
	c := &PgxConnector{migrationsFS: migrationsFS}
	for _, opt := range opts {
		opt(c)
	}
	return c
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
	poolConfig, err := c.poolConfig()
	if err != nil {
		return err
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	// Close what Init opened if it fails part-way, so a caller that
	// handles the error doesn't leak a pool.
	ready := false
	defer func() {
		if !ready {
			sqlDB.Close()
			pool.Close()
		}
	}()

	if err := ping(pool, poolConfig.ConnConfig.ConnectTimeout); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Postgres uses goose's advisory-lock session locker so concurrent app
	// instances don't run migrations simultaneously.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("failed to build session locker: %w", err)
	}

	migrator, err := goosemigrator.New(goose.DialectPostgres, sqlDB, c.migrationsFS, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("failed to build migrator: %w", err)
	}

	c.pool, c.sqlDB, c.migrator = pool, sqlDB, migrator
	ready = true
	return nil
}

// Close closes the database/sql handle used for migrations, then the pool,
// which waits up to closeTimeout for acquired connections to be released.
// Raptor calls it from Shutdown after requests have drained and services
// have stopped.
func (c *PgxConnector) Close() error {
	var err error
	if c.sqlDB != nil {
		err = c.sqlDB.Close()
	}
	if c.pool != nil && !closeWithin(c.pool.Close, closeTimeout) {
		err = errors.Join(err, fmt.Errorf("pool still closing after %v: %d connections not released", closeTimeout, c.pool.Stat().AcquiredConns()))
	}
	return err
}

// closeTimeout bounds how long Close waits for acquired connections to be
// released. A connection still held after it (a query on
// context.Background(), unclosed rows, a missing Release) would otherwise
// hold shutdown until the process is killed; the pool finishes closing in
// the background.
const closeTimeout = 5 * time.Second

// closeWithin runs closeFn and reports whether it returned within timeout.
// Past the timeout, closeFn keeps running in the background.
func closeWithin(closeFn func(), timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		closeFn()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// poolConfig turns the Raptor database config into a pool config, with
// the default connect timeout and the WithPoolConfig options applied.
func (c *PgxConnector) poolConfig() (*pgxpool.Config, error) {
	dsn, err := connString(c.config)
	if err != nil {
		return nil, err
	}
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
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

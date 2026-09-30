package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func testConfig() databaseConfig {
	return databaseConfig{Host: "localhost", Port: 5432, Username: "app", Name: "app", SSLMode: "disable"}
}

// Without a connect timeout, an unreachable database blocks startup until
// the OS gives up on the TCP handshake, about two minutes on Linux.
func TestPoolConfigBoundsConnect(t *testing.T) {
	c := NewPostgresConnector(nil).(*PostgresConnector)
	c.SetConfig(testConfig())
	cfg, err := c.poolConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.ConnectTimeout != defaultConnectTimeout {
		t.Fatalf("ConnectTimeout = %v, want %v", cfg.ConnConfig.ConnectTimeout, defaultConnectTimeout)
	}

	t.Setenv("PGCONNECT_TIMEOUT", "3")
	if cfg, err = c.poolConfig(); err != nil || cfg.ConnConfig.ConnectTimeout != 3*time.Second {
		t.Fatalf("PGCONNECT_TIMEOUT must win over the default: %v, %v", cfg.ConnConfig.ConnectTimeout, err)
	}
}

func TestWithPoolConfigAppliesInOrder(t *testing.T) {
	c := NewPostgresConnector(nil,
		WithPoolConfig(func(pc *pgxpool.Config) { pc.MaxConns = 20 }),
		WithPoolConfig(func(pc *pgxpool.Config) { pc.ConnConfig.RuntimeParams["statement_timeout"] = "5s" }),
	).(*PostgresConnector)
	c.SetConfig(testConfig())
	cfg, err := c.poolConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != 20 || cfg.ConnConfig.RuntimeParams["statement_timeout"] != "5s" {
		t.Fatalf("options not applied: MaxConns=%d statement_timeout=%q", cfg.MaxConns, cfg.ConnConfig.RuntimeParams["statement_timeout"])
	}
}

func TestInitFailsFastWhenUnreachable(t *testing.T) {
	c := NewPostgresConnector(nil, WithPoolConfig(func(pc *pgxpool.Config) {
		pc.ConnConfig.ConnectTimeout = 300 * time.Millisecond
	})).(*PostgresConnector)
	cfg := testConfig()
	cfg.Host = "10.255.255.1" // non-routable: SYNs go unanswered
	c.SetConfig(cfg)

	start := time.Now()
	if err := c.Init(); err == nil {
		t.Fatal("Init against an unreachable host must fail")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Init took %v; the connect timeout must bound it", elapsed)
	}
	if c.pool != nil || c.sqlDB != nil || c.conn != nil {
		t.Fatal("a failed Init must not keep a half-open pool")
	}
}

func TestCloseClosesPoolAndSQLHandle(t *testing.T) {
	c := NewPostgresConnector(nil).(*PostgresConnector)
	if err := c.Close(); err != nil {
		t.Fatalf("Close before Init: %v", err)
	}

	c.SetConfig(testConfig())
	cfg, err := c.poolConfig()
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg) // lazy: connects on first use
	if err != nil {
		t.Fatal(err)
	}
	c.pool, c.sqlDB = pool, stdlib.OpenDBFromPool(pool)
	c.conn = bun.NewDB(c.sqlDB, pgdialect.New())

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.conn.Ping(); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("the database/sql handle must be closed, Ping: %v", err)
	}
	if _, err := pool.Acquire(context.Background()); err == nil {
		t.Fatal("the pool must be closed")
	}
}

// A pool closes only after every acquired connection is released; a query
// on context.Background() or a missing Release would otherwise hold
// Raptor's shutdown until the process is killed.
func TestCloseWithinBoundsABlockedClose(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	start := time.Now()
	if closeWithin(func() { <-release }, 50*time.Millisecond) {
		t.Fatal("a close still blocked after the timeout must report false")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("closeWithin waited %v past a 50ms timeout", elapsed)
	}
	if !closeWithin(func() {}, time.Second) {
		t.Fatal("a close that returns in time must report true")
	}
}

// pgconn bounds each address it tries by ConnectTimeout and names the
// address in its error. An outer deadline equal to that timeout would
// expire first, hiding the host and skipping any fallback address.
func TestUnreachableErrorNamesHost(t *testing.T) {
	c := NewPostgresConnector(nil, WithPoolConfig(func(pc *pgxpool.Config) {
		pc.ConnConfig.ConnectTimeout = 300 * time.Millisecond
	})).(*PostgresConnector)
	cfg := testConfig()
	cfg.Host = "10.255.255.1"
	c.SetConfig(cfg)

	err := c.Init()
	if err == nil || !strings.Contains(err.Error(), "10.255.255.1") {
		t.Fatalf("the startup error must name the unreachable address: %v", err)
	}
}

// Raptor's readiness check (Health.Ready) calls Ping.
func TestPing(t *testing.T) {
	c := NewPostgresConnector(nil, WithPoolConfig(func(pc *pgxpool.Config) {
		pc.ConnConfig.ConnectTimeout = 300 * time.Millisecond
	})).(*PostgresConnector)
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("Ping before Init must be an error, not a panic")
	}

	cfg := testConfig()
	cfg.Host = "10.255.255.1"
	c.SetConfig(cfg)
	poolConfig, err := c.poolConfig()
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig) // lazy
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	c.pool = pool
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("Ping to an unreachable host must fail")
	}
}

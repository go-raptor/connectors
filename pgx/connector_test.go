package pgx

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testConfig() databaseConfig {
	return databaseConfig{Host: "localhost", Port: 5432, Username: "app", Name: "app", SSLMode: "disable"}
}

// Without a connect timeout, an unreachable database blocks startup until
// the OS gives up on the TCP handshake, about two minutes on Linux.
func TestPoolConfigBoundsConnect(t *testing.T) {
	c := NewPgxConnector(nil).(*PgxConnector)
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
	c := NewPgxConnector(nil,
		WithPoolConfig(func(pc *pgxpool.Config) { pc.MaxConns = 20 }),
		WithPoolConfig(func(pc *pgxpool.Config) { pc.ConnConfig.RuntimeParams["statement_timeout"] = "5s" }),
	).(*PgxConnector)
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
	c := NewPgxConnector(nil, WithPoolConfig(func(pc *pgxpool.Config) {
		pc.ConnConfig.ConnectTimeout = 300 * time.Millisecond
	})).(*PgxConnector)
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
}

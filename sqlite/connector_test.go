package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/go-raptor/connectors"
)

// testConfig mirrors the field names the connector reflects on. SQLite only
// uses Name (the database file path); the rest exist to prove they're ignored.
type testConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	Name     string
}

var testMigrations = fstest.MapFS{
	"00001_create_widgets.sql": &fstest.MapFile{
		Data: []byte(`-- +goose Up
CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT NOT NULL);

-- +goose Down
DROP TABLE widgets;
`),
	},
}

func newTestConnector(t *testing.T) connectors.DatabaseConnector {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	c := NewSQLiteConnector(testMigrations)
	c.SetConfig(testConfig{Name: dbPath})
	if err := c.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	return c
}

func TestConnReturnsSQLDB(t *testing.T) {
	c := newTestConnector(t)
	if _, ok := c.Conn().(*sql.DB); !ok {
		t.Fatalf("Conn() = %T, want *sql.DB", c.Conn())
	}
}

func TestMigratorAppliesMigrations(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()

	results, err := c.Migrator().Up(ctx)
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Up() applied %d migrations, want 1", len(results))
	}

	version, err := c.Migrator().Version(ctx)
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if version != 1 {
		t.Fatalf("Version() = %d, want 1", version)
	}

	// The migrated table must exist and be usable (proves the SQLite dialect
	// and the foreign_keys pragma DSN didn't break basic DML).
	db := c.Conn().(*sql.DB)
	if _, err := db.ExecContext(ctx, `INSERT INTO widgets (name) VALUES ('gear')`); err != nil {
		t.Fatalf("insert into migrated table: %v", err)
	}
}

func TestInMemoryDatabase(t *testing.T) {
	c := NewSQLiteConnector(testMigrations)
	c.SetConfig(testConfig{Name: ":memory:"})
	if err := c.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	ctx := context.Background()

	if _, err := c.Migrator().Up(ctx); err != nil {
		t.Fatalf("Up() error = %v", err)
	}

	// The migration ran on one pooled connection; with a shared cache and a
	// single connection it must be visible to subsequent queries.
	db := c.Conn().(*sql.DB)
	if _, err := db.ExecContext(ctx, `INSERT INTO widgets (name) VALUES ('gear')`); err != nil {
		t.Fatalf("insert into in-memory migrated table: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM widgets`).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 1 {
		t.Fatalf("widgets count = %d, want 1", n)
	}
}

func TestInitRejectsNonStructConfig(t *testing.T) {
	c := NewSQLiteConnector(nil)
	c.SetConfig("not-a-struct")
	if err := c.Init(); err == nil {
		t.Fatal("Init() with non-struct config = nil, want error")
	}
}

func TestInitRejectsEmptyName(t *testing.T) {
	c := NewSQLiteConnector(nil)
	c.SetConfig(testConfig{Name: ""})
	if err := c.Init(); err == nil {
		t.Fatal("Init() with empty Name = nil, want error")
	}
}

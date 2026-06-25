package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/go-raptor/connectors"
	"github.com/uptrace/bun"
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

type widget struct {
	bun.BaseModel `bun:"table:widgets"`
	ID            int64  `bun:"id,pk,autoincrement"`
	Name          string `bun:"name,notnull"`
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

func TestConnReturnsBunDB(t *testing.T) {
	c := newTestConnector(t)
	if _, ok := c.Conn().(*bun.DB); !ok {
		t.Fatalf("Conn() = %T, want *bun.DB", c.Conn())
	}
}

func TestMigratorAndBunQuery(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()

	if _, err := c.Migrator().Up(ctx); err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	version, err := c.Migrator().Version(ctx)
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if version != 1 {
		t.Fatalf("Version() = %d, want 1", version)
	}

	// Exercise the bun query builder against the migrated table to prove the
	// SQLite dialect is wired correctly.
	db := c.Conn().(*bun.DB)
	if _, err := db.NewInsert().Model(&widget{Name: "gear"}).Exec(ctx); err != nil {
		t.Fatalf("bun insert: %v", err)
	}

	var got widget
	if err := db.NewSelect().Model(&got).Where("name = ?", "gear").Scan(ctx); err != nil {
		t.Fatalf("bun select: %v", err)
	}
	if got.Name != "gear" {
		t.Fatalf("got name = %q, want %q", got.Name, "gear")
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
	// single connection it must be visible to subsequent bun queries.
	db := c.Conn().(*bun.DB)
	if _, err := db.NewInsert().Model(&widget{Name: "gear"}).Exec(ctx); err != nil {
		t.Fatalf("bun insert into in-memory table: %v", err)
	}
	count, err := db.NewSelect().Model((*widget)(nil)).Count(ctx)
	if err != nil {
		t.Fatalf("bun count: %v", err)
	}
	if count != 1 {
		t.Fatalf("widgets count = %d, want 1", count)
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

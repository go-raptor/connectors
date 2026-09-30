package sqlite

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type dbConfig struct{ Name string }

func open(t *testing.T, config any) *SQLiteConnector {
	t.Helper()
	c := NewSQLiteConnector(nil).(*SQLiteConnector)
	c.SetConfig(config)
	if err := c.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// Close lets SQLite checkpoint the WAL into the database file; without it
// Raptor's Shutdown left the database open and the -wal file behind.
func TestCloseCheckpointsAndClosesWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	c := NewSQLiteConnector(nil).(*SQLiteConnector)
	c.SetConfig(dbConfig{Name: path})
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.sqlDB.Exec(`CREATE TABLE t (x int); INSERT INTO t VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + "-wal"); err != nil {
		t.Fatalf("an open WAL database keeps a -wal file: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path + "-wal"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Close must checkpoint and remove the -wal file, stat: %v", err)
	}
	if err := c.sqlDB.Ping(); err == nil {
		t.Fatal("the database must be closed")
	}
}

func TestCloseBeforeInit(t *testing.T) {
	if err := NewSQLiteConnector(nil).(*SQLiteConnector).Close(); err != nil {
		t.Fatalf("Close before Init: %v", err)
	}
}

func TestPointerConfig(t *testing.T) {
	open(t, &dbConfig{Name: filepath.Join(t.TempDir(), "p.db")})
}

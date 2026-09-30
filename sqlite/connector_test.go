package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if _, err := c.db.Exec(`CREATE TABLE t (x int); INSERT INTO t VALUES (1)`); err != nil {
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
	if err := c.db.Ping(); err == nil {
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

// Go's BEGIN is deferred: a transaction that read before another one
// committed fails its write at once with SQLITE_BUSY_SNAPSHOT (517), and
// busy_timeout can't help. BEGIN IMMEDIATE takes the write lock up front,
// so the second transaction waits instead.
func TestConcurrentWriteTransactionsWait(t *testing.T) {
	db := open(t, dbConfig{Name: filepath.Join(t.TempDir(), "tx.db")}).db
	if _, err := db.Exec(`CREATE TABLE counter (n int); INSERT INTO counter VALUES (0)`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	tx1, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := tx1.QueryRowContext(ctx, `SELECT n FROM counter`).Scan(&n); err != nil {
		t.Fatal(err)
	}

	second := make(chan error, 1)
	go func() {
		tx2, err := db.BeginTx(ctx, nil)
		if err != nil {
			second <- err
			return
		}
		var m int
		if err := tx2.QueryRowContext(ctx, `SELECT n FROM counter`).Scan(&m); err != nil {
			tx2.Rollback()
			second <- err
			return
		}
		if _, err := tx2.ExecContext(ctx, `UPDATE counter SET n = ?`, m+1); err != nil {
			tx2.Rollback()
			second <- err
			return
		}
		second <- tx2.Commit()
	}()
	time.Sleep(100 * time.Millisecond) // let the second transaction start

	if _, err := tx1.ExecContext(ctx, `UPDATE counter SET n = ?`, n+1); err != nil {
		t.Fatalf("first transaction: %v", err)
	}
	if err := tx1.Commit(); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("second transaction: %v", err)
	}
	if err := db.QueryRow(`SELECT n FROM counter`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("counter = %d (%v), want 2: an increment was lost", n, err)
	}
}

func TestMemoryDatabasesAreSeparate(t *testing.T) {
	a := open(t, dbConfig{Name: ":memory:"}).db
	b := open(t, dbConfig{Name: ":memory:"}).db
	if _, err := a.Exec(`CREATE TABLE only_in_a (x int); INSERT INTO only_in_a VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := b.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'only_in_a'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("two in-memory connectors share one database (count %d, %v)", n, err)
	}
	if err := a.QueryRow(`SELECT x FROM only_in_a`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("an in-memory database must keep its data between queries: %d, %v", n, err)
	}
}

func TestNameWithQueryKeepsPragmas(t *testing.T) {
	db := open(t, dbConfig{Name: "file:" + filepath.Join(t.TempDir(), "q.db") + "?mode=rwc"}).db
	var fk, busy int
	var journal string
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if fk != 1 || busy != 5000 || journal != "wal" {
		t.Fatalf("pragmas lost: foreign_keys=%d busy_timeout=%d journal_mode=%s", fk, busy, journal)
	}
}

// Each SQLite package numbers its own in-memory databases, so the names
// must differ by package: a sqlite and a bun/sqlite connector in one
// process would otherwise open the same database.
func TestMemoryDatabaseNameIsPackageSpecific(t *testing.T) {
	if dsn := buildDSN(":memory:"); !strings.HasPrefix(dsn, "file:raptor-sqlite-memory-") {
		t.Fatalf("in-memory DSN %q must start with file:raptor-sqlite-memory-", dsn)
	}
}

// modernc begins a ReadOnly transaction with a plain deferred BEGIN, which
// keeps read-only work concurrent under _txlock=immediate. The README tells
// users to rely on it, so pin it against driver changes.
func TestReadOnlyTransactionsStayConcurrent(t *testing.T) {
	db := open(t, dbConfig{Name: filepath.Join(t.TempDir(), "ro.db")}).db
	if _, err := db.Exec(`CREATE TABLE t (x int); INSERT INTO t VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	writer, err := db.BeginTx(ctx, nil) // takes the write lock
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()

	start := time.Now()
	reader, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("read-only BeginTx while a writer is open: %v", err)
	}
	defer reader.Rollback()
	var x int
	if err := reader.QueryRowContext(ctx, `SELECT x FROM t`).Scan(&x); err != nil || x != 1 {
		t.Fatalf("read-only query: %d, %v", x, err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("a read-only transaction waited %v for the writer", elapsed)
	}
}

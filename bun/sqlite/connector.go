package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"sync/atomic"

	"github.com/go-raptor/connectors"
	"github.com/go-raptor/connectors/goosemigrator"
	"github.com/pressly/goose/v3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	_ "modernc.org/sqlite"
)

var _ io.Closer = (*SQLiteConnector)(nil)

type SQLiteConnector struct {
	config       any
	sqlDB        *sql.DB
	conn         *bun.DB
	migrationsFS fs.FS
	migrator     connectors.Migrator
}

// NewSQLiteConnector returns a SQLite connector backed by Bun, using the
// pure-Go modernc.org/sqlite driver. The migrationsFS argument should be an
// fs.FS rooted at the directory holding the migration files (typically
// `fs.Sub(embedFS, "db/migrations")`). Pass nil if no SQL migrations are
// embedded; Go migrations registered via goose.AddMigration* still work.
//
// Go migrations run against a raw *sql.Tx, not *bun.DB. To use Bun's query
// builder inside a Go migration, wrap the transaction:
//
//	bunTx := bun.NewTx(tx, sqlitedialect.New())
func NewSQLiteConnector(migrationsFS fs.FS) connectors.DatabaseConnector {
	return &SQLiteConnector{
		migrationsFS: migrationsFS,
	}
}

func (c *SQLiteConnector) SetConfig(config any) {
	c.config = config
}

func (c *SQLiteConnector) Conn() any {
	return c.conn
}

func (c *SQLiteConnector) Migrator() connectors.Migrator {
	return c.migrator
}

func (c *SQLiteConnector) Init() error {
	val := reflect.Indirect(reflect.ValueOf(c.config))
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("config must be a struct")
	}

	nameField := val.FieldByName("Name")
	if !nameField.IsValid() {
		return fmt.Errorf("config missing Name field")
	}
	name, ok := nameField.Interface().(string)
	if !ok {
		return fmt.Errorf("config Name field must be a string")
	}
	if name == "" {
		return fmt.Errorf("database name (file path) must not be empty")
	}

	db, err := sql.Open("sqlite", buildDSN(name))
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if isMemory(name) {
		// A named in-memory database is dropped when its last connection
		// closes, so keep exactly one connection alive.
		db.SetMaxOpenConns(1)
	}

	c.sqlDB = db
	c.conn = bun.NewDB(c.sqlDB, sqlitedialect.New())

	if err := c.conn.PingContext(context.Background()); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	migrator, err := goosemigrator.New(goose.DialectSQLite3, c.sqlDB, c.migrationsFS)
	if err != nil {
		return fmt.Errorf("failed to build migrator: %w", err)
	}
	c.migrator = migrator

	return nil
}

// Close closes the Bun handle and the database beneath it. For a file
// database, the last connection to close checkpoints the WAL into the main
// file and removes it. Raptor calls Close from Shutdown after requests have
// drained and services have stopped.
func (c *SQLiteConnector) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// isMemory reports whether name refers to an in-memory SQLite database.
func isMemory(name string) bool {
	return name == ":memory:" || strings.Contains(name, ":memory:") || strings.Contains(name, "mode=memory")
}

// dsnOptions are the per-connection settings every file database gets:
// wait up to 5s for a lock, enforce foreign keys, journal in WAL mode, and
// begin transactions IMMEDIATE. Go's default deferred BEGIN makes a
// transaction that read before another one committed fail its write at once
// (SQLITE_BUSY_SNAPSHOT), which no busy timeout can wait out.
const dsnOptions = "_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_txlock=immediate"

// memoryDatabases numbers in-memory databases. Each connector gets its own
// named one: SQLite shares a named in-memory database between connections,
// which keeps it alive, but a single unnamed shared one would leak tables
// between connectors, such as two test apps in one process.
var memoryDatabases atomic.Uint64

// buildDSN turns a configured database name into a modernc.org/sqlite DSN.
// Settings are appended to a query string the name already has.
func buildDSN(name string) string {
	if isMemory(name) {
		return fmt.Sprintf("file:raptor-memory-%d?mode=memory&cache=shared&_pragma=foreign_keys(ON)&_txlock=immediate", memoryDatabases.Add(1))
	}
	if strings.Contains(name, "?") {
		return name + "&" + dsnOptions
	}
	return name + "?" + dsnOptions
}

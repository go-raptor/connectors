package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"reflect"
	"strings"

	"github.com/go-raptor/connectors"
	"github.com/go-raptor/connectors/goosemigrator"
	"github.com/pressly/goose/v3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	_ "modernc.org/sqlite"
)

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
	val := reflect.ValueOf(c.config)
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
		// A shared-cache in-memory database is dropped when its last
		// connection closes, so keep exactly one connection alive.
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

// isMemory reports whether name refers to an in-memory SQLite database.
func isMemory(name string) bool {
	return name == ":memory:" || strings.Contains(name, ":memory:") || strings.Contains(name, "mode=memory")
}

// buildDSN turns a configured database name into a modernc.org/sqlite DSN with
// sensible defaults (foreign keys on; for file databases also a busy timeout
// and WAL journaling). Pragmas are applied per connection.
func buildDSN(name string) string {
	if isMemory(name) {
		return "file::memory:?cache=shared&_pragma=foreign_keys(ON)"
	}
	return name + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)"
}

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
	_ "modernc.org/sqlite"
)

type SQLiteConnector struct {
	config       any
	db           *sql.DB
	migrationsFS fs.FS
	migrator     connectors.Migrator
}

// NewSQLiteConnector returns a SQLite connector backed by the pure-Go
// modernc.org/sqlite driver. The migrationsFS argument should be an fs.FS
// rooted at the directory holding the migration files (typically
// `fs.Sub(embedFS, "db/migrations")`). Pass nil if no SQL migrations are
// embedded; Go migrations registered via goose.AddMigration* still work.
func NewSQLiteConnector(migrationsFS fs.FS) connectors.DatabaseConnector {
	return &SQLiteConnector{
		migrationsFS: migrationsFS,
	}
}

func (c *SQLiteConnector) SetConfig(config any) {
	c.config = config
}

func (c *SQLiteConnector) Conn() any {
	return c.db
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
		db.SetMaxOpenConns(1)
	}

	if err := db.PingContext(context.Background()); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	c.db = db

	migrator, err := goosemigrator.New(goose.DialectSQLite3, c.db, c.migrationsFS)
	if err != nil {
		return fmt.Errorf("failed to build migrator: %w", err)
	}
	c.migrator = migrator

	return nil
}

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

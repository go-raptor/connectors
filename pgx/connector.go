package pgx

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-raptor/connectors"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PgxConnector struct {
	config     interface{}
	pool       *pgxpool.Pool
	migrator   connectors.Migrator
	migrations Migrations
}

func NewPgxConnector(config interface{}, migrations Migrations) connectors.DatabaseConnector {
	return &PgxConnector{
		config:     config,
		migrations: migrations,
	}
}

func (c *PgxConnector) Conn() any {
	return c.pool
}

func (c *PgxConnector) Migrator() connectors.Migrator {
	return c.migrator
}

func (c *PgxConnector) Init() error {
	val := reflect.ValueOf(c.config)

	if val.Kind() != reflect.Struct {
		return fmt.Errorf("config must be a struct")
	}

	hostField := val.FieldByName("Host")
	portField := val.FieldByName("Port")
	userField := val.FieldByName("Username")
	passwordField := val.FieldByName("Password")
	nameField := val.FieldByName("Name")

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		userField.Interface().(string),
		passwordField.Interface().(string),
		hostField.Interface().(string),
		portField.Interface().(int),
		nameField.Interface().(string),
	)

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("failed to parse connection string: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	c.pool = pool

	migrator := NewPgxMigrator(pool)
	for version, migration := range c.migrations {
		migrator.AddMigration(version, migration)
	}
	c.migrator = migrator

	if err := c.migrator.Up(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

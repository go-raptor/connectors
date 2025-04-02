package postgres

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-raptor/connectors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

type PostgresConnector struct {
	migrations Migrations
	config     interface{}
	conn       *bun.DB
	migrator   *PostgresMigrator
}

func NewPostgresConnector(config interface{}, migrations Migrations) connectors.DatabaseConnector {
	return &PostgresConnector{
		config:     config,
		migrations: migrations,
	}
}

func (c *PostgresConnector) Conn() any {
	return c.conn
}

func (c *PostgresConnector) Migrator() connectors.Migrator {
	return c.migrator
}

func (c *PostgresConnector) Init() error {
	val := reflect.ValueOf(c.config)

	if val.Kind() != reflect.Struct {
		return fmt.Errorf("input is not a struct")
	}

	hostField := val.FieldByName("Host")
	portField := val.FieldByName("Port")
	userField := val.FieldByName("Username")
	passwordField := val.FieldByName("Password")
	nameField := val.FieldByName("Name")

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable",
		hostField.Interface().(string),
		userField.Interface().(string),
		passwordField.Interface().(string),
		nameField.Interface().(string),
		portField.Interface().(int),
	)

	configPgxPool, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("failed to parse DSN: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), configPgxPool)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	sqldb := stdlib.OpenDBFromPool(pool)
	db := bun.NewDB(sqldb, pgdialect.New())

	if err := db.PingContext(context.Background()); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	c.conn = db

	c.migrator = NewPostgresMigrator(c.conn, c.migrations)

	if err := c.migrator.Up(); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	return nil
}

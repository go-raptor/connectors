package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/go-raptor/connectors"
	"github.com/uptrace/bun"
)

type Migration interface {
	Up(db *bun.DB) error
	Down(db *bun.DB) error
	Name() string
}

type Migrator interface {
	Up() error
	Down() error
	UpTo(version string) error
	DownTo(version string) error
	Status() ([]MigrationStatus, error)
	Version() (string, error)
}

type MigrationStatus struct {
	Version    string
	Name       string
	ExecutedAt *time.Time
	IsApplied  bool
}

type Migrations map[string]Migration

type SchemaMigration struct {
	Version    string    `bun:"version,pk"`
	Name       string    `bun:"name,notnull"`
	ExecutedAt time.Time `bun:"executed_at,notnull"`
}

func (SchemaMigration) TableName() string {
	return "schema_migrations"
}

type PostgresMigrator struct {
	db         *bun.DB
	migrations Migrations
}

func NewPostgresMigrator(db *bun.DB, migrations Migrations) *PostgresMigrator {
	return &PostgresMigrator{
		db:         db,
		migrations: migrations,
	}
}

func (pm *PostgresMigrator) createMigrationsTable() error {
	_, err := pm.db.NewCreateTable().
		Model((*SchemaMigration)(nil)).
		IfNotExists().
		Exec(context.Background())
	return err
}

func (pm *PostgresMigrator) Up() error {
	if err := pm.createMigrationsTable(); err != nil {
		return err
	}

	var executed []string
	err := pm.db.NewSelect().
		Model((*SchemaMigration)(nil)).
		Column("version").
		Order("version ASC").
		Scan(context.Background(), &executed)
	if err != nil {
		return err
	}

	pending := make([]string, 0)
	for version := range pm.migrations {
		isExecuted := false
		for _, ex := range executed {
			if version == ex {
				isExecuted = true
				break
			}
		}
		if !isExecuted {
			pending = append(pending, version)
		}
	}
	for _, version := range pending {
		migration := pm.migrations[version]

		tx, err := pm.db.BeginTx(context.Background(), nil)
		if err != nil {
			return err
		}

		if err := migration.Up(pm.db); err != nil {
			tx.Rollback()
			return err
		}

		_, err = tx.NewInsert().
			Model(&SchemaMigration{
				Version:    version,
				Name:       migration.Name(),
				ExecutedAt: time.Now(),
			}).
			Exec(context.Background())

		if err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func (pm *PostgresMigrator) Down() error {
	return fmt.Errorf("not implemented")
}

func (pm *PostgresMigrator) UpTo(version string) error {
	return fmt.Errorf("not implemented")
}

func (pm *PostgresMigrator) DownTo(version string) error {
	return fmt.Errorf("not implemented")
}

func (pm *PostgresMigrator) Status() ([]connectors.MigrationStatus, error) {
	return nil, fmt.Errorf("not implemented")
}

func (pm *PostgresMigrator) Version() (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (pm *PostgresMigrator) Create(name string) error {
	return fmt.Errorf("not implemented")
}

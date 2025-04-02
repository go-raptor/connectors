package pgx

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/go-raptor/connectors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Migration interface {
	Up(tx pgx.Tx) error
	Down(tx pgx.Tx) error
	Name() string
}

type Migrations map[string]Migration

type PgxMigrator struct {
	pool       *pgxpool.Pool
	migrations Migrations
}

type SchemaMigration struct {
	Version    string
	Name       string
	ExecutedAt time.Time
}

func NewPgxMigrator(pool *pgxpool.Pool) *PgxMigrator {
	return &PgxMigrator{
		pool:       pool,
		migrations: make(Migrations),
	}
}

func (m *PgxMigrator) AddMigration(version string, migration Migration) {
	m.migrations[version] = migration
}

func (m *PgxMigrator) createMigrationsTable(ctx context.Context) error {
	sql := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`

	_, err := m.pool.Exec(ctx, sql)
	return err
}

func (m *PgxMigrator) Up() error {
	ctx := context.Background()

	if err := m.createMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	rows, err := m.pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version ASC")
	if err != nil {
		return fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	executed := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("failed to scan migration version: %w", err)
		}
		executed[version] = true
	}

	var pending []string
	for version := range m.migrations {
		if !executed[version] {
			pending = append(pending, version)
		}
	}
	sort.Strings(pending)

	for _, version := range pending {
		migration := m.migrations[version]

		tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}

		if err := migration.Up(tx); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("failed to execute migration %s: %w", version, err)
		}

		_, err = tx.Exec(ctx,
			"INSERT INTO schema_migrations (version, name, executed_at) VALUES ($1, $2, $3)",
			version, migration.Name(), time.Now(),
		)
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("failed to record migration %s: %w", version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", version, err)
		}
	}

	return nil
}

func (m *PgxMigrator) Down() error {
	ctx := context.Background()

	rows, err := m.pool.Query(ctx,
		"SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1",
	)
	if err != nil {
		return fmt.Errorf("failed to query last migration: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil
	}

	var version string
	if err := rows.Scan(&version); err != nil {
		return fmt.Errorf("failed to scan migration version: %w", err)
	}

	migration, exists := m.migrations[version]
	if !exists {
		return fmt.Errorf("migration %s not found", version)
	}

	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	if err := migration.Down(tx); err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("failed to roll back migration %s: %w", version, err)
	}

	_, err = tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", version)
	if err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("failed to delete migration record %s: %w", version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit rollback of migration %s: %w", version, err)
	}

	return nil
}

func (m *PgxMigrator) UpTo(version string) error {
	ctx := context.Background()

	if err := m.createMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	var versions []string
	for v := range m.migrations {
		if v <= version {
			versions = append(versions, v)
		}
	}
	sort.Strings(versions)

	for _, v := range versions {
		migration := m.migrations[v]
		tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}

		if err := migration.Up(tx); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("failed to execute migration %s: %w", v, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", v, err)
		}
	}

	return nil
}

func (m *PgxMigrator) DownTo(version string) error {
	ctx := context.Background()

	rows, err := m.pool.Query(ctx,
		"SELECT version FROM schema_migrations WHERE version > $1 ORDER BY version DESC",
		version,
	)
	if err != nil {
		return fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("failed to scan migration version: %w", err)
		}
		versions = append(versions, v)
	}

	for _, v := range versions {
		migration := m.migrations[v]
		tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("failed to start transaction: %w", err)
		}

		if err := migration.Down(tx); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("failed to roll back migration %s: %w", v, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit rollback of migration %s: %w", v, err)
		}
	}

	return nil
}

func (m *PgxMigrator) Status() ([]connectors.MigrationStatus, error) {
	ctx := context.Background()

	rows, err := m.pool.Query(ctx,
		"SELECT version, name, executed_at FROM schema_migrations ORDER BY version ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	executed := make(map[string]*connectors.MigrationStatus)
	for rows.Next() {
		var status connectors.MigrationStatus
		var executedAt time.Time
		if err := rows.Scan(&status.Version, &status.Name, &executedAt); err != nil {
			return nil, fmt.Errorf("failed to scan migration status: %w", err)
		}
		status.ExecutedAt = &executedAt
		status.IsApplied = true
		executed[status.Version] = &status
	}

	var statuses []connectors.MigrationStatus
	for version, migration := range m.migrations {
		if status, exists := executed[version]; exists {
			statuses = append(statuses, *status)
		} else {
			statuses = append(statuses, connectors.MigrationStatus{
				Version:    version,
				Name:       migration.Name(),
				ExecutedAt: nil,
				IsApplied:  false,
			})
		}
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Version < statuses[j].Version
	})

	return statuses, nil
}

func (m *PgxMigrator) Version() (string, error) {
	ctx := context.Background()

	var version string
	err := m.pool.QueryRow(ctx,
		"SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1",
	).Scan(&version)

	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query current version: %w", err)
	}

	return version, nil
}

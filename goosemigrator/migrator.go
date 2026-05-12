// Package goosemigrator wraps github.com/pressly/goose/v3 to implement the
// connectors.Migrator interface. It is consumed by Raptor's connector
// implementations (such as connectors/pgx and connectors/bun/postgres)
// and lives in its own Go module so that the top-level connectors module
// stays goose-free for Raptor apps that don't use a database.
package goosemigrator

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/go-raptor/connectors"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

type Migrator struct {
	provider *goose.Provider
}

func New(db *sql.DB, fsys fs.FS, log *slog.Logger) (*Migrator, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("goosemigrator: build session locker: %w", err)
	}

	opts := []goose.ProviderOption{
		goose.WithSessionLocker(locker),
	}
	if log != nil {
		opts = append(opts, goose.WithSlog(log))
	}

	p, err := goose.NewProvider(goose.DialectPostgres, db, fsys, opts...)
	if err != nil {
		return nil, fmt.Errorf("goosemigrator: new provider: %w", err)
	}

	return &Migrator{provider: p}, nil
}

func (m *Migrator) Up(ctx context.Context) ([]connectors.MigrationResult, error) {
	results, err := m.provider.Up(ctx)
	return convertResults(results), err
}

func (m *Migrator) UpByOne(ctx context.Context) (*connectors.MigrationResult, error) {
	result, err := m.provider.UpByOne(ctx)
	return convertResult(result), err
}

func (m *Migrator) UpTo(ctx context.Context, version int64) ([]connectors.MigrationResult, error) {
	results, err := m.provider.UpTo(ctx, version)
	return convertResults(results), err
}

func (m *Migrator) Down(ctx context.Context) (*connectors.MigrationResult, error) {
	result, err := m.provider.Down(ctx)
	return convertResult(result), err
}

func (m *Migrator) DownTo(ctx context.Context, version int64) ([]connectors.MigrationResult, error) {
	results, err := m.provider.DownTo(ctx, version)
	return convertResults(results), err
}

func (m *Migrator) Redo(ctx context.Context) (*connectors.MigrationResult, error) {
	if _, err := m.provider.Down(ctx); err != nil {
		return nil, fmt.Errorf("goosemigrator: redo down: %w", err)
	}
	result, err := m.provider.UpByOne(ctx)
	if err != nil {
		return nil, fmt.Errorf("goosemigrator: redo up: %w", err)
	}
	return convertResult(result), nil
}

func (m *Migrator) Reset(ctx context.Context) ([]connectors.MigrationResult, error) {
	results, err := m.provider.DownTo(ctx, 0)
	return convertResults(results), err
}

func (m *Migrator) Status(ctx context.Context) ([]connectors.MigrationStatus, error) {
	statuses, err := m.provider.Status(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]connectors.MigrationStatus, 0, len(statuses))
	for _, s := range statuses {
		if s == nil {
			continue
		}
		ms := connectors.MigrationStatus{
			IsApplied: s.State == goose.StateApplied,
		}
		if s.Source != nil {
			ms.Version = s.Source.Version
			ms.Source = s.Source.Path
		}
		if !s.AppliedAt.IsZero() {
			t := s.AppliedAt
			ms.AppliedAt = &t
		}
		out = append(out, ms)
	}
	return out, nil
}

func (m *Migrator) Version(ctx context.Context) (int64, error) {
	return m.provider.GetDBVersion(ctx)
}

func convertResult(r *goose.MigrationResult) *connectors.MigrationResult {
	if r == nil {
		return nil
	}
	out := &connectors.MigrationResult{
		Duration: r.Duration,
		Empty:    r.Empty,
	}
	if r.Source != nil {
		out.Version = r.Source.Version
		out.Source = r.Source.Path
	}
	return out
}

func convertResults(rs []*goose.MigrationResult) []connectors.MigrationResult {
	if len(rs) == 0 {
		return nil
	}
	out := make([]connectors.MigrationResult, 0, len(rs))
	for _, r := range rs {
		if r == nil {
			continue
		}
		out = append(out, *convertResult(r))
	}
	return out
}

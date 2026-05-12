package connectors

import (
	"context"
	"time"
)

type DatabaseConnector interface {
	SetConfig(config any)
	Init() error
	Conn() any
	Migrator() Migrator
}

type Migrator interface {
	Up(ctx context.Context) ([]MigrationResult, error)
	UpByOne(ctx context.Context) (*MigrationResult, error)
	UpTo(ctx context.Context, version int64) ([]MigrationResult, error)
	Down(ctx context.Context) (*MigrationResult, error)
	DownTo(ctx context.Context, version int64) ([]MigrationResult, error)
	Redo(ctx context.Context) (*MigrationResult, error)
	Reset(ctx context.Context) ([]MigrationResult, error)
	Status(ctx context.Context) ([]MigrationStatus, error)
	Version(ctx context.Context) (int64, error)
}

type MigrationResult struct {
	Version  int64
	Source   string
	Duration time.Duration
	Empty    bool
}

type MigrationStatus struct {
	Version   int64
	Source    string
	AppliedAt *time.Time
	IsApplied bool
}

package connectors

import "time"

type DatabaseConnector interface {
	Init(config interface{}) error
	Conn() any
	Migrator() Migrator
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

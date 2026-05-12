package connectors

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"
	"time"
)

const (
	EnvMigrateCmd     = "RAPTOR_MIGRATE_CMD"
	EnvMigrateVersion = "RAPTOR_MIGRATE_VERSION"
)

// RunMigrateFromEnv dispatches a migration command described by the
// RAPTOR_MIGRATE_CMD environment variable. The Raptor CLI sets this
// variable before exec'ing the user's binary so that goose runs inside the
// compiled app (the only place Go migrations registered via init() are
// visible).
//
// Supported commands: up, up-by-one, up-to, down, down-to, redo, reset,
// status, version. The up-to and down-to commands additionally read
// RAPTOR_MIGRATE_VERSION.
//
// Returns handled=true if a command was dispatched (regardless of err).
// Callers are expected to os.Exit after a handled command so the HTTP
// server does not start.
func RunMigrateFromEnv(ctx context.Context, m Migrator) (handled bool, err error) {
	cmd := os.Getenv(EnvMigrateCmd)
	if cmd == "" {
		return false, nil
	}
	if m == nil {
		return true, fmt.Errorf("migrate command %q requested but no database connector is registered", cmd)
	}

	switch cmd {
	case "up":
		results, err := m.Up(ctx)
		printResults("Applied", results)
		return true, err

	case "up-by-one":
		result, err := m.UpByOne(ctx)
		printResult("Applied", result)
		return true, err

	case "up-to":
		v, verr := parseVersion()
		if verr != nil {
			return true, verr
		}
		results, err := m.UpTo(ctx, v)
		printResults("Applied", results)
		return true, err

	case "down":
		result, err := m.Down(ctx)
		printResult("Rolled back", result)
		return true, err

	case "down-to":
		v, verr := parseVersion()
		if verr != nil {
			return true, verr
		}
		results, err := m.DownTo(ctx, v)
		printResults("Rolled back", results)
		return true, err

	case "redo":
		result, err := m.Redo(ctx)
		printResult("Re-applied", result)
		return true, err

	case "reset":
		results, err := m.Reset(ctx)
		printResults("Rolled back", results)
		return true, err

	case "status":
		statuses, err := m.Status(ctx)
		if err != nil {
			return true, err
		}
		printStatus(statuses)
		return true, nil

	case "version":
		v, err := m.Version(ctx)
		if err != nil {
			return true, err
		}
		fmt.Println(v)
		return true, nil

	default:
		return true, fmt.Errorf("unknown migrate command %q", cmd)
	}
}

func parseVersion() (int64, error) {
	raw := os.Getenv(EnvMigrateVersion)
	if raw == "" {
		return 0, fmt.Errorf("%s required for this command", EnvMigrateVersion)
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", EnvMigrateVersion, raw, err)
	}
	return v, nil
}

func printResult(verb string, r *MigrationResult) {
	if r == nil {
		fmt.Println("No migration to run.")
		return
	}
	suffix := ""
	if r.Empty {
		suffix = " (empty)"
	}
	fmt.Printf("%s migration %d %s in %s%s\n", verb, r.Version, sourceLabel(r.Source), r.Duration.Round(time.Millisecond), suffix)
}

func printResults(verb string, rs []MigrationResult) {
	if len(rs) == 0 {
		fmt.Println("No migrations to run.")
		return
	}
	for _, r := range rs {
		printResult(verb, &r)
	}
	fmt.Printf("%s %d migration(s).\n", verb, len(rs))
}

func printStatus(statuses []MigrationStatus) {
	if len(statuses) == 0 {
		fmt.Println("No migrations found.")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STATUS\tVERSION\tSOURCE\tAPPLIED AT")
	for _, s := range statuses {
		state := "pending"
		appliedAt := "-"
		if s.IsApplied {
			state = "applied"
			if s.AppliedAt != nil {
				appliedAt = s.AppliedAt.Format(time.RFC3339)
			}
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", state, s.Version, sourceLabel(s.Source), appliedAt)
	}
	w.Flush()
}

func sourceLabel(src string) string {
	if src == "" {
		return "-"
	}
	return filepath.Base(src)
}

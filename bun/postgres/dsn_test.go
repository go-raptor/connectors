package postgres

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// databaseConfig mirrors raptor/v4 config.DatabaseConfig from v4.4.0 on.
type databaseConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string
	Name        string
	AutoMigrate bool
	SSLMode     string
}

// legacyDatabaseConfig mirrors DatabaseConfig before SSLMode existed.
type legacyDatabaseConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string
	Name        string
	AutoMigrate bool
}

func parse(t *testing.T, config any) *pgxpool.Config {
	t.Helper()
	dsn, err := connString(config)
	if err != nil {
		t.Fatalf("connString: %v", err)
	}
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig(%q): %v", dsn, err)
	}
	return pc
}

func TestConnStringRoundTripsHostileCredentials(t *testing.T) {
	cfg := databaseConfig{Host: "db.example", Port: 6543, Username: "app",
		Password: "p@ss:w/rd ?#'x y", Name: "app_db", SSLMode: "require"}
	cc := parse(t, cfg).ConnConfig
	if cc.Host != "db.example" || cc.Port != 6543 || cc.User != "app" || cc.Password != cfg.Password || cc.Database != "app_db" {
		t.Fatalf("round trip lost a field: host=%q port=%d user=%q password=%q db=%q", cc.Host, cc.Port, cc.User, cc.Password, cc.Database)
	}
	if cc.TLSConfig == nil || len(cc.Fallbacks) != 0 {
		t.Fatalf("sslmode=require must use TLS with no plaintext fallback (tls=%v fallbacks=%d)", cc.TLSConfig != nil, len(cc.Fallbacks))
	}
}

func TestConnStringSSLMode(t *testing.T) {
	base := databaseConfig{Host: "db.example", Port: 5432, Username: "app", Password: "pw", Name: "app"}
	withMode := func(m string) databaseConfig { c := base; c.SSLMode = m; return c }
	tests := []struct {
		name              string
		config            any
		wantTLS           bool
		wantPlainFallback bool
	}{
		{"legacy config keeps disable", legacyDatabaseConfig{Host: "db.example", Port: 5432, Username: "app", Password: "pw", Name: "app"}, false, false},
		{"empty keeps disable", withMode(""), false, false},
		{"disable", withMode("disable"), false, false},
		{"prefer", withMode("prefer"), true, true},
		{"require", withMode("require"), true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := parse(t, tt.config).ConnConfig
			if got := cc.TLSConfig != nil; got != tt.wantTLS {
				t.Errorf("TLS = %v, want %v", got, tt.wantTLS)
			}
			if got := len(cc.Fallbacks) == 1 && cc.Fallbacks[0].TLSConfig == nil; got != tt.wantPlainFallback {
				t.Errorf("plaintext fallback = %v, want %v", got, tt.wantPlainFallback)
			}
		})
	}
}

func TestConnStringIPv6AndUnixSocketHosts(t *testing.T) {
	for host, want := range map[string]string{
		"::1":                 "::1",
		"[::1]":               "::1", // the bracketed form older pgx releases required
		"/var/run/postgresql": "/var/run/postgresql",
	} {
		cc := parse(t, databaseConfig{Host: host, Port: 5433, Username: "app", Password: "pw", Name: "app", SSLMode: "disable"}).ConnConfig
		if cc.Host != want || cc.Port != 5433 {
			t.Errorf("host %q: got host=%q port=%d, want host=%q", host, cc.Host, cc.Port, want)
		}
	}
}

func TestConnStringOmitsEmptyPassword(t *testing.T) {
	dsn, err := connString(databaseConfig{Host: "h", Port: 5432, Username: "app", Name: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "//app@") {
		t.Fatalf("an empty password must be omitted so PGPASSWORD/.pgpass still apply: %s", dsn)
	}
}

func TestConnStringRejectsBadConfig(t *testing.T) {
	for _, config := range []any{nil, "dsn", struct{ Host int }{1}} {
		if _, err := connString(config); err == nil {
			t.Errorf("connString(%#v) should fail, not panic or succeed", config)
		}
	}
}

package pgx

import (
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// legacySSLMode applies when the config has no SSLMode field (raptor/v4
// before v4.4.0) or leaves it empty: what this connector always used.
const legacySSLMode = "disable"

// connString builds a postgres:// URL from a raptor DatabaseConfig. Fields
// are read by name so this module does not import raptor, and net/url
// escapes every part, so credentials may contain any character.
func connString(config any) (string, error) {
	val := reflect.Indirect(reflect.ValueOf(config))
	if val.Kind() != reflect.Struct {
		return "", fmt.Errorf("database config must be a struct, got %T", config)
	}
	host, err := stringField(val, "Host")
	if err != nil {
		return "", err
	}
	user, err := stringField(val, "Username")
	if err != nil {
		return "", err
	}
	password, err := stringField(val, "Password")
	if err != nil {
		return "", err
	}
	name, err := stringField(val, "Name")
	if err != nil {
		return "", err
	}
	portField := val.FieldByName("Port")
	if !portField.IsValid() || portField.Kind() != reflect.Int {
		return "", fmt.Errorf("database config field Port must be an int")
	}
	port := strconv.FormatInt(portField.Int(), 10)

	sslMode := legacySSLMode
	if f := val.FieldByName("SSLMode"); f.IsValid() && f.Kind() == reflect.String && f.String() != "" {
		sslMode = f.String()
	}

	u := url.URL{Scheme: "postgres", User: url.User(user), Path: "/" + name}
	if password != "" {
		u.User = url.UserPassword(user, password)
	}
	query := url.Values{"sslmode": {sslMode}}
	if strings.HasPrefix(host, "/") {
		// A Unix-socket directory cannot sit in the URL authority.
		query.Set("host", host)
		query.Set("port", port)
	} else {
		// Older releases needed IPv6 hosts bracketed ("[::1]"); JoinHostPort
		// adds the brackets itself.
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = host[1 : len(host)-1]
		}
		u.Host = net.JoinHostPort(host, port)
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func stringField(val reflect.Value, name string) (string, error) {
	f := val.FieldByName(name)
	if !f.IsValid() || f.Kind() != reflect.String {
		return "", fmt.Errorf("database config field %s must be a string", name)
	}
	return f.String(), nil
}

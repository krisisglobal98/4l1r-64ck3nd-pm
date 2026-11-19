// Package config reads the service settings from the environment, after loading a local .env file if present.
package config

import (
	"bufio"
	"errors"
	"os"
	"regexp"
	"strings"
)

type Config struct {
	Port        string
	DatabaseURL string
	// DefaultUser (username) and DefaultUserID (UUID) act for every request until the Authentication service
	// issues tokens.
	DefaultUser   string
	DefaultUserID string
	LogLevel      string
	// AuthMode is "jwt" (tokens of the Authentication service, checked against its JWKS) or "default-user"
	// (every request acts as DefaultUser with full access: development and the e2e suite only).
	AuthMode   string
	JWKSURL    string
	AuthIssuer string
	// DefaultTenantID is the tenant of the default user (PM_AUTH_MODE=default-user).
	DefaultTenantID string
}

func Load() (Config, error) {
	loadDotEnv(".env")
	c := Config{
		Port:            get("PORT", "8081"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		DefaultUser:     get("PM_DEFAULT_USER", "admin"),
		DefaultUserID:   get("PM_DEFAULT_USER_ID", "00000000-0000-7000-8000-000000000001"),
		LogLevel:        get("LOG_LEVEL", "info"),
		AuthMode:        get("PM_AUTH_MODE", "jwt"),
		JWKSURL:         get("PM_AUTH_JWKS_URL", "http://localhost:8083/.well-known/jwks.json"),
		AuthIssuer:      get("PM_AUTH_ISSUER", "alir-auth"),
		DefaultTenantID: get("PM_DEFAULT_TENANT_ID", "00000000-0000-7000-8000-0000000000a1"),
	}
	if c.AuthMode != "jwt" && c.AuthMode != "default-user" {
		return c, errors.New(`PM_AUTH_MODE must be "jwt" or "default-user"`)
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is not set (see .env.example)")
	}
	if !uuidPattern.MatchString(c.DefaultUserID) {
		return c, errors.New("PM_DEFAULT_USER_ID must be a UUID")
	}
	return c, nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func get(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv sets KEY=VALUE lines of the file; variables already in the environment win.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}

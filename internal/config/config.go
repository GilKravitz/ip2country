// Package config loads service configuration from the environment.
//
// Configuration is intentionally environment-variable only (12-factor style):
// no config files, no flags, no dependency. Load validates eagerly so the
// process fails fast at startup rather than on the first bad request.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config holds the validated runtime configuration.
type Config struct {
	ListenAddr string     // address the HTTP server binds to
	RateLimit  int        // global allowed requests per second
	DB         string     // active datastore name (selects the geoip.Store)
	CSVPath    string     // path to the CSV datastore (when DB == "csv")
	LogLevel   slog.Level // structured-log verbosity
}

// Load reads configuration from the environment, applies defaults, and
// validates it. It returns an error describing the first problem found.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr: getenv("LISTEN_ADDR", ":8080"),
		DB:         getenv("IP2COUNTRY_DB", "csv"),
	}

	rps, err := parseInt(getenv("RATE_LIMIT_RPS", "100"))
	if err != nil {
		return Config{}, fmt.Errorf("RATE_LIMIT_RPS: %w", err)
	}
	if rps <= 0 {
		return Config{}, fmt.Errorf("RATE_LIMIT_RPS must be positive, got %v", rps)
	}
	cfg.RateLimit = rps

	level, err := parseLevel(getenv("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	cfg.LogLevel = level

	if cfg.DB == "csv" {
		cfg.CSVPath = os.Getenv("IP2COUNTRY_CSV_PATH")
		if cfg.CSVPath == "" {
			return Config{}, fmt.Errorf("IP2COUNTRY_CSV_PATH is required when IP2COUNTRY_DB=csv")
		}
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func parseLevel(s string) (slog.Level, error) {
	var l slog.Level
	// UnmarshalText accepts "debug"/"info"/"warn"/"error" (case-insensitive).
	if err := l.UnmarshalText([]byte(strings.TrimSpace(s))); err != nil {
		return 0, fmt.Errorf("unknown level %q", s)
	}
	return l, nil
}

package config

import (
	"log/slog"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("IP2COUNTRY_CSV_PATH", "data.csv") // required for default DB=csv

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", cfg.ListenAddr)
	}
	if cfg.RateLimit != 100 {
		t.Errorf("RateLimit = %v, want 100", cfg.RateLimit)
	}
	if cfg.DB != "csv" {
		t.Errorf("DB = %q, want csv", cfg.DB)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("LISTEN_ADDR", ":9999")
	t.Setenv("RATE_LIMIT_RPS", "2")
	t.Setenv("IP2COUNTRY_DB", "csv")
	t.Setenv("IP2COUNTRY_CSV_PATH", "/tmp/db.csv")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ListenAddr != ":9999" || cfg.RateLimit != 2 || cfg.CSVPath != "/tmp/db.csv" || cfg.LogLevel != slog.LevelDebug {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadValidation(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"non-positive rps", map[string]string{"RATE_LIMIT_RPS": "0", "IP2COUNTRY_CSV_PATH": "x"}},
		{"fractional rps", map[string]string{"RATE_LIMIT_RPS": "2.5", "IP2COUNTRY_CSV_PATH": "x"}},
		{"bad rps", map[string]string{"RATE_LIMIT_RPS": "abc", "IP2COUNTRY_CSV_PATH": "x"}},
		{"bad level", map[string]string{"LOG_LEVEL": "verbose", "IP2COUNTRY_CSV_PATH": "x"}},
		{"missing csv path", map[string]string{"IP2COUNTRY_DB": "csv"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Errorf("Load() expected error, got nil")
			}
		})
	}
}

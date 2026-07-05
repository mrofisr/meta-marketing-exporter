package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	// Create a valid temp config file.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfgContent := `
account_id: "act_123456789"
poll_interval: "10m"
otlp_endpoint: "http://localhost:4318/otlp/v1/metrics"
health_port: 8080
log_level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Set required env var.
	t.Setenv("META_ACCESS_TOKEN", "test-token-abc123")
	t.Setenv("OTEL_AUTH_HEADER", "Bearer xyz")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load(%q) error: %v", cfgPath, err)
	}

	// Verify fields.
	if cfg.AccountID != "act_123456789" {
		t.Errorf("AccountID = %q, want %q", cfg.AccountID, "act_123456789")
	}
	if cfg.PollInterval.Minutes() != 10 {
		t.Errorf("PollInterval = %v, want 10m", cfg.PollInterval)
	}
	if cfg.OTLPEndpoint != "http://localhost:4318/otlp/v1/metrics" {
		t.Errorf("OTLPEndpoint = %q, want http://localhost:4318/otlp/v1/metrics", cfg.OTLPEndpoint)
	}
	if cfg.HealthPort != 8080 {
		t.Errorf("HealthPort = %d, want 8080", cfg.HealthPort)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.AccessToken != "test-token-abc123" {
		t.Errorf("AccessToken = %q, want %q", cfg.AccessToken, "test-token-abc123")
	}
	if cfg.AuthHeader != "Bearer xyz" {
		t.Errorf("AuthHeader = %q, want %q", cfg.AuthHeader, "Bearer xyz")
	}
}

func TestLoad_MissingAccessToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfgContent := `
account_id: "act_123456789"
poll_interval: "10m"
otlp_endpoint: "http://localhost:4318/otlp/v1/metrics"
health_port: 8080
log_level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Ensure META_ACCESS_TOKEN is unset.
	t.Setenv("META_ACCESS_TOKEN", "")

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing META_ACCESS_TOKEN, got nil")
	}
	if !strings.Contains(err.Error(), "META_ACCESS_TOKEN") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "META_ACCESS_TOKEN")
	}
}

func TestLoad_MalformedAccountID(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
		wantErr   string
	}{
		{"plain number", "123", "account"},
		{"missing act_ prefix", "1234567890", "account"},
		{"wrong prefix", "acct_123", "account"},
		{"empty string", "", "account"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			cfgContent := `
account_id: "` + tc.accountID + `"
poll_interval: "10m"
otlp_endpoint: "http://localhost:4318/otlp/v1/metrics"
health_port: 8080
log_level: "info"
`
			if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			t.Setenv("META_ACCESS_TOKEN", "valid-token")

			_, err := Load(cfgPath)
			if err == nil {
				t.Fatalf("expected error for account_id=%q, got nil", tc.accountID)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoad_InvalidPollInterval(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfgContent := `
account_id: "act_123456789"
poll_interval: "0s"
otlp_endpoint: "http://localhost:4318/otlp/v1/metrics"
health_port: 8080
log_level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("META_ACCESS_TOKEN", "valid-token")

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for poll_interval=0, got nil")
	}
	if !strings.Contains(err.Error(), "poll_interval") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "poll_interval")
	}
}

func TestLoad_InvalidOTLPEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{"empty", ""},
		{"no scheme", "localhost:4318"},
		{"no host", "http://"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			cfgContent := `
account_id: "act_123456789"
poll_interval: "10m"
otlp_endpoint: "` + tc.endpoint + `"
health_port: 8080
log_level: "info"
`
			if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			t.Setenv("META_ACCESS_TOKEN", "valid-token")

			_, err := Load(cfgPath)
			if err == nil {
				t.Fatalf("expected error for otlp_endpoint=%q, got nil", tc.endpoint)
			}
			if !strings.Contains(err.Error(), "otlp_endpoint") {
				t.Errorf("error = %q, want to contain %q", err.Error(), "otlp_endpoint")
			}
		})
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

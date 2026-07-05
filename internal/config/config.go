// Package config loads and validates runtime configuration from a YAML file
// (defaults to ./config.yaml, overridable via --config flag) and environment
// variables. Secrets (META_ACCESS_TOKEN, optional OTEL_AUTH_HEADER) are
// sourced exclusively from env vars, never from YAML.
package config

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// accountIDPattern matches Meta Marketing API ad account IDs (e.g. "act_1234567890").
var accountIDPattern = regexp.MustCompile(`^act_\d+$`)

// Config holds runtime configuration for the meta-ads-exporter daemon.
//
// AccessToken and AuthHeader are loaded from env vars (META_ACCESS_TOKEN and
// OTEL_AUTH_HEADER) and are not present in the YAML file. All other fields
// come from the YAML file.
type Config struct {
	// AccountID is the Meta ad account id (must match ^act_\d+$).
	AccountID string `yaml:"account_id"`

	// PollInterval is the cadence at which insights are polled from Meta.
	// Must be > 0.
	PollInterval time.Duration `yaml:"poll_interval"`

	// OTLPEndpoint is the OTLP/HTTP metrics endpoint URL. Must be a valid URL.
	OTLPEndpoint string `yaml:"otlp_endpoint"`

	// HealthPort is the TCP port that /healthz and /readyz listen on.
	HealthPort int `yaml:"health_port"`

	// LogLevel controls slog output verbosity (e.g. "debug", "info", "warn", "error").
	LogLevel string `yaml:"log_level"`

	// AccessToken is the Meta Marketing API long-lived access token, loaded
	// from the META_ACCESS_TOKEN env var. Required.
	AccessToken string `yaml:"-"`

	// AuthHeader is an optional bearer/authorization header value attached to
	// OTLP requests, loaded from the OTEL_AUTH_HEADER env var.
	AuthHeader string `yaml:"-"`
}

// Load reads configuration from the YAML file at path (or ./config.yaml if
// path is ""), overlays env vars, and validates the result. It returns a
// descriptive error on any validation failure.
func Load(path string) (*Config, error) {
	if path == "" {
		path = "./config.yaml"
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	cfg.AccessToken = os.Getenv("META_ACCESS_TOKEN")
	cfg.AuthHeader = os.Getenv("OTEL_AUTH_HEADER")

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate enforces field-level invariants: AccountID matches ^act_\d+$,
// PollInterval > 0, OTLPEndpoint is a parseable URL, META_ACCESS_TOKEN is set.
// Errors are prefixed with "config:" and name the offending field.
func (c *Config) Validate() error {
	if c.AccessToken == "" {
		return errors.New("config: META_ACCESS_TOKEN env var is required")
	}
	if c.AccountID == "" {
		return errors.New("config: account_id is required")
	}
	if !accountIDPattern.MatchString(c.AccountID) {
		return fmt.Errorf("config: account_id %q must match pattern act_<digits>", c.AccountID)
	}
	if c.PollInterval <= 0 {
		return fmt.Errorf("config: poll_interval must be > 0 (got %s)", c.PollInterval)
	}
	if c.OTLPEndpoint == "" {
		return errors.New("config: otlp_endpoint is required")
	}
	u, err := url.Parse(c.OTLPEndpoint)
	if err != nil {
		return fmt.Errorf("config: otlp_endpoint %q is not a valid URL: %w", c.OTLPEndpoint, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("config: otlp_endpoint %q must include scheme and host", c.OTLPEndpoint)
	}
	return nil
}

// LoadFromFlags is a convenience helper for cmd/main.go: it registers a
// --config flag on the default FlagSet (if not already parsed) and calls Load
// with the resolved path. Callers that manage their own FlagSet should call
// Load directly.
func LoadFromFlags() (*Config, error) {
	var path string
	flag.StringVar(&path, "config", "./config.yaml", "path to YAML config file")
	if !flag.Parsed() {
		flag.Parse()
	}
	return Load(path)
}

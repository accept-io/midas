package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// CurrentVersion is the supported config schema version.
const CurrentVersion = 1

// Profile controls validation posture at startup.
type Profile string

const (
	// ProfileDev is permissive: any store backend, any auth mode.
	ProfileDev Profile = "dev"
	// ProfileProduction enforces postgres store + required auth mode.
	ProfileProduction Profile = "production"
)

// AuthMode controls whether inbound governance requests must carry credentials.
type AuthMode string

const (
	// AuthModeOpen allows unauthenticated access. Use in dev/local mode only.
	AuthModeOpen AuthMode = "open"
	// AuthModeRequired enforces bearer token authentication on all governed routes.
	AuthModeRequired AuthMode = "required"
)

// Duration wraps time.Duration to support YAML unmarshaling from strings like "30s".
type Duration time.Duration

// D returns the underlying time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// UnmarshalYAML parses Go duration strings (e.g. "30s", "2m", "1h").
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	*d = Duration(dur)
	return nil
}

// MarshalYAML serialises a Duration back to a Go duration string.
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// Config is the top-level runtime configuration for MIDAS.
// Load midas.yaml or supply env var overrides; see loader.go.
type Config struct {
	Version       int                 `yaml:"version"`
	Profile       Profile             `yaml:"profile"`
	Server        ServerConfig        `yaml:"server"`
	Store         StoreConfig         `yaml:"store"`
	Auth          AuthConfig          `yaml:"auth"`
	Observability ObservabilityConfig `yaml:"observability"`
	ControlPlane  ControlPlaneConfig  `yaml:"control_plane"`
	Dev           DevConfig           `yaml:"dev"`
	Dispatcher    DispatcherConfig    `yaml:"dispatcher"`
	Kafka         KafkaConfig         `yaml:"kafka"`
}

// DevConfig holds settings that only apply in developer / local mode.
// These settings are intentionally non-operational: they only affect
// startup behaviour (e.g. seeding) and are safe to leave disabled in production.
type DevConfig struct {
	// SeedDemoData seeds a minimal demonstration dataset at startup when true.
	// The seed is idempotent — running it on a store that already contains the
	// demo surface is a no-op. Defaults to true so that `make dev` / memory mode
	// works out of the box; set false in production or when supplying real data.
	SeedDemoData bool `yaml:"seed_demo_data"`
}

// ServerConfig controls the HTTP listener.
type ServerConfig struct {
	// Port is the TCP port to listen on. Default: 8080.
	Port int `yaml:"port"`
	// ShutdownTimeout is the graceful shutdown deadline. Default: "15s".
	ShutdownTimeout Duration `yaml:"shutdown_timeout"`
	// ExplorerEnabled serves the interactive evaluation sandbox at /explorer.
	// Enabled by default; set false in production if the UI is not needed.
	ExplorerEnabled bool `yaml:"explorer_enabled"`
}

// StoreConfig selects and configures the persistence backend.
type StoreConfig struct {
	// Backend selects the store implementation. Valid values: "memory", "postgres".
	Backend string `yaml:"backend"`
	// DSN is the Postgres connection string. Required when Backend is "postgres".
	// Supports placeholder expansion: "${DATABASE_URL}".
	DSN string `yaml:"dsn"`
}

// AuthConfig controls authentication enforcement on governance routes.
type AuthConfig struct {
	// Mode controls whether auth is enforced.
	// "open"     — no authentication required (dev/local only).
	// "required" — bearer token mandatory on all governed routes.
	Mode AuthMode `yaml:"mode"`
	// Tokens lists static bearer token entries.
	// Token values support placeholder expansion: "${MY_SECRET_TOKEN}".
	Tokens []TokenEntry `yaml:"tokens"`
}

// TokenEntry represents a single static bearer token with its principal and roles.
type TokenEntry struct {
	// Token is the bearer token value. Supports ${VAR} expansion.
	Token string `yaml:"token"`
	// Principal is the actor ID (e.g. "user:alice", "svc:payments").
	Principal string `yaml:"principal"`
	// Roles is a comma-separated list of roles (e.g. "admin,operator").
	Roles string `yaml:"roles"`
}

// ObservabilityConfig controls logging behaviour.
type ObservabilityConfig struct {
	// LogLevel controls verbosity. Valid: "debug", "info", "warn", "error". Default: "info".
	LogLevel string `yaml:"log_level"`
	// LogFormat selects the output format. Valid: "json", "text". Default: "json".
	LogFormat string `yaml:"log_format"`
}

// ControlPlaneConfig enables or disables the control plane endpoints.
type ControlPlaneConfig struct {
	// Enabled controls whether /v1/controlplane/* routes are registered.
	Enabled bool `yaml:"enabled"`
}

// DispatcherConfig controls the outbox dispatcher goroutine.
type DispatcherConfig struct {
	// Enabled controls whether the dispatcher goroutine is started.
	Enabled bool `yaml:"enabled"`
	// Publisher selects the broker implementation. Valid: "none", "kafka".
	Publisher string `yaml:"publisher"`
	// BatchSize is the maximum outbox rows claimed per poll cycle. Default: 100.
	BatchSize int `yaml:"batch_size"`
	// PollInterval is the sleep between poll cycles when the queue is empty. Default: "2s".
	PollInterval Duration `yaml:"poll_interval"`
	// MaxBackoff is the upper bound for exponential sleep on consecutive errors. Default: "30s".
	MaxBackoff Duration `yaml:"max_backoff"`
}

// KafkaConfig holds Kafka broker settings for the Kafka publisher.
// Only meaningful when DispatcherConfig.Publisher is "kafka".
type KafkaConfig struct {
	// Brokers is the list of seed broker addresses in "host:port" form.
	Brokers []string `yaml:"brokers"`
	// ClientID is an optional identifier sent to the broker for observability.
	ClientID string `yaml:"client_id"`
	// RequiredAcks controls acknowledgement level: -1=all ISRs, 0=none, 1=leader.
	RequiredAcks int `yaml:"required_acks"`
	// WriteTimeout bounds the per-message publish call. Zero means no timeout.
	WriteTimeout Duration `yaml:"write_timeout"`
}

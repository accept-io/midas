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
	LocalIAM      LocalIAMConfig      `yaml:"local_iam"`
	PlatformOIDC  PlatformOIDCConfig  `yaml:"platform_oidc"`
	Observability ObservabilityConfig `yaml:"observability"`
	ControlPlane  ControlPlaneConfig  `yaml:"control_plane"`
	Dev           DevConfig           `yaml:"dev"`
	Dispatcher    DispatcherConfig    `yaml:"dispatcher"`
	Kafka         KafkaConfig         `yaml:"kafka"`
}

// PlatformOIDCConfig configures OIDC-based platform login (Explorer/console SSO).
// This is entirely separate from runtime governance auth on /v1/* routes.
// The structure is provider-agnostic; any OIDC-compliant provider (e.g. Entra,
// Google Workspace) is supported via configuration alone.
type PlatformOIDCConfig struct {
	// Enabled activates OIDC login. When true the /auth/oidc/* endpoints are
	// registered. Requires LocalIAM to also be enabled for session management.
	Enabled bool `yaml:"enabled"`
	// ProviderName is a display label only (e.g. "entra", "google"). Not used in auth logic.
	ProviderName string `yaml:"provider_name"`

	// IssuerURL is the OIDC issuer used for provider discovery.
	// e.g. https://login.microsoftonline.com/<tenant>/v2.0 (Entra)
	// or   https://accounts.google.com (Google)
	IssuerURL string `yaml:"issuer_url"`
	// AuthURL overrides the discovered authorization endpoint (optional).
	AuthURL string `yaml:"auth_url"`
	// TokenURL overrides the discovered token endpoint (optional).
	TokenURL string `yaml:"token_url"`

	// ClientID is the OAuth2 application client ID. Supports ${VAR} expansion.
	ClientID string `yaml:"client_id"`
	// ClientSecret is the OAuth2 application client secret. Supports ${VAR} expansion.
	ClientSecret string `yaml:"client_secret"`
	// RedirectURL is the callback URL registered with the identity provider.
	RedirectURL string `yaml:"redirect_url"`

	// Scopes requested from the provider. Defaults: openid, profile, email.
	Scopes []string `yaml:"scopes"`

	// SubjectClaim is the ID token claim used as the principal subject. Default: "sub".
	SubjectClaim string `yaml:"subject_claim"`
	// UsernameClaim is the claim used as the display username.
	// Default: "preferred_username" (Entra). Set to "email" for Google Workspace.
	UsernameClaim string `yaml:"username_claim"`
	// GroupsClaim is the claim containing group membership or domain identity.
	// Default: "groups" (Entra array). Set to "hd" for Google Workspace (hosted domain).
	// The value may be a JSON string or array; both are normalised to []string.
	// If absent from the token, groups are treated as empty.
	GroupsClaim string `yaml:"groups_claim"`

	// DomainHint is passed to the provider as a login hint (optional).
	DomainHint string `yaml:"domain_hint"`
	// AllowedGroups restricts login to members of at least one listed group (optional).
	// An empty slice allows any authenticated user.
	AllowedGroups []string `yaml:"allowed_groups"`

	// RoleMappings maps external group identifiers to MIDAS internal roles.
	RoleMappings []OIDCRoleMapping `yaml:"role_mappings"`

	// DenyIfNoRoles rejects login when no internal roles are mapped. Default: true.
	DenyIfNoRoles bool `yaml:"deny_if_no_roles"`
	// UsePKCE enables PKCE (Proof Key for Code Exchange). Default: true.
	UsePKCE bool `yaml:"use_pkce"`
}

// OIDCRoleMapping maps a single external group value to a MIDAS internal role.
type OIDCRoleMapping struct {
	// External is the group name as returned by the identity provider.
	External string `yaml:"external"`
	// Internal is the canonical MIDAS role (e.g. "platform.admin").
	Internal string `yaml:"internal"`
}

// LocalIAMConfig controls local platform IAM for the Explorer/console.
// Local IAM provides username/password login with session cookies and is
// entirely separate from runtime authority (bearer-token auth on /v1/* routes).
type LocalIAMConfig struct {
	// Enabled activates local platform IAM. When true, bootstrap admin/admin is
	// created on first run and the /auth/* endpoints are registered.
	Enabled bool `yaml:"enabled"`
	// SessionTTL controls how long a session remains valid. Default: "8h".
	SessionTTL Duration `yaml:"session_ttl"`
	// SecureCookies sets the Secure flag on session cookies. Enable in
	// production (HTTPS). Defaults to false for local HTTP development.
	SecureCookies bool `yaml:"secure_cookies"`
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
	// ReadTimeout bounds the time to read the entire request including body.
	// Default: "30s". Zero means no timeout (not recommended for production).
	ReadTimeout Duration `yaml:"read_timeout"`
	// ReadHeaderTimeout bounds the time to read request headers.
	// Default: "10s". Zero means no timeout.
	ReadHeaderTimeout Duration `yaml:"read_header_timeout"`
	// WriteTimeout bounds the time to write the full response.
	// Default: "60s". Zero means no timeout (not recommended for production).
	WriteTimeout Duration `yaml:"write_timeout"`
	// IdleTimeout bounds how long to keep idle keep-alive connections open.
	// Default: "120s". Zero falls back to ReadTimeout.
	IdleTimeout Duration `yaml:"idle_timeout"`
	// ExplorerEnabled serves the interactive evaluation sandbox at /explorer.
	// Enabled by default; set false in production if the UI is not needed.
	ExplorerEnabled bool `yaml:"explorer_enabled"`
	// Headless disables all browser-facing surfaces and platform-login routes.
	// When true: /explorer, /auth/*, local IAM, and OIDC are not initialised.
	// /v1/*, /healthz, and /readyz remain fully operational.
	// Conflicts with: explorer_enabled=true, local_iam.enabled=true, platform_oidc.enabled=true.
	// Default: false.
	Headless bool `yaml:"headless"`
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

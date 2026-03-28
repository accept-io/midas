package config

import "time"

// DefaultConfig returns the baseline configuration used when no file is present.
// All defaults are safe for local development. Production deployments are
// expected to supply a midas.yaml that overrides store, auth, and profile.
func DefaultConfig() Config {
	return Config{
		Version: CurrentVersion,
		Profile: ProfileDev,
		Server: ServerConfig{
			Port:            8080,
			ShutdownTimeout: Duration(15 * time.Second),
			ExplorerEnabled: true,
			Headless:        false,
		},
		Store: StoreConfig{
			Backend: "memory",
			DSN:     "",
		},
		Auth: AuthConfig{
			Mode:   AuthModeOpen,
			Tokens: nil,
		},
		LocalIAM: LocalIAMConfig{
			Enabled:       false,
			SessionTTL:    Duration(8 * time.Hour),
			SecureCookies: false,
		},
		PlatformOIDC: PlatformOIDCConfig{
			Enabled:       false,
			SubjectClaim:  "sub",
			UsernameClaim: "preferred_username",
			GroupsClaim:   "groups",
			Scopes:        []string{"openid", "profile", "email"},
			DenyIfNoRoles: true,
			UsePKCE:       true,
		},
		Observability: ObservabilityConfig{
			LogLevel:  "info",
			LogFormat: "json",
		},
		ControlPlane: ControlPlaneConfig{
			Enabled: true,
		},
		Dev: DevConfig{
			SeedDemoData: true,
		},
		Dispatcher: DispatcherConfig{
			Enabled:      false,
			Publisher:    "none",
			BatchSize:    100,
			PollInterval: Duration(2 * time.Second),
			MaxBackoff:   Duration(30 * time.Second),
		},
		Kafka: KafkaConfig{
			ClientID:     "midas",
			RequiredAcks: -1,
		},
	}
}

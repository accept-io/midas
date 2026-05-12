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
			Port:              8080,
			ShutdownTimeout:   Duration(15 * time.Second),
			ReadTimeout:       Duration(30 * time.Second),
			ReadHeaderTimeout: Duration(10 * time.Second),
			WriteTimeout:      Duration(60 * time.Second),
			IdleTimeout:       Duration(120 * time.Second),
			HandlerTimeout:    Duration(30 * time.Second),
			ExplorerEnabled:   true,
			Headless:          false,
		},
		Store: StoreConfig{
			Backend:         "memory",
			DSN:             "",
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: Duration(30 * time.Minute),
			ConnMaxIdleTime: Duration(5 * time.Minute),
		},
		Auth: AuthConfig{
			Mode:   AuthModeOpen,
			Tokens: nil,
		},
		LocalIAM: LocalIAMConfig{
			Enabled:       true,
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
			LogLevel:       "info",
			LogFormat:      "json",
			MetricsEnabled: true,
			MetricsPath:    "/metrics",
		},
		ControlPlane: ControlPlaneConfig{
			Enabled: true,
		},
		Dev: DevConfig{
			SeedDemoData: true,
			SeedDemoUser: true,
			// SeedSyntheticDrift is intentionally left nil so the
			// effective behaviour (resolved by
			// DevConfig.EffectiveSeedSyntheticDrift) inherits from
			// SeedDemoData. An explicit env / YAML value of true or
			// false overrides the inheritance.
			SeedSyntheticDrift: nil,
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
		Structural: StructuralConfig{
			Mode: StructuralModePermissive,
		},
		FailMode: FailModeConfig{
			DeploymentDefaultPolicyID: "",
		},
	}
}

package config

import (
	"testing"
	"time"
)

// TestDefaultConfig verifies that DefaultConfig produces a valid configuration
// that passes both structural and semantic validation without modification.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if err := ValidateStructural(cfg); err != nil {
		t.Errorf("DefaultConfig() failed structural validation: %v", err)
	}
	if err := ValidateSemantic(cfg); err != nil {
		t.Errorf("DefaultConfig() failed semantic validation: %v", err)
	}
}

func TestDefaultConfig_Values(t *testing.T) {
	cfg := DefaultConfig()

	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"version", cfg.Version, CurrentVersion},
		{"profile", string(cfg.Profile), string(ProfileDev)},
		{"store.backend", cfg.Store.Backend, "memory"},
		{"auth.mode", string(cfg.Auth.Mode), string(AuthModeOpen)},
		{"server.port", cfg.Server.Port, 8080},
		{"observability.log_level", cfg.Observability.LogLevel, "info"},
		{"observability.log_format", cfg.Observability.LogFormat, "json"},
		{"local_iam.enabled", cfg.LocalIAM.Enabled, true},
		{"dispatcher.enabled", cfg.Dispatcher.Enabled, false},
		{"dispatcher.publisher", cfg.Dispatcher.Publisher, "none"},
		{"dispatcher.batch_size", cfg.Dispatcher.BatchSize, 100},
		{"kafka.client_id", cfg.Kafka.ClientID, "midas"},
		{"kafka.required_acks", cfg.Kafka.RequiredAcks, -1},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("want %v, got %v", c.want, c.got)
			}
		})
	}

	// Duration fields
	if cfg.Server.ShutdownTimeout.D() != 15*time.Second {
		t.Errorf("server.shutdown_timeout: want 15s, got %v", cfg.Server.ShutdownTimeout.D())
	}
	if cfg.Dispatcher.PollInterval.D() != 2*time.Second {
		t.Errorf("dispatcher.poll_interval: want 2s, got %v", cfg.Dispatcher.PollInterval.D())
	}
	if cfg.Dispatcher.MaxBackoff.D() != 30*time.Second {
		t.Errorf("dispatcher.max_backoff: want 30s, got %v", cfg.Dispatcher.MaxBackoff.D())
	}
}

// TestDefaultConfig_HandlerTimeoutDefault verifies the per-handler safety
// timeout default (D27d). The default is shorter than WriteTimeout so the
// safety bound fires before the coarse server-level deadline.
func TestDefaultConfig_HandlerTimeoutDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.HandlerTimeout.D() != 30*time.Second {
		t.Errorf("server.handler_timeout: want 30s, got %v", cfg.Server.HandlerTimeout.D())
	}
}

// TestDefaultConfig_MetricsDefaults verifies that runtime metrics are
// enabled by default and exposed at /metrics. These defaults match the
// production-ready posture from D27c — operators who want metrics off must
// opt out explicitly.
func TestDefaultConfig_MetricsDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Observability.MetricsEnabled {
		t.Error("metrics_enabled: want true (default), got false")
	}
	if cfg.Observability.MetricsPath != "/metrics" {
		t.Errorf("metrics_path: want \"/metrics\", got %q", cfg.Observability.MetricsPath)
	}
}

// TestDefaultConfig_StorePoolDefaults verifies the Postgres connection pool
// defaults. These values are conservative improvements on database/sql defaults
// (which are: unlimited open conns, 2 idle, no lifetime cap).
func TestDefaultConfig_StorePoolDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Store.MaxOpenConns != 25 {
		t.Errorf("store.max_open_conns: want 25, got %d", cfg.Store.MaxOpenConns)
	}
	if cfg.Store.MaxIdleConns != 5 {
		t.Errorf("store.max_idle_conns: want 5, got %d", cfg.Store.MaxIdleConns)
	}
	if cfg.Store.ConnMaxLifetime.D() != 30*time.Minute {
		t.Errorf("store.conn_max_lifetime: want 30m, got %v", cfg.Store.ConnMaxLifetime.D())
	}
	if cfg.Store.ConnMaxIdleTime.D() != 5*time.Minute {
		t.Errorf("store.conn_max_idle_time: want 5m, got %v", cfg.Store.ConnMaxIdleTime.D())
	}
}

// TestDefaultConfig_StructuralModeIsPermissive verifies that the default
// structural mode is permissive, preserving out-of-box usability.
func TestDefaultConfig_StructuralModeIsPermissive(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Structural.Mode != StructuralModePermissive {
		t.Errorf("structural.mode: want %q (default), got %q", StructuralModePermissive, cfg.Structural.Mode)
	}
}

// TestValidateStructural_InvalidStructuralMode verifies that an unknown
// structural mode is rejected.
func TestValidateStructural_InvalidStructuralMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Structural.Mode = "strict"
	if err := ValidateStructural(cfg); err == nil {
		t.Error("want error for unknown structural.mode, got nil")
	}
}

// TestValidateStructural_KnownStructuralModes verifies that both known modes pass.
func TestValidateStructural_KnownStructuralModes(t *testing.T) {
	for _, mode := range []StructuralMode{StructuralModePermissive, StructuralModeEnforced} {
		t.Run(string(mode), func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Structural.Mode = mode
			if err := ValidateStructural(cfg); err != nil {
				t.Errorf("structural.mode=%q: unexpected validation error: %v", mode, err)
			}
		})
	}
}

// TestDuration_UnmarshalYAML verifies that Duration YAML roundtrips correctly.
func TestDuration_UnmarshalYAML(t *testing.T) {
	cases := []struct {
		input string
		want  time.Duration
	}{
		{"30s", 30 * time.Second},
		{"2m", 2 * time.Minute},
		{"1h30m", 90 * time.Minute},
		{"500ms", 500 * time.Millisecond},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			d, err := time.ParseDuration(c.input)
			if err != nil {
				t.Fatal(err)
			}
			if d != c.want {
				t.Errorf("want %v, got %v", c.want, d)
			}
		})
	}
}

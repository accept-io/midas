package config

import (
	"testing"
)

// ---------------------------------------------------------------------------
// ValidateStructural — field-level enum checks
// ---------------------------------------------------------------------------

func TestValidateStructural_ValidDefaults(t *testing.T) {
	if err := ValidateStructural(DefaultConfig()); err != nil {
		t.Errorf("DefaultConfig structural validation failed: %v", err)
	}
}

func TestValidateStructural_UnsupportedVersion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 99
	if err := ValidateStructural(cfg); err == nil {
		t.Error("want error for unsupported version")
	}
}

func TestValidateStructural_InvalidProfile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Profile = "enterprise"
	if err := ValidateStructural(cfg); err == nil {
		t.Error("want error for invalid profile")
	}
}

func TestValidateStructural_InvalidAuthMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth.Mode = "optional"
	if err := ValidateStructural(cfg); err == nil {
		t.Error("want error for invalid auth mode")
	}
}

func TestValidateStructural_InvalidStoreBackend(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Store.Backend = "redis"
	if err := ValidateStructural(cfg); err == nil {
		t.Error("want error for invalid store backend")
	}
}

func TestValidateStructural_InvalidLogLevel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Observability.LogLevel = "trace"
	if err := ValidateStructural(cfg); err == nil {
		t.Error("want error for invalid log level")
	}
}

func TestValidateStructural_InvalidLogFormat(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Observability.LogFormat = "yaml"
	if err := ValidateStructural(cfg); err == nil {
		t.Error("want error for invalid log format")
	}
}

func TestValidateStructural_InvalidPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Port = 0
	if err := ValidateStructural(cfg); err == nil {
		t.Error("want error for port=0")
	}

	cfg.Server.Port = 99999
	if err := ValidateStructural(cfg); err == nil {
		t.Error("want error for port=99999")
	}
}

func TestValidateStructural_MultipleErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Profile = "bad"
	cfg.Auth.Mode = "optional"
	cfg.Store.Backend = "redis"

	err := ValidateStructural(cfg)
	if err == nil {
		t.Fatal("want multiple errors, got nil")
	}
	ve, ok := err.(ValidationErrors)
	if !ok {
		t.Fatalf("want ValidationErrors, got %T", err)
	}
	if len(ve) < 3 {
		t.Errorf("want at least 3 errors, got %d: %v", len(ve), ve)
	}
}

// ---------------------------------------------------------------------------
// ValidateSemantic — cross-field business rules
// ---------------------------------------------------------------------------

func TestValidateSemantic_ValidDefaults(t *testing.T) {
	if err := ValidateSemantic(DefaultConfig()); err != nil {
		t.Errorf("DefaultConfig semantic validation failed: %v", err)
	}
}

func TestValidateSemantic_PostgresRequiresDSN(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Store.Backend = "postgres"
	cfg.Store.DSN = ""
	if err := ValidateSemantic(cfg); err == nil {
		t.Error("want error: postgres without DSN")
	}
}

func TestValidateSemantic_ProductionRequiresPostgres(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Profile = ProfileProduction
	cfg.Store.Backend = "memory" // wrong for production
	cfg.Auth.Mode = AuthModeRequired
	cfg.Auth.Tokens = []TokenEntry{{Token: "t", Principal: "u", Roles: "admin"}}

	if err := ValidateSemantic(cfg); err == nil {
		t.Error("want error: production with memory store")
	}
}

func TestValidateSemantic_ProductionRequiresRequiredAuth(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Profile = ProfileProduction
	cfg.Store.Backend = "postgres"
	cfg.Store.DSN = "postgres://x:y@host/db"
	cfg.Auth.Mode = AuthModeOpen // wrong for production

	if err := ValidateSemantic(cfg); err == nil {
		t.Error("want error: production with open auth")
	}
}

func TestValidateSemantic_ProductionValid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Profile = ProfileProduction
	cfg.Store.Backend = "postgres"
	cfg.Store.DSN = "postgres://x:y@host/db"
	cfg.Auth.Mode = AuthModeRequired
	cfg.Auth.Tokens = []TokenEntry{{Token: "tok", Principal: "user:admin", Roles: "admin"}}

	if err := ValidateSemantic(cfg); err != nil {
		t.Errorf("valid production config failed: %v", err)
	}
}

func TestValidateSemantic_RequiredAuthNeedsTokens(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth.Mode = AuthModeRequired
	cfg.Auth.Tokens = nil

	if err := ValidateSemantic(cfg); err == nil {
		t.Error("want error: required auth with no tokens")
	}
}

func TestValidateSemantic_EmptyTokenValue(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth.Mode = AuthModeRequired
	cfg.Auth.Tokens = []TokenEntry{{Token: "", Principal: "user:alice", Roles: "admin"}}

	if err := ValidateSemantic(cfg); err == nil {
		t.Error("want error: empty token value")
	}
}

func TestValidateSemantic_EmptyPrincipal(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth.Mode = AuthModeRequired
	cfg.Auth.Tokens = []TokenEntry{{Token: "tok", Principal: "", Roles: "admin"}}

	if err := ValidateSemantic(cfg); err == nil {
		t.Error("want error: empty principal")
	}
}

func TestValidateSemantic_DispatcherEnabledNoPublisher(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dispatcher.Enabled = true
	cfg.Dispatcher.Publisher = "none"

	if err := ValidateSemantic(cfg); err == nil {
		t.Error("want error: dispatcher enabled with publisher=none")
	}
}

func TestValidateSemantic_DispatcherKafkaNoBrokers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dispatcher.Enabled = true
	cfg.Dispatcher.Publisher = "kafka"
	cfg.Kafka.Brokers = nil

	if err := ValidateSemantic(cfg); err == nil {
		t.Error("want error: kafka publisher without brokers")
	}
}

func TestValidateSemantic_DispatcherKafkaValid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dispatcher.Enabled = true
	cfg.Dispatcher.Publisher = "kafka"
	cfg.Kafka.Brokers = []string{"broker1:9092"}

	if err := ValidateSemantic(cfg); err != nil {
		t.Errorf("valid kafka dispatcher config failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ValidationError formatting
// ---------------------------------------------------------------------------

func TestValidationErrors_Error(t *testing.T) {
	ve := ValidationErrors{
		{Field: "auth.mode", Message: "must be open or required"},
		{Field: "store.backend", Message: "must be memory or postgres"},
	}
	msg := ve.Error()
	if msg == "" {
		t.Error("want non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// maskDSN — secret masking
// ---------------------------------------------------------------------------

func TestMaskDSN(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"postgres://user:pass@localhost:5432/mydb?sslmode=disable", "localhost:5432/mydb"},
		{"postgresql://user:secret@db.example.com/prod", "db.example.com/prod"},
		{"postgres://host/db", "host/db"},
		{"localhost:5432/mydb", "localhost:5432/mydb"}, // no scheme, no userinfo
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := maskDSN(c.input)
			if got != c.want {
				t.Errorf("maskDSN(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

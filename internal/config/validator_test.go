package config

import (
	"strings"
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
	cfg.LocalIAM.SecureCookies = true

	if err := ValidateSemantic(cfg); err != nil {
		t.Errorf("valid production config failed: %v", err)
	}
}

// productionBaseConfig returns a Config that satisfies every production-profile
// requirement except those each Local-IAM secure-cookies test deliberately
// mutates. Tests built from this baseline isolate the secure_cookies rule from
// the existing postgres / auth-mode / token rules.
func productionBaseConfig() Config {
	cfg := DefaultConfig()
	cfg.Profile = ProfileProduction
	cfg.Store.Backend = "postgres"
	cfg.Store.DSN = "postgres://x:y@host/db"
	cfg.Auth.Mode = AuthModeRequired
	cfg.Auth.Tokens = []TokenEntry{{Token: "tok", Principal: "user:admin", Roles: "admin"}}
	return cfg
}

// hasFieldError reports whether ve carries a ValidationError for the named field.
func hasFieldError(err error, field string) bool {
	ve, ok := err.(ValidationErrors)
	if !ok {
		return false
	}
	for _, e := range ve {
		if e.Field == field {
			return true
		}
	}
	return false
}

// Case A: production + local_iam.enabled=true + secure_cookies=false → must
// fail with an error on field "local_iam.secure_cookies".
func TestValidateSemantic_ProductionRequiresSecureCookies_WhenLocalIAMEnabled(t *testing.T) {
	cfg := productionBaseConfig()
	cfg.LocalIAM.Enabled = true
	cfg.LocalIAM.SecureCookies = false

	err := ValidateSemantic(cfg)
	if err == nil {
		t.Fatal("want error: production profile with local_iam.enabled and secure_cookies=false")
	}
	if !hasFieldError(err, "local_iam.secure_cookies") {
		t.Errorf("want ValidationError on local_iam.secure_cookies, got: %v", err)
	}
}

// Case B: production + local_iam.enabled=true + secure_cookies=true → no error
// from the secure_cookies rule.
func TestValidateSemantic_ProductionAllowsSecureCookiesTrue(t *testing.T) {
	cfg := productionBaseConfig()
	cfg.LocalIAM.Enabled = true
	cfg.LocalIAM.SecureCookies = true

	if err := ValidateSemantic(cfg); err != nil {
		t.Errorf("valid production config with secure_cookies=true failed: %v", err)
	}
}

// Case C: production + local_iam.enabled=false + secure_cookies=false → no
// error: a headless production deployment writes no Local IAM cookies, so the
// rule does not fire.
func TestValidateSemantic_ProductionLocalIAMDisabled_DoesNotRequireSecureCookies(t *testing.T) {
	cfg := productionBaseConfig()
	cfg.LocalIAM.Enabled = false
	cfg.LocalIAM.SecureCookies = false

	if err := ValidateSemantic(cfg); err != nil {
		t.Errorf("production with local_iam.enabled=false should pass regardless of secure_cookies: %v", err)
	}
}

// Case D: dev profile + local_iam.enabled=true + secure_cookies=false → no
// error: the rule is scoped to the production profile so local HTTP
// development is unaffected.
func TestValidateSemantic_DevProfile_DoesNotRequireSecureCookies(t *testing.T) {
	cfg := DefaultConfig() // ProfileDev by default
	cfg.LocalIAM.Enabled = true
	cfg.LocalIAM.SecureCookies = false

	err := ValidateSemantic(cfg)
	if err != nil {
		t.Errorf("dev profile with secure_cookies=false should pass: %v", err)
	}
	if hasFieldError(err, "local_iam.secure_cookies") {
		t.Errorf("dev profile must not raise local_iam.secure_cookies error: %v", err)
	}
}

// TestDefaultConfig_LocalIAM_DevPosture locks in the documented defaulting
// contract: DefaultConfig returns a dev-profile configuration whose Local IAM
// posture supports local HTTP login. SecureCookies is intentionally false at
// this layer so a binary launched with no midas.yaml works on
// http://localhost:8080 (browsers do not round-trip Secure cookies over plain
// HTTP). Production-profile deployments are required to flip SecureCookies to
// true via ValidateSemantic — the test cases above lock in that path.
//
// If a future change flips the default to true, this test will fail and force
// a deliberate decision about the local-quickstart experience rather than
// drifting silently. See internal/config/validator.go for the production rule
// and docs/guides/authentication.md for the local-HTTP rationale.
func TestDefaultConfig_LocalIAM_DevPosture(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Profile != ProfileDev {
		t.Errorf("DefaultConfig().Profile: want %q (dev profile is the documented default), got %q", ProfileDev, cfg.Profile)
	}
	if !cfg.LocalIAM.Enabled {
		t.Errorf("DefaultConfig().LocalIAM.Enabled: want true (out-of-box Explorer login), got false")
	}
	if cfg.LocalIAM.SecureCookies {
		t.Errorf("DefaultConfig().LocalIAM.SecureCookies: want false " +
			"(dev profile supports local HTTP; production validator enforces true). " +
			"If this default is being flipped, update the test, the docs, and verify the local quickstart still works on http://localhost.")
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
// Headless conflict validation
// ---------------------------------------------------------------------------

func headlessCfg() Config {
	cfg := DefaultConfig()
	cfg.Server.Headless = true
	cfg.Server.ExplorerEnabled = false // avoid conflict by default
	cfg.LocalIAM.Enabled = false       // avoid conflict: headless + local_iam is invalid
	return cfg
}

func TestValidateSemantic_Headless_DefaultsValid(t *testing.T) {
	// headless=true with no conflicting options — must be valid.
	cfg := headlessCfg()
	if err := ValidateSemantic(cfg); err != nil {
		t.Errorf("headless with no conflicts: want no error, got %v", err)
	}
}

func TestValidateSemantic_Headless_WithRequiredAuth_Valid(t *testing.T) {
	// headless + auth.mode=required is explicitly valid.
	cfg := headlessCfg()
	cfg.Auth.Mode = AuthModeRequired
	cfg.Auth.Tokens = []TokenEntry{{Token: "tok", Principal: "svc:a", Roles: "platform.operator"}}
	if err := ValidateSemantic(cfg); err != nil {
		t.Errorf("headless + required auth: want no error, got %v", err)
	}
}

func TestValidateSemantic_Headless_NoLocalIAMOrOIDC_Valid(t *testing.T) {
	// Headless with neither local_iam nor platform_oidc in config — clean.
	cfg := headlessCfg()
	// local_iam.enabled defaults to true but headlessCfg() sets it false to avoid conflict
	if err := ValidateSemantic(cfg); err != nil {
		t.Errorf("headless with no iam/oidc: want no error, got %v", err)
	}
}

func TestValidateSemantic_Headless_ConflictsWithExplorerEnabled(t *testing.T) {
	cfg := headlessCfg()
	cfg.Server.ExplorerEnabled = true
	err := ValidateSemantic(cfg)
	if err == nil {
		t.Fatal("want error for headless + explorer_enabled, got nil")
	}
	if !strings.Contains(err.Error(), "server.headless") {
		t.Errorf("error must mention server.headless, got: %v", err)
	}
	if !strings.Contains(err.Error(), "server.explorer_enabled") {
		t.Errorf("error must mention server.explorer_enabled, got: %v", err)
	}
}

func TestValidateSemantic_Headless_ConflictsWithLocalIAM(t *testing.T) {
	cfg := headlessCfg()
	cfg.LocalIAM.Enabled = true
	err := ValidateSemantic(cfg)
	if err == nil {
		t.Fatal("want error for headless + local_iam.enabled, got nil")
	}
	if !strings.Contains(err.Error(), "server.headless") {
		t.Errorf("error must mention server.headless, got: %v", err)
	}
	if !strings.Contains(err.Error(), "local_iam.enabled") {
		t.Errorf("error must mention local_iam.enabled, got: %v", err)
	}
}

func TestValidateSemantic_Headless_ConflictsWithOIDC(t *testing.T) {
	cfg := headlessCfg()
	cfg.PlatformOIDC.Enabled = true
	err := ValidateSemantic(cfg)
	if err == nil {
		t.Fatal("want error for headless + platform_oidc.enabled, got nil")
	}
	if !strings.Contains(err.Error(), "server.headless") {
		t.Errorf("error must mention server.headless, got: %v", err)
	}
	if !strings.Contains(err.Error(), "platform_oidc.enabled") {
		t.Errorf("error must mention platform_oidc.enabled, got: %v", err)
	}
}

func TestValidateSemantic_Headless_AllConflicts_ReturnsAllErrors(t *testing.T) {
	// All three conflicts at once — must report all three.
	cfg := headlessCfg()
	cfg.Server.ExplorerEnabled = true
	cfg.LocalIAM.Enabled = true
	cfg.PlatformOIDC.Enabled = true
	err := ValidateSemantic(cfg)
	if err == nil {
		t.Fatal("want error for all conflicts, got nil")
	}
	errs, ok := err.(ValidationErrors)
	if !ok {
		t.Fatalf("want ValidationErrors, got %T: %v", err, err)
	}
	if len(errs) < 3 {
		t.Errorf("want at least 3 validation errors (one per conflict), got %d: %v", len(errs), err)
	}
}

func TestValidateSemantic_NotHeadless_ExplorerAndIAMEnabled_Valid(t *testing.T) {
	// headless=false — conflict checks must NOT fire.
	cfg := DefaultConfig()
	cfg.Server.Headless = false
	cfg.Server.ExplorerEnabled = true
	cfg.LocalIAM.Enabled = true
	cfg.PlatformOIDC.Enabled = false
	if err := ValidateSemantic(cfg); err != nil {
		t.Errorf("non-headless with explorer + local_iam: want no error, got %v", err)
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

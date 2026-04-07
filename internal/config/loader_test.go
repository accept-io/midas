package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noDiscovery is a LoadOptions SearchPaths value that prevents automatic file
// discovery. Use it in any test that wants pure-env or pure-defaults behaviour,
// so the repo-root midas.yaml is never accidentally loaded.
var noDiscovery = []string{}

// env returns a fake getenv function for use in LoadOptions.EnvOverride.
func env(pairs ...string) func(string) string {
	m := make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return func(key string) string { return m[key] }
}

// ---------------------------------------------------------------------------
// ParseEnvTokens
// ---------------------------------------------------------------------------

func TestParseEnvTokens_ValidSingle(t *testing.T) {
	entries, err := ParseEnvTokens("mytoken|user:alice|admin,operator")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Token != "mytoken" {
		t.Errorf("token: want %q, got %q", "mytoken", e.Token)
	}
	if e.Principal != "user:alice" {
		t.Errorf("principal: want %q, got %q", "user:alice", e.Principal)
	}
	if e.Roles != "admin,operator" {
		t.Errorf("roles: want %q, got %q", "admin,operator", e.Roles)
	}
}

func TestParseEnvTokens_Multiple(t *testing.T) {
	entries, err := ParseEnvTokens("tok1|user:a|admin;tok2|svc:b|operator")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
}

func TestParseEnvTokens_NoRoles(t *testing.T) {
	entries, err := ParseEnvTokens("tok|user:alice")
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Roles != "" {
		t.Errorf("want empty roles, got %q", entries[0].Roles)
	}
}

func TestParseEnvTokens_SkipsEmptyEntries(t *testing.T) {
	entries, err := ParseEnvTokens("tok|user:a|admin;;  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("want 1 entry, got %d", len(entries))
	}
}

func TestParseEnvTokens_MalformedEntry(t *testing.T) {
	_, err := ParseEnvTokens("just-a-token-no-pipe")
	if err == nil {
		t.Error("want error for malformed entry, got nil")
	}
}

func TestParseEnvTokens_EmptyToken(t *testing.T) {
	_, err := ParseEnvTokens("|user:alice|admin")
	if err == nil {
		t.Error("want error for empty token, got nil")
	}
}

func TestParseEnvTokens_EmptyPrincipal(t *testing.T) {
	_, err := ParseEnvTokens("tok||admin")
	if err == nil {
		t.Error("want error for empty principal, got nil")
	}
}

// ---------------------------------------------------------------------------
// Load — defaults (no file, no env)
// ---------------------------------------------------------------------------

func TestLoad_PureDefaults(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env(),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if result.File != "" {
		t.Errorf("File: want empty (no file loaded), got %q", result.File)
	}
	if result.Config.Store.Backend != "memory" {
		t.Errorf("store.backend: want memory, got %q", result.Config.Store.Backend)
	}
	if result.Config.Auth.Mode != AuthModeOpen {
		t.Errorf("auth.mode: want open, got %q", result.Config.Auth.Mode)
	}
	for _, s := range topLevelSections {
		if result.Sources[s] != SourceDefault {
			t.Errorf("source[%s]: want default, got %s", s, result.Sources[s])
		}
	}
}

// ---------------------------------------------------------------------------
// Load — env var overrides
// ---------------------------------------------------------------------------

func TestLoad_EnvOverridesDefaults(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env(
			"MIDAS_LOG_LEVEL", "debug",
			"MIDAS_SERVER_PORT", "9090",
			"MIDAS_STORE_BACKEND", "memory",
			"MIDAS_AUTH_MODE", "open",
		),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if result.Config.Observability.LogLevel != "debug" {
		t.Errorf("log_level: want debug, got %s", result.Config.Observability.LogLevel)
	}
	if result.Config.Server.Port != 9090 {
		t.Errorf("server.port: want 9090, got %d", result.Config.Server.Port)
	}
	if result.Sources["observability"] != SourceEnv {
		t.Errorf("observability source: want env, got %s", result.Sources["observability"])
	}
	if result.Sources["server"] != SourceEnv {
		t.Errorf("server source: want env, got %s", result.Sources["server"])
	}
}

func TestLoad_AuthTokensFromEnv(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env(
			"MIDAS_AUTH_MODE", "required",
			"MIDAS_AUTH_TOKENS", "tok1|user:alice|admin;tok2|svc:worker|operator",
		),
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Config.Auth.Mode != AuthModeRequired {
		t.Errorf("auth.mode: want required, got %s", result.Config.Auth.Mode)
	}
	if len(result.Config.Auth.Tokens) != 2 {
		t.Errorf("auth.tokens: want 2, got %d", len(result.Config.Auth.Tokens))
	}
}

func TestLoad_KafkaBrokersFromEnv(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env("MIDAS_KAFKA_BROKERS", "broker1:9092, broker2:9092"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Config.Kafka.Brokers) != 2 {
		t.Errorf("kafka.brokers: want 2, got %d", len(result.Config.Kafka.Brokers))
	}
}

func TestLoad_InvalidServerPort(t *testing.T) {
	_, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env("MIDAS_SERVER_PORT", "99999"),
	})
	if err == nil {
		t.Error("want error for out-of-range server port")
	}
}

func TestLoad_InvalidDispatcherEnabled(t *testing.T) {
	_, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env("MIDAS_DISPATCHER_ENABLED", "notabool"),
	})
	if err == nil {
		t.Error("want error for non-boolean dispatcher enabled")
	}
}

// ---------------------------------------------------------------------------
// Load — file loading
// ---------------------------------------------------------------------------

func minimalYAML(port int, logLevel string) string {
	return fmt.Sprintf(`version: 1
profile: dev
server:
  port: %d
store:
  backend: memory
  dsn: ""
auth:
  mode: open
  tokens: []
observability:
  log_level: %s
  log_format: json
control_plane:
  enabled: true
dispatcher:
  enabled: false
  publisher: none
  batch_size: 100
  poll_interval: 2s
  max_backoff: 30s
kafka:
  client_id: midas
  required_acks: -1
  write_timeout: 0s
`, port, logLevel)
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "midas.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_FileOverridesDefaults(t *testing.T) {
	cfgPath := writeConfig(t, minimalYAML(7777, "warn"))

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env(),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if result.Config.Server.Port != 7777 {
		t.Errorf("server.port: want 7777, got %d", result.Config.Server.Port)
	}
	if result.Config.Observability.LogLevel != "warn" {
		t.Errorf("log_level: want warn, got %s", result.Config.Observability.LogLevel)
	}
	if result.Sources["server"] != SourceFile {
		t.Errorf("server source: want file, got %s", result.Sources["server"])
	}
	if result.File != cfgPath {
		t.Errorf("File: want %q, got %q", cfgPath, result.File)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	cfgPath := writeConfig(t, minimalYAML(7777, "info"))

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env("MIDAS_SERVER_PORT", "5000"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Env must win over file.
	if result.Config.Server.Port != 5000 {
		t.Errorf("server.port: want 5000 (env wins), got %d", result.Config.Server.Port)
	}
	if result.Sources["server"] != SourceEnv {
		t.Errorf("server source: want env, got %s", result.Sources["server"])
	}
}

func TestLoad_UnknownFieldsRejected(t *testing.T) {
	cfgPath := writeConfig(t, `
version: 1
profile: dev
completely_unknown_field: surprise
store:
  backend: memory
auth:
  mode: open
`)

	_, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env(),
	})
	if err == nil {
		t.Error("want error for unknown YAML field, got nil")
	}
}

// ---------------------------------------------------------------------------
// Load — inference config
// ---------------------------------------------------------------------------

// TestLoad_InferenceDefaultIsFalse verifies that the default for inference.enabled
// is false when no file and no env are present.
func TestLoad_InferenceDefaultIsFalse(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env(),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Config.Inference.Enabled {
		t.Error("inference.enabled: want false (default), got true")
	}
	if result.Sources["inference"] != SourceDefault {
		t.Errorf("inference source: want default, got %s", result.Sources["inference"])
	}
}

// TestLoad_InferenceEnabledFromEnv verifies that MIDAS_INFERENCE_ENABLED=true
// overrides the default.
func TestLoad_InferenceEnabledFromEnv(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env("MIDAS_INFERENCE_ENABLED", "true"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !result.Config.Inference.Enabled {
		t.Error("inference.enabled: want true (env override), got false")
	}
	if result.Sources["inference"] != SourceEnv {
		t.Errorf("inference source: want env, got %s", result.Sources["inference"])
	}
}

// TestLoad_InferenceInvalidEnvRejected verifies that a non-boolean value for
// MIDAS_INFERENCE_ENABLED produces an error.
func TestLoad_InferenceInvalidEnvRejected(t *testing.T) {
	_, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env("MIDAS_INFERENCE_ENABLED", "notabool"),
	})
	if err == nil {
		t.Error("want error for non-boolean MIDAS_INFERENCE_ENABLED, got nil")
	}
}

// TestLoad_InferenceEnabledFromYAML verifies that inference.enabled: true is
// correctly parsed from a YAML config file.
func TestLoad_InferenceEnabledFromYAML(t *testing.T) {
	cfgPath := writeConfig(t, `
version: 1
profile: dev
store:
  backend: memory
auth:
  mode: open
inference:
  enabled: true
`)
	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env(),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !result.Config.Inference.Enabled {
		t.Error("inference.enabled: want true (from YAML), got false")
	}
	if result.Sources["inference"] != SourceFile {
		t.Errorf("inference source: want file, got %s", result.Sources["inference"])
	}
}

// TestLoad_StructuralModeDefaultIsPermissive verifies that the default structural
// mode is permissive when no file and no env are present.
func TestLoad_StructuralModeDefaultIsPermissive(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env(),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Config.Structural.Mode != StructuralModePermissive {
		t.Errorf("structural.mode: want %q (default), got %q", StructuralModePermissive, result.Config.Structural.Mode)
	}
	if result.Sources["structural"] != SourceDefault {
		t.Errorf("structural source: want default, got %s", result.Sources["structural"])
	}
}

// TestLoad_StructuralModeFromEnv verifies that MIDAS_STRUCTURAL_MODE overrides the default.
func TestLoad_StructuralModeFromEnv(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env("MIDAS_STRUCTURAL_MODE", "enforced"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Config.Structural.Mode != StructuralModeEnforced {
		t.Errorf("structural.mode: want %q (env override), got %q", StructuralModeEnforced, result.Config.Structural.Mode)
	}
	if result.Sources["structural"] != SourceEnv {
		t.Errorf("structural source: want env, got %s", result.Sources["structural"])
	}
}

// TestLoad_StructuralModeFromYAML verifies that structural.mode is parsed from a YAML config file.
func TestLoad_StructuralModeFromYAML(t *testing.T) {
	cfgPath := writeConfig(t, `
version: 1
profile: dev
store:
  backend: memory
auth:
  mode: open
structural:
  mode: enforced
`)
	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env(),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Config.Structural.Mode != StructuralModeEnforced {
		t.Errorf("structural.mode: want %q (from YAML), got %q", StructuralModeEnforced, result.Config.Structural.Mode)
	}
	if result.Sources["structural"] != SourceFile {
		t.Errorf("structural source: want file, got %s", result.Sources["structural"])
	}
}

func TestLoad_PlaceholderExpansion(t *testing.T) {
	cfgPath := writeConfig(t, `
version: 1
profile: dev
store:
  backend: postgres
  dsn: "${TEST_DB_URL}"
auth:
  mode: open
`)

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env("TEST_DB_URL", "postgres://user:pass@localhost:5432/testdb"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	want := "postgres://user:pass@localhost:5432/testdb"
	if result.Config.Store.DSN != want {
		t.Errorf("store.dsn: want %q, got %q", want, result.Config.Store.DSN)
	}
}

func TestLoad_UnresolvedPlaceholderFails(t *testing.T) {
	cfgPath := writeConfig(t, `
version: 1
profile: dev
store:
  backend: postgres
  dsn: "${MISSING_VAR}"
auth:
  mode: open
`)

	_, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env(), // MISSING_VAR not set
	})
	if err == nil {
		t.Error("want error for unresolved placeholder, got nil")
	}
}

func TestLoad_MIDASSConfigEnvVar(t *testing.T) {
	cfgPath := writeConfig(t, minimalYAML(6789, "info"))

	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env("MIDAS_CONFIG", cfgPath),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Config.Server.Port != 6789 {
		t.Errorf("server.port: want 6789, got %d", result.Config.Server.Port)
	}
	if result.File != cfgPath {
		t.Errorf("File: want %q, got %q", cfgPath, result.File)
	}
}

// ---------------------------------------------------------------------------
// Fix 1: Placeholder expansion timing — AFTER env overlay
// ---------------------------------------------------------------------------

// TestLoad_EnvOverridePlaceholder_DoesNotExpandUnusedPlaceholder verifies that
// a file field containing an unresolvable ${UNSET_VAR} placeholder does NOT
// cause a failure when an env var override replaces that same field. The
// placeholder is never reached by expansion because the env overlay wins first.
func TestLoad_EnvOverridePlaceholder_DoesNotExpandUnusedPlaceholder(t *testing.T) {
	cfgPath := writeConfig(t, `
version: 1
profile: dev
store:
  backend: postgres
  dsn: "${UNSET_VAR}"
auth:
  mode: open
`)

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env("MIDAS_DATABASE_URL", "postgres://actual:pass@localhost/db"),
	})
	if err != nil {
		t.Fatalf("Load failed (env override should suppress unresolved placeholder): %v", err)
	}

	want := "postgres://actual:pass@localhost/db"
	if result.Config.Store.DSN != want {
		t.Errorf("store.dsn: want %q, got %q", want, result.Config.Store.DSN)
	}
}

// TestLoad_PlaceholderExpanded_AfterEnvOverlay verifies that a resolvable
// ${VAR} placeholder in a file field is correctly expanded after env overlay.
func TestLoad_PlaceholderExpanded_AfterEnvOverlay(t *testing.T) {
	cfgPath := writeConfig(t, `
version: 1
profile: dev
store:
  backend: postgres
  dsn: "${MY_DB_URL}"
auth:
  mode: open
`)

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env("MY_DB_URL", "postgres://user:secret@db:5432/mydb"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	want := "postgres://user:secret@db:5432/mydb"
	if result.Config.Store.DSN != want {
		t.Errorf("store.dsn: want %q, got %q", want, result.Config.Store.DSN)
	}
}

// TestLoad_UnresolvedPlaceholder_FailsWhenActive verifies that an unresolvable
// placeholder in a field that is NOT overridden by an env var causes a hard
// error with a clear message identifying the field.
func TestLoad_UnresolvedPlaceholder_FailsWhenActive(t *testing.T) {
	cfgPath := writeConfig(t, `
version: 1
profile: dev
store:
  backend: postgres
  dsn: "${UNSET_VAR}"
auth:
  mode: open
`)

	_, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env(), // UNSET_VAR not set, no MIDAS_DATABASE_URL override
	})
	if err == nil {
		t.Fatal("want error for unresolved placeholder with no env override, got nil")
	}
	if !strings.Contains(err.Error(), "store.dsn") {
		t.Errorf("error should identify field path: got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Fix 2: Env var naming — MIDAS_STORE_BACKEND not MIDAS_STORE_KIND
// ---------------------------------------------------------------------------

// TestLoad_MIDAS_STORE_BACKEND_Recognized verifies the canonical env var name
// MIDAS_STORE_BACKEND overrides store.backend.
func TestLoad_MIDAS_STORE_BACKEND_Recognized(t *testing.T) {
	cfgPath := writeConfig(t, minimalYAML(8080, "info"))

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env("MIDAS_STORE_BACKEND", "postgres"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Config.Store.Backend != "postgres" {
		t.Errorf("store.backend: want postgres (env wins), got %q", result.Config.Store.Backend)
	}
	if result.Sources["store"] != SourceEnv {
		t.Errorf("store source: want env, got %s", result.Sources["store"])
	}
}

// TestLoad_MIDAS_STORE_KIND_NotRecognized verifies that the legacy/incorrect
// env var name MIDAS_STORE_KIND has no effect on store.backend.
func TestLoad_MIDAS_STORE_KIND_NotRecognized(t *testing.T) {
	cfgPath := writeConfig(t, minimalYAML(8080, "info"))

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env("MIDAS_STORE_KIND", "postgres"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// MIDAS_STORE_KIND must be silently ignored — file value wins
	if result.Config.Store.Backend != "memory" {
		t.Errorf("store.backend: want memory (MIDAS_STORE_KIND ignored), got %q", result.Config.Store.Backend)
	}
	if result.Sources["store"] != SourceFile {
		t.Errorf("store source: want file (env var unknown), got %s", result.Sources["store"])
	}
}

// ---------------------------------------------------------------------------
// New env vars — local_iam, platform_oidc, control_plane
// ---------------------------------------------------------------------------

// localIAMYAML returns a minimal YAML config that includes a local_iam section.
func localIAMYAML(enabled bool) string {
	return fmt.Sprintf(`version: 1
profile: dev
local_iam:
  enabled: %v
  session_ttl: 8h
  secure_cookies: false
`, enabled)
}

// platformOIDCYAML returns a minimal YAML config that includes a platform_oidc section.
func platformOIDCYAML(enabled bool, clientID, clientSecret string) string {
	return fmt.Sprintf(`version: 1
profile: dev
platform_oidc:
  enabled: %v
  client_id: %q
  client_secret: %q
  subject_claim: sub
  username_claim: preferred_username
  groups_claim: groups
  deny_if_no_roles: true
  use_pkce: true
`, enabled, clientID, clientSecret)
}

func TestLoad_MIDAS_LOCAL_IAM_ENABLED_FalseOverridesDefault(t *testing.T) {
	// Default is local_iam.enabled=true; env sets it false (headless/API-only opt-out).
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env("MIDAS_LOCAL_IAM_ENABLED", "false"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Config.LocalIAM.Enabled {
		t.Error("local_iam.enabled: want false (env wins over default), got true")
	}
	if result.Sources["local_iam"] != SourceEnv {
		t.Errorf("local_iam source: want env, got %s", result.Sources["local_iam"])
	}
}

func TestLoad_MIDAS_LOCAL_IAM_ENABLED_OverridesFile(t *testing.T) {
	// File has local_iam.enabled=false; env sets it true.
	cfgPath := writeConfig(t, localIAMYAML(false))

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env("MIDAS_LOCAL_IAM_ENABLED", "true"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !result.Config.LocalIAM.Enabled {
		t.Error("local_iam.enabled: want true (env wins), got false")
	}
	if result.Sources["local_iam"] != SourceEnv {
		t.Errorf("local_iam source: want env, got %s", result.Sources["local_iam"])
	}
}

func TestLoad_MIDAS_OIDC_ENABLED_OverridesFile(t *testing.T) {
	// File has platform_oidc.enabled=false; env sets it true.
	cfgPath := writeConfig(t, platformOIDCYAML(false, "cid", "csecret"))

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env("MIDAS_OIDC_ENABLED", "true"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !result.Config.PlatformOIDC.Enabled {
		t.Error("platform_oidc.enabled: want true (env wins), got false")
	}
	if result.Sources["platform_oidc"] != SourceEnv {
		t.Errorf("platform_oidc source: want env, got %s", result.Sources["platform_oidc"])
	}
}

func TestLoad_MIDAS_OIDC_CLIENT_ID_OverridesFile(t *testing.T) {
	cfgPath := writeConfig(t, platformOIDCYAML(false, "from-file-id", "csecret"))

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env("MIDAS_OIDC_CLIENT_ID", "from-env-id"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Config.PlatformOIDC.ClientID != "from-env-id" {
		t.Errorf("client_id: want from-env-id (env wins), got %q", result.Config.PlatformOIDC.ClientID)
	}
	if result.Sources["platform_oidc"] != SourceEnv {
		t.Errorf("platform_oidc source: want env, got %s", result.Sources["platform_oidc"])
	}
}

func TestLoad_MIDAS_OIDC_CLIENT_SECRET_OverridesFile(t *testing.T) {
	cfgPath := writeConfig(t, platformOIDCYAML(false, "cid", "file-secret"))

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env("MIDAS_OIDC_CLIENT_SECRET", "env-secret"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Config.PlatformOIDC.ClientSecret != "env-secret" {
		t.Errorf("client_secret: want env-secret (env wins), got %q", result.Config.PlatformOIDC.ClientSecret)
	}
	// The secret value must not appear in Sources — Sources only contains
	// the string values "default", "file", or "env", never config values.
	for k, v := range result.Sources {
		if strings.Contains(string(v), "env-secret") {
			t.Errorf("secret value found in Sources[%s]=%q — must never appear in introspection", k, v)
		}
	}
}

func TestLoad_MIDAS_CONTROL_PLANE_ENABLED_OverridesDefault(t *testing.T) {
	// Default is true; env disables it.
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env("MIDAS_CONTROL_PLANE_ENABLED", "false"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Config.ControlPlane.Enabled {
		t.Error("control_plane.enabled: want false (env wins over default true), got true")
	}
	if result.Sources["control_plane"] != SourceEnv {
		t.Errorf("control_plane source: want env, got %s", result.Sources["control_plane"])
	}
}

func TestLoad_FileValues_UsedWhenEnvAbsent(t *testing.T) {
	// File sets local_iam.enabled=true, platform_oidc.enabled=true with a client_id.
	// No env vars. File values must be used.
	combined := `version: 1
profile: dev
local_iam:
  enabled: true
  session_ttl: 4h
  secure_cookies: true
platform_oidc:
  enabled: true
  client_id: "file-only-client-id"
  subject_claim: sub
  username_claim: email
  groups_claim: hd
  deny_if_no_roles: true
  use_pkce: true
`
	cfgPath := writeConfig(t, combined)

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env(), // no env vars
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !result.Config.LocalIAM.Enabled {
		t.Error("local_iam.enabled: want true (from file), got false")
	}
	if !result.Config.PlatformOIDC.Enabled {
		t.Error("platform_oidc.enabled: want true (from file), got false")
	}
	if result.Config.PlatformOIDC.ClientID != "file-only-client-id" {
		t.Errorf("client_id: want file-only-client-id, got %q", result.Config.PlatformOIDC.ClientID)
	}
	if result.Sources["local_iam"] != SourceFile {
		t.Errorf("local_iam source: want file, got %s", result.Sources["local_iam"])
	}
	if result.Sources["platform_oidc"] != SourceFile {
		t.Errorf("platform_oidc source: want file, got %s", result.Sources["platform_oidc"])
	}
}

func TestLoad_DirectEnvAndPlaceholder_WorkTogether(t *testing.T) {
	// File has client_id (literal) and client_secret as a placeholder.
	// MIDAS_OIDC_CLIENT_ID overrides client_id via direct env.
	// MY_OIDC_SECRET is expanded into client_secret via placeholder expansion.
	// Both mechanisms must work independently in the same Load call.
	cfgPath := writeConfig(t, `version: 1
profile: dev
platform_oidc:
  enabled: false
  client_id: "file-client-id"
  client_secret: "${MY_OIDC_SECRET}"
  subject_claim: sub
  username_claim: preferred_username
  groups_claim: groups
  deny_if_no_roles: true
  use_pkce: true
`)

	result, err := Load(LoadOptions{
		ConfigFile: cfgPath,
		EnvOverride: env(
			"MIDAS_OIDC_CLIENT_ID", "env-client-id",  // direct env override
			"MY_OIDC_SECRET", "placeholder-secret",   // used for ${VAR} expansion
		),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// Direct env override wins over file value for client_id.
	if result.Config.PlatformOIDC.ClientID != "env-client-id" {
		t.Errorf("client_id: want env-client-id (env override), got %q", result.Config.PlatformOIDC.ClientID)
	}
	// Placeholder expansion resolves client_secret.
	if result.Config.PlatformOIDC.ClientSecret != "placeholder-secret" {
		t.Errorf("client_secret: want placeholder-secret (${MY_OIDC_SECRET} expanded), got %q", result.Config.PlatformOIDC.ClientSecret)
	}
}

func TestLoad_LocalIAMAndPlatformOIDC_InSources_PureDefaults(t *testing.T) {
	// With no file and no env, both sections must appear in Sources as default.
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env(),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Sources["local_iam"] != SourceDefault {
		t.Errorf("local_iam source: want default, got %s", result.Sources["local_iam"])
	}
	if result.Sources["platform_oidc"] != SourceDefault {
		t.Errorf("platform_oidc source: want default, got %s", result.Sources["platform_oidc"])
	}
}

func TestLoad_LocalIAMAndPlatformOIDC_InSources_FromFile(t *testing.T) {
	// When a file includes local_iam, its source must be recorded as file.
	cfgPath := writeConfig(t, localIAMYAML(true))

	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env(),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// Any section present in topLevelSections is marked SourceFile when a file is loaded.
	if result.Sources["local_iam"] != SourceFile {
		t.Errorf("local_iam source: want file, got %s", result.Sources["local_iam"])
	}
	if result.Sources["platform_oidc"] != SourceFile {
		t.Errorf("platform_oidc source: want file (file loaded), got %s", result.Sources["platform_oidc"])
	}
}

// ---------------------------------------------------------------------------
// Headless mode — env var and loader
// ---------------------------------------------------------------------------

func TestLoad_ServerHeadless_DefaultsFalse(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env(),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if result.Config.Server.Headless {
		t.Error("server.headless: want false (default), got true")
	}
}

func TestLoad_MIDAS_SERVER_HEADLESS_OverridesDefault(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env("MIDAS_SERVER_HEADLESS", "true"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !result.Config.Server.Headless {
		t.Error("server.headless: want true (from MIDAS_SERVER_HEADLESS), got false")
	}
	if result.Sources["server"] != SourceEnv {
		t.Errorf("server source: want env, got %s", result.Sources["server"])
	}
}

func TestLoad_MIDAS_SERVER_HEADLESS_OverridesFile(t *testing.T) {
	cfgPath := writeConfig(t, `version: 1
profile: dev
server:
  headless: false
  port: 8080
  explorer_enabled: false
`)
	result, err := Load(LoadOptions{
		ConfigFile:  cfgPath,
		EnvOverride: env("MIDAS_SERVER_HEADLESS", "true"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !result.Config.Server.Headless {
		t.Error("server.headless: want true (env wins over file), got false")
	}
}

func TestLoad_HeadlessWithLocalIAMEnabled_TriggersFatalValidation(t *testing.T) {
	// Confirms that validation runs after env overrides are applied.
	// MIDAS_SERVER_HEADLESS=true combined with MIDAS_LOCAL_IAM_ENABLED=true
	// must produce a ValidateSemantic error — proving the check runs on the merged config.
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env(
			"MIDAS_SERVER_HEADLESS", "true",
			"MIDAS_LOCAL_IAM_ENABLED", "true",
		),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// Verify both env vars were applied.
	if !result.Config.Server.Headless {
		t.Error("server.headless: want true (from env)")
	}
	if !result.Config.LocalIAM.Enabled {
		t.Error("local_iam.enabled: want true (from env)")
	}
	// Now validate — this must return a conflict error.
	if err := ValidateSemantic(result.Config); err == nil {
		t.Error("ValidateSemantic: want error for headless + local_iam.enabled, got nil")
	} else if !strings.Contains(err.Error(), "server.headless") || !strings.Contains(err.Error(), "local_iam.enabled") {
		t.Errorf("error message must include both conflicting keys, got: %v", err)
	}
}

func TestLoad_InvalidBooleanEnvVars_ReturnErrors(t *testing.T) {
	tests := []struct {
		name   string
		envKey string
	}{
		{"MIDAS_LOCAL_IAM_ENABLED", "MIDAS_LOCAL_IAM_ENABLED"},
		{"MIDAS_OIDC_ENABLED", "MIDAS_OIDC_ENABLED"},
		{"MIDAS_CONTROL_PLANE_ENABLED", "MIDAS_CONTROL_PLANE_ENABLED"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(LoadOptions{
				SearchPaths: noDiscovery,
				EnvOverride: env(tc.envKey, "notabool"),
			})
			if err == nil {
				t.Errorf("%s=notabool: want error, got nil", tc.envKey)
			}
		})
	}
}

func TestLoad_MIDAS_DEV_SEED_DEMO_USER_SetsFlag(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
		EnvOverride: env("MIDAS_DEV_SEED_DEMO_USER", "true"),
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !result.Config.Dev.SeedDemoUser {
		t.Error("dev.seed_demo_user: want true (from env), got false")
	}
	if result.Sources["dev"] != SourceEnv {
		t.Errorf("dev source: want env, got %s", result.Sources["dev"])
	}
}

func TestLoad_MIDAS_DEV_SEED_DEMO_USER_DefaultTrue(t *testing.T) {
	result, err := Load(LoadOptions{
		SearchPaths: noDiscovery,
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !result.Config.Dev.SeedDemoUser {
		t.Error("dev.seed_demo_user: want true by default, got false")
	}
}


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


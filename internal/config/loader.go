package config

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Source identifies where a configuration value came from.
type Source string

const (
	SourceDefault Source = "default"
	SourceFile    Source = "file"
	SourceEnv     Source = "env"
)

// LoadOptions controls how Load discovers and reads configuration.
type LoadOptions struct {
	// ConfigFile is an explicit path to the YAML config file.
	// When set, file discovery is skipped. Pass "" to use automatic discovery.
	ConfigFile string

	// SearchPaths overrides the default file discovery path list.
	// nil → use candidatePaths (default).
	// []string{} (empty, non-nil) → skip file discovery entirely.
	// Useful in tests to prevent accidentally loading the repo-root midas.yaml.
	SearchPaths []string

	// EnvOverride replaces os.Getenv for all environment variable reads.
	// Nil means use os.Getenv. Set in tests to supply a fake environment.
	EnvOverride func(string) string
}

// LoadResult is the output of a successful Load call.
type LoadResult struct {
	// Config is the fully merged configuration (defaults → file → env).
	Config Config

	// Sources tracks which layer each top-level section was last written by.
	// Keys are YAML field names; values are SourceDefault, SourceFile, SourceEnv.
	Sources map[string]Source

	// File is the path to the config file that was read, or "" if none was found.
	File string

	// Warnings are non-fatal notices generated during loading
	// (e.g. deprecated field detected).
	Warnings []string
}

// candidatePaths lists config file locations searched in order when no
// explicit path is supplied.
var candidatePaths = []string{
	"midas.yaml",
	"midas.yml",
	"/etc/midas/midas.yaml",
	"/etc/midas/midas.yml",
}

// topLevelSections is the set of top-level YAML field names tracked in Sources.
var topLevelSections = []string{
	"version", "profile", "server", "store", "auth",
	"local_iam", "platform_oidc",
	"observability", "control_plane", "dev", "dispatcher", "kafka", "structural",
}

// Load discovers, reads, and returns the merged Config.
//
// Precedence (highest to lowest):
//  1. Environment variable overrides (MIDAS_* vars)
//  2. Config file (midas.yaml or explicit path)
//  3. Built-in defaults
//
// Placeholder expansion (${VAR}) is applied AFTER env overlay so that fields
// overridden by MIDAS_* env vars are never expanded as placeholders — an
// unresolved placeholder in a file field that gets replaced by an env var
// never causes a startup failure. Structural and semantic validation are the
// caller's responsibility.
func Load(opts LoadOptions) (LoadResult, error) {
	getenv := os.Getenv
	if opts.EnvOverride != nil {
		getenv = opts.EnvOverride
	}

	result := LoadResult{
		Config:  DefaultConfig(),
		Sources: make(map[string]Source),
	}
	for _, s := range topLevelSections {
		result.Sources[s] = SourceDefault
	}

	// --- Resolve config file path ---

	filePath := opts.ConfigFile
	if filePath == "" {
		if v := getenv("MIDAS_CONFIG"); v != "" {
			filePath = v
		}
	}
	if filePath == "" {
		paths := candidatePaths
		if opts.SearchPaths != nil {
			paths = opts.SearchPaths
		}
		filePath = discoverConfigFile(paths)
	}

	// --- Load and parse config file ---

	if filePath != "" {
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return LoadResult{}, fmt.Errorf("config: read %q: %w", filePath, err)
		}

		// Parse raw YAML without placeholder expansion. ${VAR} strings are
		// treated as literal values at this stage; expansion happens below,
		// after env overlay, so that fields overridden by MIDAS_* env vars
		// are never expanded as placeholders.
		fileCfg := DefaultConfig()
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true) // reject unknown fields: config drift is a hard error
		if err := dec.Decode(&fileCfg); err != nil {
			return LoadResult{}, fmt.Errorf("config: parse %q: %w", filePath, err)
		}

		result.Config = fileCfg
		result.File = filePath
		for _, s := range topLevelSections {
			result.Sources[s] = SourceFile
		}
	}

	// --- Apply environment variable overrides ---

	if err := applyEnvOverrides(&result.Config, result.Sources, getenv); err != nil {
		return LoadResult{}, err
	}

	// --- Expand placeholders on the final effective config ---
	// Any field overridden by an env var above is already a resolved value,
	// so its original placeholder (if any) is gone and never triggers an error.

	if err := expandConfigPlaceholders(&result.Config, getenv); err != nil {
		return LoadResult{}, fmt.Errorf("config: %w", err)
	}

	return result, nil
}

// discoverConfigFile walks paths and returns the first path that exists.
func discoverConfigFile(paths []string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// applyEnvOverrides writes recognised MIDAS_* environment variables into cfg,
// updating sources to SourceEnv for each section that is touched.
func applyEnvOverrides(cfg *Config, sources map[string]Source, getenv func(string) string) error {
	markEnv := func(section string) { sources[section] = SourceEnv }

	// Profile
	if v := getenv("MIDAS_PROFILE"); v != "" {
		cfg.Profile = Profile(strings.ToLower(strings.TrimSpace(v)))
		markEnv("profile")
	}

	// Store
	if v := getenv("MIDAS_STORE_BACKEND"); v != "" {
		cfg.Store.Backend = strings.ToLower(strings.TrimSpace(v))
		markEnv("store")
	}
	if v := getenv("MIDAS_DATABASE_URL"); v != "" {
		cfg.Store.DSN = v
		markEnv("store")
	}

	// Auth
	if v := getenv("MIDAS_AUTH_MODE"); v != "" {
		cfg.Auth.Mode = AuthMode(strings.ToLower(strings.TrimSpace(v)))
		markEnv("auth")
	}
	if v := getenv("MIDAS_AUTH_TOKENS"); v != "" {
		tokens, err := ParseEnvTokens(v)
		if err != nil {
			return err
		}
		cfg.Auth.Tokens = tokens
		markEnv("auth")
	}

	// Observability
	if v := getenv("MIDAS_LOG_LEVEL"); v != "" {
		cfg.Observability.LogLevel = strings.ToLower(strings.TrimSpace(v))
		markEnv("observability")
	}
	if v := getenv("MIDAS_LOG_FORMAT"); v != "" {
		cfg.Observability.LogFormat = strings.ToLower(strings.TrimSpace(v))
		markEnv("observability")
	}

	// Server
	if v := getenv("MIDAS_SERVER_HEADLESS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("config: MIDAS_SERVER_HEADLESS must be a boolean (true/false): %q", v)
		}
		cfg.Server.Headless = b
		markEnv("server")
	}
	if v := getenv("MIDAS_SERVER_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			return fmt.Errorf("config: MIDAS_SERVER_PORT must be a port number (1-65535): %q", v)
		}
		cfg.Server.Port = n
		markEnv("server")
	}
	if v := getenv("MIDAS_EXPLORER_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("config: MIDAS_EXPLORER_ENABLED must be a boolean (true/false): %q", v)
		}
		cfg.Server.ExplorerEnabled = b
		markEnv("server")
	}

	// Dev
	if v := getenv("MIDAS_DEV_SEED_DEMO_DATA"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("config: MIDAS_DEV_SEED_DEMO_DATA must be a boolean (true/false): %q", v)
		}
		cfg.Dev.SeedDemoData = b
		markEnv("dev")
	}
	if v := getenv("MIDAS_DEV_SEED_DEMO_USER"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("config: MIDAS_DEV_SEED_DEMO_USER must be a boolean (true/false): %q", v)
		}
		cfg.Dev.SeedDemoUser = b
		markEnv("dev")
	}

	// Dispatcher
	if v := getenv("MIDAS_DISPATCHER_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("config: MIDAS_DISPATCHER_ENABLED must be a boolean (true/false): %q", v)
		}
		cfg.Dispatcher.Enabled = b
		markEnv("dispatcher")
	}
	if v := getenv("MIDAS_DISPATCHER_PUBLISHER"); v != "" {
		cfg.Dispatcher.Publisher = strings.ToLower(strings.TrimSpace(v))
		markEnv("dispatcher")
	}
	if v := getenv("MIDAS_DISPATCHER_BATCH_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return fmt.Errorf("config: MIDAS_DISPATCHER_BATCH_SIZE must be a positive integer: %q", v)
		}
		cfg.Dispatcher.BatchSize = n
		markEnv("dispatcher")
	}

	// Local IAM
	if v := getenv("MIDAS_LOCAL_IAM_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("config: MIDAS_LOCAL_IAM_ENABLED must be a boolean (true/false): %q", v)
		}
		cfg.LocalIAM.Enabled = b
		markEnv("local_iam")
	}

	// Platform OIDC
	if v := getenv("MIDAS_OIDC_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("config: MIDAS_OIDC_ENABLED must be a boolean (true/false): %q", v)
		}
		cfg.PlatformOIDC.Enabled = b
		markEnv("platform_oidc")
	}
	if v := getenv("MIDAS_OIDC_CLIENT_ID"); v != "" {
		cfg.PlatformOIDC.ClientID = v
		markEnv("platform_oidc")
	}
	if v := getenv("MIDAS_OIDC_CLIENT_SECRET"); v != "" {
		cfg.PlatformOIDC.ClientSecret = v
		markEnv("platform_oidc")
	}

	// Control plane
	if v := getenv("MIDAS_CONTROL_PLANE_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("config: MIDAS_CONTROL_PLANE_ENABLED must be a boolean (true/false): %q", v)
		}
		cfg.ControlPlane.Enabled = b
		markEnv("control_plane")
	}

	// Structural
	if v := getenv("MIDAS_STRUCTURAL_MODE"); v != "" {
		cfg.Structural.Mode = StructuralMode(strings.ToLower(strings.TrimSpace(v)))
		markEnv("structural")
	}

	// Kafka
	if v := getenv("MIDAS_KAFKA_BROKERS"); v != "" {
		var brokers []string
		for _, b := range strings.Split(v, ",") {
			b = strings.TrimSpace(b)
			if b != "" {
				brokers = append(brokers, b)
			}
		}
		cfg.Kafka.Brokers = brokers
		markEnv("kafka")
	}

	return nil
}

// ParseEnvTokens parses the MIDAS_AUTH_TOKENS wire format into []TokenEntry.
//
// Format: semicolon-separated entries, each: token|principal-id|role1,role2
//
// The pipe separator allows principal IDs to contain colons (e.g. "user:alice").
//
// Example:
//
//	"secret-1|user:alice|admin,operator;secret-2|svc:deploy|operator"
func ParseEnvTokens(raw string) ([]TokenEntry, error) {
	var entries []TokenEntry
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.SplitN(entry, "|", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("config: malformed token entry %q (want token|principal-id[|roles])", entry)
		}

		token := strings.TrimSpace(parts[0])
		principal := strings.TrimSpace(parts[1])

		if token == "" {
			return nil, fmt.Errorf("config: empty token in entry %q", entry)
		}
		if principal == "" {
			return nil, fmt.Errorf("config: empty principal in entry %q", entry)
		}

		var roles string
		if len(parts) == 3 {
			roles = strings.TrimSpace(parts[2])
		}

		entries = append(entries, TokenEntry{
			Token:     token,
			Principal: principal,
			Roles:     roles,
		})
	}
	return entries, nil
}

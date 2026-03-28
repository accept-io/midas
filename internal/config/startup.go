package config

import (
	"log/slog"
	"strings"
)

// LogStartupSummary emits a structured log line summarising the active
// configuration. Secrets are masked:
//   - Store DSN: host and database only; credentials stripped.
//   - Auth tokens: count and principal IDs logged; token values never appear.
//   - OIDC client_secret: never logged under any circumstances.
func LogStartupSummary(result LoadResult) {
	cfg := result.Config

	slog.Info("midas_config_loaded",
		"file", fileOrNone(result.File),
		"profile", string(cfg.Profile),
		"store_backend", cfg.Store.Backend,
		"store_dsn_masked", maskDSN(cfg.Store.DSN),
		"auth_mode", string(cfg.Auth.Mode),
		"auth_token_count", len(cfg.Auth.Tokens),
		"auth_principals", principals(cfg.Auth.Tokens),
		"log_level", cfg.Observability.LogLevel,
		"log_format", cfg.Observability.LogFormat,
		"server_port", cfg.Server.Port,
		"server_headless", cfg.Server.Headless,
		"explorer_enabled", cfg.Server.ExplorerEnabled,
		"control_plane_enabled", cfg.ControlPlane.Enabled,
		"local_iam_enabled", cfg.LocalIAM.Enabled,
		"oidc_enabled", cfg.PlatformOIDC.Enabled,
		"oidc_provider", cfg.PlatformOIDC.ProviderName,
		"dispatcher_enabled", cfg.Dispatcher.Enabled,
		"dispatcher_publisher", cfg.Dispatcher.Publisher,
		"source_profile", string(result.Sources["profile"]),
		"source_store", string(result.Sources["store"]),
		"source_auth", string(result.Sources["auth"]),
		"source_local_iam", string(result.Sources["local_iam"]),
		"source_platform_oidc", string(result.Sources["platform_oidc"]),
		"warnings", result.Warnings,
	)

	if cfg.Server.Headless {
		slog.Info("midas_headless_mode",
			"headless", true,
			"explorer", "disabled",
			"local_iam", "disabled",
			"oidc", "disabled",
			"detail", "running as pure governance engine; /v1/*, /healthz, /readyz remain operational",
		)
	}
}

// maskDSN extracts the host[:port]/database segment from a Postgres connection
// string, discarding credentials and query parameters.
// Returns "" for an empty input.
func maskDSN(dsn string) string {
	if dsn == "" {
		return ""
	}

	s := dsn
	// Strip scheme: postgres:// or postgresql://
	s = strings.TrimPrefix(s, "postgresql://")
	s = strings.TrimPrefix(s, "postgres://")

	// Strip userinfo (everything up to and including the last @).
	if idx := strings.LastIndex(s, "@"); idx >= 0 {
		s = s[idx+1:]
	}

	// Strip query string.
	if idx := strings.Index(s, "?"); idx >= 0 {
		s = s[:idx]
	}

	return s
}

// principals returns the principal ID from each TokenEntry for logging.
// Never returns the token value.
func principals(tokens []TokenEntry) []string {
	if len(tokens) == 0 {
		return nil
	}
	out := make([]string, len(tokens))
	for i, t := range tokens {
		out[i] = t.Principal
	}
	return out
}

func fileOrNone(f string) string {
	if f == "" {
		return "<none>"
	}
	return f
}

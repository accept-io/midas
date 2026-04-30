package config

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq" // postgres driver for operational validation
)

// ValidationError describes a single validation failure with a field path.
type ValidationError struct {
	Field   string // e.g. "auth.mode" or "auth.tokens[0].token"
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("config: %s: %s", e.Field, e.Message)
}

// ValidationErrors is a slice of ValidationError returned as a single error.
// It implements the error interface so callers can treat it as a fatal startup
// failure without unwrapping.
type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
	msgs := make([]string, len(ve))
	for i, e := range ve {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

// ValidateStructural checks that all field values are within the defined
// enumeration of valid values. It does not check cross-field consistency
// (that is ValidateSemantic's job) and does not open any connections.
//
// Checks:
//   - version is 0 (unset) or CurrentVersion
//   - profile is "dev" or "production"
//   - auth.mode is "open" or "required"
//   - store.backend is "memory" or "postgres"
//   - observability.log_level is debug/info/warn/error
//   - observability.log_format is "json" or "text"
//   - server.port is 1–65535
func ValidateStructural(cfg Config) error {
	var errs ValidationErrors

	if cfg.Version != 0 && cfg.Version != CurrentVersion {
		errs = append(errs, ValidationError{
			"version",
			fmt.Sprintf("unsupported version %d (supported: %d)", cfg.Version, CurrentVersion),
		})
	}

	switch cfg.Profile {
	case ProfileDev, ProfileProduction:
	default:
		errs = append(errs, ValidationError{
			"profile",
			fmt.Sprintf("must be %q or %q, got %q", ProfileDev, ProfileProduction, cfg.Profile),
		})
	}

	switch cfg.Auth.Mode {
	case AuthModeOpen, AuthModeRequired:
	default:
		errs = append(errs, ValidationError{
			"auth.mode",
			fmt.Sprintf("must be %q or %q, got %q", AuthModeOpen, AuthModeRequired, cfg.Auth.Mode),
		})
	}

	switch cfg.Store.Backend {
	case "memory", "postgres":
	default:
		errs = append(errs, ValidationError{
			"store.backend",
			fmt.Sprintf(`must be "memory" or "postgres", got %q`, cfg.Store.Backend),
		})
	}

	switch strings.ToLower(cfg.Observability.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, ValidationError{
			"observability.log_level",
			fmt.Sprintf("must be debug/info/warn/error, got %q", cfg.Observability.LogLevel),
		})
	}

	switch strings.ToLower(cfg.Observability.LogFormat) {
	case "json", "text":
	default:
		errs = append(errs, ValidationError{
			"observability.log_format",
			fmt.Sprintf(`must be "json" or "text", got %q`, cfg.Observability.LogFormat),
		})
	}

	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		errs = append(errs, ValidationError{
			"server.port",
			fmt.Sprintf("must be 1–65535, got %d", cfg.Server.Port),
		})
	}

	switch cfg.Structural.Mode {
	case StructuralModePermissive, StructuralModeEnforced:
	case "": // unset: treated as permissive; not a config error
	default:
		errs = append(errs, ValidationError{
			"structural.mode",
			fmt.Sprintf("must be %q or %q, got %q", StructuralModePermissive, StructuralModeEnforced, cfg.Structural.Mode),
		})
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// ValidateSemantic checks cross-field business rules that cannot be expressed
// as single-field constraints. It does not open any connections.
//
// Checks:
//   - postgres store requires a non-empty DSN
//   - production profile requires postgres store + required auth mode
//   - required auth mode requires at least one token entry
//   - each token entry must have non-empty token and principal fields
//   - dispatcher.enabled=true requires a real publisher (not "none" or "")
//   - dispatcher.publisher=kafka requires at least one broker
func ValidateSemantic(cfg Config) error {
	var errs ValidationErrors

	// Postgres requires a DSN.
	if cfg.Store.Backend == "postgres" && strings.TrimSpace(cfg.Store.DSN) == "" {
		errs = append(errs, ValidationError{
			"store.dsn",
			`required when store.backend is "postgres"`,
		})
	}

	// Production profile enforces both store and auth.
	if cfg.Profile == ProfileProduction {
		if cfg.Store.Backend != "postgres" {
			errs = append(errs, ValidationError{
				"store.backend",
				`production profile requires "postgres"`,
			})
		}
		if cfg.Auth.Mode != AuthModeRequired {
			errs = append(errs, ValidationError{
				"auth.mode",
				`production profile requires "required"`,
			})
		}
		// Local IAM cookies must carry the Secure attribute in production. The
		// rule is gated on local_iam.enabled because a headless production
		// deployment writes no Local IAM cookies.
		if cfg.LocalIAM.Enabled && !cfg.LocalIAM.SecureCookies {
			errs = append(errs, ValidationError{
				"local_iam.secure_cookies",
				`production profile requires "true" when local_iam.enabled is true`,
			})
		}
	}

	// Required auth needs at least one token.
	if cfg.Auth.Mode == AuthModeRequired && len(cfg.Auth.Tokens) == 0 {
		errs = append(errs, ValidationError{
			"auth.tokens",
			`at least one token entry required when auth.mode is "required"`,
		})
	}

	// Each token entry must be complete.
	for i, t := range cfg.Auth.Tokens {
		if strings.TrimSpace(t.Token) == "" {
			errs = append(errs, ValidationError{
				fmt.Sprintf("auth.tokens[%d].token", i),
				"must not be empty",
			})
		}
		if strings.TrimSpace(t.Principal) == "" {
			errs = append(errs, ValidationError{
				fmt.Sprintf("auth.tokens[%d].principal", i),
				"must not be empty",
			})
		}
	}

	// Headless mode conflicts: headless=true is incompatible with browser-facing features.
	if cfg.Server.Headless {
		if cfg.Server.ExplorerEnabled {
			errs = append(errs, ValidationError{
				"server.headless",
				"server.headless=true conflicts with server.explorer_enabled=true",
			})
		}
		if cfg.LocalIAM.Enabled {
			errs = append(errs, ValidationError{
				"server.headless",
				"server.headless=true conflicts with local_iam.enabled=true",
			})
		}
		if cfg.PlatformOIDC.Enabled {
			errs = append(errs, ValidationError{
				"server.headless",
				"server.headless=true conflicts with platform_oidc.enabled=true",
			})
		}
	}

	// Dispatcher: enabled requires a real publisher.
	if cfg.Dispatcher.Enabled {
		switch cfg.Dispatcher.Publisher {
		case "none", "":
			errs = append(errs, ValidationError{
				"dispatcher.publisher",
				`must be a real publisher when dispatcher.enabled is true (e.g. "kafka")`,
			})
		case "kafka":
			if len(cfg.Kafka.Brokers) == 0 {
				errs = append(errs, ValidationError{
					"kafka.brokers",
					`required when dispatcher.publisher is "kafka"`,
				})
			}
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// ValidateOperational performs live connectivity checks. It opens and pings
// the configured Postgres instance. Both dev and production profiles fail on
// an unreachable database — there is no degraded mode.
//
// A no-op (nil error) is returned when store.backend is "memory".
//
// This function is intended for the `midas config validate` CLI command and
// for startup validation in main.go. The open+ping+close adds a small but
// acceptable overhead at startup.
func ValidateOperational(ctx context.Context, cfg Config) error {
	if cfg.Store.Backend != "postgres" {
		return nil
	}

	db, err := sql.Open("postgres", cfg.Store.DSN)
	if err != nil {
		return fmt.Errorf("config: open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("config: database unreachable (%s): %w", maskDSN(cfg.Store.DSN), err)
	}

	return nil
}

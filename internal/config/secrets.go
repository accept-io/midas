package config

import (
	"fmt"
	"os"
	"regexp"
)

// placeholderRe matches ${VAR_NAME} placeholders in string values.
var placeholderRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// expandWithEnv replaces every ${VAR} occurrence in data using getenv. An unset
// or empty variable is a hard error — a partially-resolved value is almost
// always a configuration mistake.
func expandWithEnv(data []byte, getenv func(string) string) ([]byte, error) {
	var firstErr error
	expanded := placeholderRe.ReplaceAllFunc(data, func(match []byte) []byte {
		if firstErr != nil {
			return match
		}
		name := string(match[2 : len(match)-1]) // strip ${ and }
		val := getenv(name)
		if val == "" {
			firstErr = fmt.Errorf("unresolved placeholder ${%s}: environment variable is unset or empty", name)
			return match
		}
		return []byte(val)
	})
	if firstErr != nil {
		return nil, firstErr
	}
	return expanded, nil
}

// ExpandPlaceholders expands ${VAR} in data using os.Getenv.
// Exported for use by the `midas config validate` CLI command.
func ExpandPlaceholders(data []byte) ([]byte, error) {
	return expandWithEnv(data, os.Getenv)
}

// expandConfigPlaceholders expands ${VAR} placeholders in the Config struct
// fields that support secrets injection. It is called AFTER env overlay so
// that any field overridden via a MIDAS_* env var never triggers an error for
// an unresolved placeholder in the original file value.
//
// Fields expanded:
//   - store.dsn
//   - auth.tokens[i].token
func expandConfigPlaceholders(cfg *Config, getenv func(string) string) error {
	// store.dsn
	if v, err := expandField(cfg.Store.DSN, "store.dsn", getenv); err != nil {
		return err
	} else {
		cfg.Store.DSN = v
	}

	// auth.tokens[i].token
	for i := range cfg.Auth.Tokens {
		v, err := expandField(cfg.Auth.Tokens[i].Token, fmt.Sprintf("auth.tokens[%d].token", i), getenv)
		if err != nil {
			return err
		}
		cfg.Auth.Tokens[i].Token = v
	}

	return nil
}

// expandField expands a single string value and wraps any error with the field
// path to help operators identify which config field caused the failure.
// Example error: "placeholder expansion failed: store.dsn: unresolved placeholder ${DATABASE_URL}: ..."
func expandField(value, fieldPath string, getenv func(string) string) (string, error) {
	result, err := expandWithEnv([]byte(value), getenv)
	if err != nil {
		return "", fmt.Errorf("placeholder expansion failed: %s: %w", fieldPath, err)
	}
	return string(result), nil
}

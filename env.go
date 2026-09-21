package sys1

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Environment variables read by New. They are the same names
// TypeSafe's official Python and JavaScript SDKs use, so one shell
// configuration works across all of them. A blank or whitespace-only
// value is treated as unset.
const (
	// EnvAPIKey names the environment variable holding the API key.
	EnvAPIKey = "TYPESAFE_API_KEY"
	// EnvBaseURL names the environment variable holding the base URL.
	EnvBaseURL = "TYPESAFE_BASE_URL"
	// EnvModel names the environment variable holding the default
	// model.
	EnvModel = "TYPESAFE_DEFAULT_MODEL"
	// EnvLogLevel names the environment variable selecting the log
	// level: debug, info, warning (or warn), error, or off.
	EnvLogLevel = "TYPESAFE_LOG_LEVEL"
)

// optionsFromEnv translates the TYPESAFE_* environment variables into
// Options, so New has a single code path for validation and precedence.
func optionsFromEnv() ([]Option, error) {
	var opts []Option

	if v := envValue(EnvAPIKey); v != "" {
		opts = append(opts, WithAPIKey(v))
	}
	if v := envValue(EnvBaseURL); v != "" {
		opts = append(opts, WithBaseURL(v))
	}
	if v := envValue(EnvModel); v != "" {
		opts = append(opts, withModel(v))
	}
	if v := envValue(EnvLogLevel); v != "" {
		level, enabled, err := parseLogLevel(v)
		if err != nil {
			return nil, err
		}
		if enabled {
			handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
			opts = append(opts, WithLogger(slog.New(handler)))
		}
	}

	return opts, nil
}

// envValue returns the trimmed value of name, or "" when it is unset
// or blank.
func envValue(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

// parseLogLevel maps a TYPESAFE_LOG_LEVEL value onto a slog level. The
// second result is false for "off", which disables logging entirely
// rather than selecting a level. Matching is case-insensitive and
// accepts both "warning" (the Python SDK's spelling) and "warn".
func parseLogLevel(v string) (slog.Level, bool, error) {
	switch strings.ToLower(v) {
	case "debug":
		return slog.LevelDebug, true, nil
	case "info":
		return slog.LevelInfo, true, nil
	case "warning", "warn":
		return slog.LevelWarn, true, nil
	case "error":
		return slog.LevelError, true, nil
	case "off":
		return 0, false, nil
	default:
		return 0, false, fmt.Errorf("sys1: %s: unknown log level %q (want debug, info, warning, warn, error or off)", EnvLogLevel, v)
	}
}

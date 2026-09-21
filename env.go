package sys1

import (
	"fmt"
	"log/slog"
	"net/http"
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

// New builds a Client from the TYPESAFE_* environment variables and
// the given options. Precedence is explicit option, then environment
// variable, then package default (DefaultBaseURL, DefaultModel,
// DefaultTimeout and DefaultRetryPolicy), so a shell configured for
// TypeSafe's official SDKs works here unchanged and any option still
// wins.
//
// The environment variables read are:
//   - TYPESAFE_API_KEY: the API key (required unless WithAPIKey is given)
//   - TYPESAFE_BASE_URL: the API base URL, default DefaultBaseURL
//   - TYPESAFE_DEFAULT_MODEL: the default model, default DefaultModel
//   - TYPESAFE_LOG_LEVEL: debug, info, warning, error or off; when set
//     to anything but off, requests are logged to stderr at that level
//     and above (see WithLogger). Unset or off disables logging.
//
// New makes no network call. It returns ErrMissingAPIKey if no API key
// is available, or an error if the base URL, log level or retry policy
// is invalid.
func New(opts ...Option) (*Client, error) {
	envOpts, err := optionsFromEnv()
	if err != nil {
		return nil, err
	}

	baseURL, err := parseBaseURL(DefaultBaseURL)
	if err != nil {
		return nil, err
	}
	c := &Client{
		baseURL:    baseURL,
		model:      DefaultModel,
		httpClient: &http.Client{},
		timeout:    DefaultTimeout,
		retry:      DefaultRetryPolicy(),
		headers:    make(http.Header),
		sleep:      defaultSleepFunc,
	}

	// Environment options go first so that explicit options, applied
	// afterwards, override them.
	for _, opt := range append(envOpts, opts...) {
		if opt == nil {
			continue
		}
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	if strings.TrimSpace(c.apiKey) == "" {
		return nil, ErrMissingAPIKey
	}
	return c, nil
}

// optionsFromEnv translates the TYPESAFE_* environment variables into
// Options. Expressing the environment as options keeps a single code
// path for validation and precedence inside New.
func optionsFromEnv() ([]Option, error) {
	var opts []Option

	if v := envValue(EnvAPIKey); v != "" {
		opts = append(opts, WithAPIKey(v))
	}
	if v := envValue(EnvBaseURL); v != "" {
		opts = append(opts, WithBaseURL(v))
	}
	if v := envValue(EnvModel); v != "" {
		opts = append(opts, WithModel(v))
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
		return 0, false, fmt.Errorf("sys1: %s: unknown log level %q (want debug, info, warning, error or off)", EnvLogLevel, v)
	}
}

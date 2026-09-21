package sys1

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Defaults applied when neither an Option nor the matching environment
// variable overrides them.
const (
	// DefaultBaseURL is the production TypeSafe API endpoint.
	DefaultBaseURL = "https://api.typesafe.ai"
	// DefaultModel is TypeSafe's flagship model alias.
	DefaultModel = "jev-latest"
	// DefaultTimeout is the per-attempt request timeout. It bounds a
	// single HTTP round trip, not the overall call; use the caller's
	// context for a total time budget across retries.
	DefaultTimeout = 10 * time.Second
)

// Option configures a Client during New.
type Option func(*Client) error

// WithAPIKey sets the API key sent as a Bearer token, overriding
// TYPESAFE_API_KEY. New fails with ErrMissingAPIKey if neither is set.
func WithAPIKey(key string) Option {
	return func(c *Client) error {
		c.apiKey = key
		return nil
	}
}

// WithBaseURL sets the API base URL, overriding TYPESAFE_BASE_URL and
// DefaultBaseURL. Trailing slashes are trimmed.
func WithBaseURL(raw string) Option {
	return func(c *Client) error {
		u, err := parseBaseURL(raw)
		if err != nil {
			return err
		}
		c.baseURL = u
		return nil
	}
}

// parseBaseURL trims trailing slashes and validates that raw is an
// absolute URL.
func parseBaseURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return nil, fmt.Errorf("sys1: base URL must not be empty")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("sys1: invalid base URL %q: %w", raw, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("sys1: invalid base URL %q: must be absolute", raw)
	}
	return u, nil
}

// withModel sets the default model during construction. It exists
// alongside Client.WithModel so optionsFromEnv can apply
// TYPESAFE_DEFAULT_MODEL with the same precedence as any other
// environment-derived Option, ahead of the caller's explicit opts.
func withModel(name string) Option {
	return func(c *Client) error {
		c.model = name
		return nil
	}
}

// WithHTTPClient replaces the *http.Client used for requests. The
// default is an *http.Client owned by this package, never
// http.DefaultClient, so sys1 never mutates process-wide state.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) error {
		if hc == nil {
			return errors.New("sys1: WithHTTPClient: client must not be nil")
		}
		c.httpClient = hc
		return nil
	}
}

// WithUserAgent appends ua to the User-Agent header in parentheses,
// giving "sys1-go/<Version> (ua)".
func WithUserAgent(ua string) Option {
	return func(c *Client) error {
		c.userAgentSuffix = ua
		return nil
	}
}

// WithLogger sets the logger for request logging. Every attempt is
// logged at debug level (method, path, status, attempt, duration and
// request id) and every scheduled retry at info level. Headers and
// bodies are never logged. The default, nil, disables logging;
// TYPESAFE_LOG_LEVEL sets a stderr logger without code.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) error {
		c.logger = l
		return nil
	}
}

package sys1

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

// Option configures a Client during New. Option is a function
// type rather than an interface so callers cannot forge their own
// options that reach into Client's unexported fields.
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

// WithModel sets the default model used by Evaluate and the shortcut
// methods when no per-call WithRequestModel is given, overriding
// TYPESAFE_DEFAULT_MODEL and DefaultModel.
func WithModel(name string) Option {
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

// WithTimeout sets the per-attempt request timeout, overriding
// DefaultTimeout. It bounds a single HTTP round trip; a retried
// request may take a multiple of this. 0 disables the per-attempt
// timeout, leaving only the caller's context as a deadline.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) error {
		if d < 0 {
			return errors.New("sys1: WithTimeout: duration must be >= 0")
		}
		c.timeout = d
		return nil
	}
}

// WithRetry sets the client's retry policy, overriding
// DefaultRetryPolicy. Pass RetryPolicy{} to disable retries entirely.
func WithRetry(p RetryPolicy) Option {
	return func(c *Client) error {
		if err := p.validate(); err != nil {
			return err
		}
		c.retry = p
		return nil
	}
}

// WithHeader adds a header sent with every request, in addition to the
// headers sys1 sets itself. Calling it again with the same key
// replaces the previous value.
func WithHeader(key, value string) Option {
	return func(c *Client) error {
		if strings.TrimSpace(key) == "" {
			return errors.New("sys1: WithHeader: key must not be empty")
		}
		c.headers.Set(key, value)
		return nil
	}
}

// WithUserAgent appends a suffix to the User-Agent header, after
// "sys1-go/<Version>".
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

// requestConfig holds the per-call overrides collected from
// RequestOptions.
type requestConfig struct {
	model     string
	headers   http.Header
	retry     *RetryPolicy
	extraBody map[string]any
}

// newRequestConfig applies opts to a fresh requestConfig.
func newRequestConfig(opts []RequestOption) *requestConfig {
	rc := &requestConfig{}
	for _, opt := range opts {
		rc.apply(opt)
	}
	return rc
}

// apply runs opt against rc, tolerating a nil option.
func (rc *requestConfig) apply(opt RequestOption) {
	if opt != nil {
		opt(rc)
	}
}

// effectiveRetry returns the per-call retry policy when one was given,
// validated and wrapped with ErrInvalidRequest if it is malformed, and
// def otherwise.
func (rc *requestConfig) effectiveRetry(def RetryPolicy) (RetryPolicy, error) {
	if rc.retry == nil {
		return def, nil
	}
	if err := rc.retry.validate(); err != nil {
		return RetryPolicy{}, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	return *rc.retry, nil
}

// RequestOption configures a single Evaluate or Models call, layered
// on top of the Client's defaults. It satisfies EvaluateArg, so it
// can sit anywhere in Evaluate's variadic list next to the questions.
type RequestOption func(*requestConfig)

func (RequestOption) evaluateArg() {}

// WithRequestModel overrides the client's default model for a single
// call.
func WithRequestModel(name string) RequestOption {
	return func(rc *requestConfig) {
		rc.model = name
	}
}

// WithRequestHeader adds a header sent with a single call, overriding
// any client-level header set via WithHeader with the same key.
func WithRequestHeader(key, value string) RequestOption {
	return func(rc *requestConfig) {
		if rc.headers == nil {
			rc.headers = make(http.Header)
		}
		rc.headers.Set(key, value)
	}
}

// WithRequestRetry overrides the client's retry policy for a single
// call.
func WithRequestRetry(p RetryPolicy) RequestOption {
	return func(rc *requestConfig) {
		rc.retry = &p
	}
}

// WithRequestExtraBody adds extra top-level fields to the request
// body, for API fields this library predates. Fields are shallow
// merged over state, model and questions: an extra field with one of
// those names replaces it.
func WithRequestExtraBody(fields map[string]any) RequestOption {
	return func(rc *requestConfig) {
		rc.extraBody = fields
	}
}

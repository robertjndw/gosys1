package sys1

import (
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a TypeSafe SystemOne API client. Construct one with New.
// A Client and every client derived from it are safe for concurrent
// use.
type Client struct {
	apiKey          string
	baseURL         *url.URL
	model           string
	httpClient      *http.Client
	timeout         time.Duration
	retry           RetryPolicy
	headers         http.Header
	extraBody       map[string]any
	userAgentSuffix string
	logger          *slog.Logger

	// sleep backs the retry backoff. Tests swap in an instant version
	// so retry tests don't actually have to wait.
	sleep sleepFunc
}

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
//   - TYPESAFE_LOG_LEVEL: debug, info, warning (or warn), error or off;
//     when set to anything but off, requests are logged to stderr at
//     that level and above (see WithLogger). Unset or off disables
//     logging.
//
// New makes no network call. It returns ErrMissingAPIKey if no API key
// is available, or an error if the base URL, the log level or an
// option's argument is invalid.
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

// with returns a copy of c with mutate applied, leaving c itself
// unchanged. The header map is always cloned first, so mutate (or a
// later call on the copy) can never write through to c's headers, even
// when it has nothing to do with headers.
func (c *Client) with(mutate func(*Client)) *Client {
	cp := *c
	cp.headers = c.headers.Clone()
	mutate(&cp)
	return &cp
}

// WithModel returns a copy of c that uses name as the default model
// for Ask and the shortcut methods. c itself is unchanged.
func (c *Client) WithModel(name string) *Client {
	return c.with(func(cp *Client) { cp.model = name })
}

// WithTimeout returns a copy of c with d as the per-attempt request
// timeout. d <= 0 disables it, leaving the caller's context as the
// only deadline. c itself is unchanged.
func (c *Client) WithTimeout(d time.Duration) *Client {
	return c.with(func(cp *Client) { cp.timeout = d })
}

// WithRetry returns a copy of c that uses p as its retry policy. p is
// not validated here; a bad policy surfaces as an error wrapping
// ErrInvalidRequest on the derived client's next call. c itself is
// unchanged.
func (c *Client) WithRetry(p RetryPolicy) *Client {
	return c.with(func(cp *Client) { cp.retry = p })
}

// WithHeader returns a copy of c that sends key: value with every
// request, on top of the headers sys1 sets itself. Setting the same
// key again replaces the value on the derived client only. Nothing is
// validated here; a bad header name fails at the transport. c itself
// is unchanged.
func (c *Client) WithHeader(key, value string) *Client {
	return c.with(func(cp *Client) { cp.headers.Set(key, value) })
}

// WithExtraBody returns a copy of c that merges fields into the top
// level of every request body, for API fields this library predates.
// state, model and questions are reserved; Ask rejects a
// collision with an error wrapping ErrInvalidRequest rather than
// letting it shadow the built-in value. c itself is unchanged.
func (c *Client) WithExtraBody(fields map[string]any) *Client {
	return c.with(func(cp *Client) { cp.extraBody = maps.Clone(fields) })
}

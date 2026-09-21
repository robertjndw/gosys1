package sys1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// maxResponseBodyBytes bounds how much of a response body is read into
// memory, guarding against a misbehaving or malicious server.
const maxResponseBodyBytes = 16 * 1024 * 1024

// requestIDHeader is the response header carrying TypeSafe's request
// id, surfaced on Response.RequestID and APIError.RequestID.
const requestIDHeader = "x-typesafe-request-id"

// do performs an HTTP request with retries per the client's policy and
// decodes a 2xx response body into out. body is nil for requests
// without one (GET). It returns the response headers, or an error: an
// error wrapping ErrInvalidRequest if the client's retry policy is
// malformed, the caller's ctx error if the caller canceled,
// ErrInvalidResponse if a 2xx body does not decode, or the *APIError
// or transport error from the final attempt. Only that last kind is
// wrapped with ErrRetriesExhausted, and only when at least one retry
// was attempted.
func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) (http.Header, error) {
	retry := c.retry
	if err := retry.validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	reqURL := c.baseURL.JoinPath(path)
	reqURLStr := reqURL.String()
	attempts := retry.MaxRetries + 1

	var last attemptResult
	attempt := 0
	for ; attempt < attempts; attempt++ {
		start := time.Now()
		last = c.doAttempt(ctx, method, reqURLStr, body)
		c.logAttempt(method, path, last.status, attempt, time.Since(start), last.header.Get(requestIDHeader))

		var retryable bool
		var reason string
		switch {
		case last.err != nil:
			// A canceled or expired caller context is returned as-is
			// and never retried; a per-attempt timeout is a different
			// context and falls through to the retry logic below.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			retryable, reason = retry.RetryConnErrors, last.err.Error()
		case last.status >= 200 && last.status < 300:
			if err := json.Unmarshal(last.body, out); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrInvalidResponse, err)
			}
			return last.header, nil
		default:
			retryable, reason = retry.shouldRetryStatus(last.status), "status "+strconv.Itoa(last.status)
		}
		if !retryable || attempt == attempts-1 {
			break
		}
		if err := c.wait(ctx, retry, last.header, attempt, reason); err != nil {
			return nil, err
		}
	}

	// The APIError is built only for the attempt that ends the loop, so
	// a retried failure whose retry succeeds never pays for parsing its
	// body.
	err := last.err
	if err == nil {
		err = newAPIError(method, reqURL, last.status, last.header, last.body)
	}
	if attempt > 0 {
		return nil, fmt.Errorf("%w: %w", ErrRetriesExhausted, err)
	}
	return nil, err
}

// attemptResult is the outcome of a single HTTP attempt inside do's
// retry loop.
type attemptResult struct {
	body   []byte
	header http.Header
	status int
	// err is a transport-level failure: building the request, the
	// round trip itself, or reading the body. It is nil whenever an
	// HTTP response was received and its body read, even for a
	// non-2xx status.
	err error
}

// doAttempt performs one HTTP attempt, applying the per-attempt
// timeout and this client's headers.
func (c *Client) doAttempt(ctx context.Context, method, reqURL string, body []byte) attemptResult {
	attemptCtx := ctx
	if c.timeout > 0 {
		var cancel context.CancelFunc
		attemptCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(attemptCtx, method, reqURL, reader)
	if err != nil {
		return attemptResult{err: fmt.Errorf("sys1: building request: %w", err)}
	}
	c.setHeaders(req, body != nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return attemptResult{err: fmt.Errorf("sys1: request failed: %w", err)}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return attemptResult{header: resp.Header, err: fmt.Errorf("sys1: reading response body: %w", err)}
	}

	return attemptResult{body: data, header: resp.Header, status: resp.StatusCode}
}

// setHeaders applies sys1's own headers, then the client's headers
// (WithHeader), which override sys1's own on a key collision.
func (c *Client) setHeaders(req *http.Request, hasBody bool) {
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", c.userAgent())

	for k, vs := range c.headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
}

// userAgent renders the User-Agent header value.
func (c *Client) userAgent() string {
	if c.userAgentSuffix == "" {
		return userAgent
	}
	return userAgent + " (" + c.userAgentSuffix + ")"
}

// wait sleeps for the retry delay before the given attempt.
func (c *Client) wait(ctx context.Context, retry RetryPolicy, header http.Header, attempt int, reason string) error {
	delay := retryDelay(retry, header, attempt)
	c.logRetry(attempt+1, retry.MaxRetries, delay, reason)
	return c.sleep(ctx, delay)
}

// logRetry logs a scheduled retry at info level when a logger is
// configured. Retries are rarer and more worth noticing than single
// attempts, hence info rather than logAttempt's debug.
func (c *Client) logRetry(retryNum, maxRetries int, delay time.Duration, reason string) {
	if c.logger == nil {
		return
	}
	c.logger.Info("sys1: retrying request",
		"retry", retryNum,
		"max_retries", maxRetries,
		"delay", delay,
		"reason", reason,
	)
}

// logAttempt logs one HTTP attempt at debug level when a logger is
// configured. It never logs headers or bodies.
func (c *Client) logAttempt(method, path string, status, attempt int, duration time.Duration, requestID string) {
	if c.logger == nil {
		return
	}
	c.logger.Debug("sys1: request",
		"method", method,
		"path", path,
		"status", status,
		"attempt", attempt,
		"duration", duration,
		"request_id", requestID,
	)
}

package sys1

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// RetryPolicy controls how the client retries failed requests. The
// zero value disables retries (MaxRetries 0); use DefaultRetryPolicy
// for TypeSafe's recommended defaults.
type RetryPolicy struct {
	// MaxRetries is the number of retry attempts after the first,
	// failed attempt. 0 disables retries.
	MaxRetries int
	// InitialBackoff is the delay before the first retry.
	InitialBackoff time.Duration
	// MaxBackoff caps the computed backoff delay, before jitter is
	// applied.
	MaxBackoff time.Duration
	// Jitter is the fraction of the computed backoff subtracted at
	// random, in [0, 1]. 0.25 means the actual delay is somewhere
	// between 75% and 100% of the computed backoff.
	Jitter float64
	// RetryStatuses decides whether a response status code should be
	// retried. A nil func retries 408, 429 and any 5xx status
	// (including 529).
	RetryStatuses func(code int) bool
	// RespectRetryAfter, when true, uses a response's Retry-After or
	// retry-after-ms header as the retry delay instead of the computed
	// backoff, subject to MaxRetryAfter.
	RespectRetryAfter bool
	// MaxRetryAfter caps how long a server-provided Retry-After hint is
	// honored; longer hints fall back to the computed backoff.
	MaxRetryAfter time.Duration
	// RetryConnErrors, when true, retries transport-level failures
	// (connection errors, per-attempt timeouts) in addition to
	// retryable status codes.
	RetryConnErrors bool
}

// DefaultRetryPolicy returns TypeSafe's recommended retry policy: two
// retries, 500ms initial backoff doubling up to 5s, 25% jitter,
// retrying 408/429/5xx and connection errors, honoring Retry-After up
// to 60s.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:        2,
		InitialBackoff:    500 * time.Millisecond,
		MaxBackoff:        5 * time.Second,
		Jitter:            0.25,
		RetryStatuses:     nil,
		RespectRetryAfter: true,
		MaxRetryAfter:     60 * time.Second,
		RetryConnErrors:   true,
	}
}

// validate reports whether the policy's numeric fields are sane. It is
// checked by New and WithRequestRetry so a misconfigured policy
// fails fast instead of misbehaving at request time.
func (p RetryPolicy) validate() error {
	switch {
	case p.MaxRetries < 0:
		return errors.New("sys1: retry policy: MaxRetries must be >= 0")
	case p.InitialBackoff < 0:
		return errors.New("sys1: retry policy: InitialBackoff must be >= 0")
	case p.MaxBackoff < 0:
		return errors.New("sys1: retry policy: MaxBackoff must be >= 0")
	case p.Jitter < 0 || p.Jitter > 1:
		return errors.New("sys1: retry policy: Jitter must be in [0, 1]")
	case p.MaxRetryAfter < 0:
		return errors.New("sys1: retry policy: MaxRetryAfter must be >= 0")
	}
	return nil
}

// shouldRetryStatus reports whether a response with the given status
// code should be retried under this policy.
func (p RetryPolicy) shouldRetryStatus(code int) bool {
	if p.RetryStatuses != nil {
		return p.RetryStatuses(code)
	}
	return defaultRetryStatus(code)
}

// defaultRetryStatus is the fallback used when RetryPolicy.RetryStatuses
// is nil: 408, 429 and any 5xx status (529 included).
func defaultRetryStatus(code int) bool {
	if code == http.StatusRequestTimeout || code == http.StatusTooManyRequests {
		return true
	}
	return code >= 500 && code < 600
}

// backoff computes the delay before the given retry attempt (0-based:
// attempt 0 is the delay before the first retry), applying the
// configured cap and jitter. Jitter comes from math/rand's package
// source, which is auto-seeded and safe for concurrent callers.
func (p RetryPolicy) backoff(attempt int) time.Duration {
	d := float64(p.InitialBackoff) * math.Pow(2, float64(attempt))
	if p.MaxBackoff > 0 {
		d = min(d, float64(p.MaxBackoff))
	}
	if p.Jitter > 0 {
		d -= d * p.Jitter * rand.Float64()
	}
	return time.Duration(d)
}

// retryAfter parses a Retry-After or retry-after-ms response header
// into a delay. retry-after-ms takes priority when both are present.
// The bool result is false when neither header is present or parsable.
func retryAfter(h http.Header) (time.Duration, bool) {
	if v := h.Get("retry-after-ms"); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms >= 0 {
			return time.Duration(ms) * time.Millisecond, true
		}
	}
	if v := h.Get("Retry-After"); v != "" {
		if secs, err := strconv.ParseInt(v, 10, 64); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second, true
		}
		if t, err := http.ParseTime(v); err == nil {
			return max(time.Until(t), 0), true
		}
	}
	return 0, false
}

// retryDelay picks the delay before the given retry attempt: a
// server-provided Retry-After hint when the policy respects it and the
// hint is within MaxRetryAfter, otherwise the computed backoff.
func retryDelay(p RetryPolicy, h http.Header, attempt int) time.Duration {
	if p.RespectRetryAfter {
		if d, ok := retryAfter(h); ok && d <= p.MaxRetryAfter {
			return d
		}
	}
	return p.backoff(attempt)
}

// sleepFunc pauses for d, returning early with ctx's error if ctx is
// done first. It is a field on Client so tests can inject an instant,
// deterministic implementation.
type sleepFunc func(ctx context.Context, d time.Duration) error

// defaultSleepFunc is the production sleepFunc: a real timer that
// respects context cancellation.
func defaultSleepFunc(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

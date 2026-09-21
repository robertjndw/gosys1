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

// RetryPolicy controls how the client retries failed requests. Only
// MaxRetries needs setting: a zero duration takes the default from
// DefaultRetryPolicy, and the boolean fields opt out of behavior that
// is on by default, so RetryPolicy{MaxRetries: 5} is the default
// policy with more retries. The zero value disables retries.
type RetryPolicy struct {
	// MaxRetries is the number of retry attempts after the first,
	// failed attempt. 0 disables retries.
	MaxRetries int
	// InitialBackoff is the delay before the first retry. 0 means
	// 500ms.
	InitialBackoff time.Duration
	// MaxBackoff caps the computed backoff delay, before jitter is
	// applied. 0 means 5s.
	MaxBackoff time.Duration
	// Jitter is the fraction of the computed backoff subtracted at
	// random, in [0, 1]. 0.25 means the actual delay is somewhere
	// between 75% and 100% of the computed backoff. Unlike the
	// durations, 0 is taken literally and means no jitter.
	Jitter float64
	// RetryStatuses decides whether a response status code should be
	// retried. A nil func retries 408, 429 and any 5xx status
	// (including 529).
	RetryStatuses func(code int) bool
	// IgnoreRetryAfter, when true, always uses the computed backoff as
	// the retry delay. By default a response's Retry-After or
	// retry-after-ms header takes precedence, subject to MaxRetryAfter.
	IgnoreRetryAfter bool
	// MaxRetryAfter caps how long a server-provided Retry-After hint is
	// honored; longer hints fall back to the computed backoff. 0 means
	// 60s.
	MaxRetryAfter time.Duration
	// DisableConnRetries, when true, retries only retryable status
	// codes. By default transport-level failures (connection errors,
	// per-attempt timeouts) are retried as well.
	DisableConnRetries bool
}

// What a zero duration in a RetryPolicy resolves to.
const (
	defaultInitialBackoff = 500 * time.Millisecond
	defaultMaxBackoff     = 5 * time.Second
	defaultMaxRetryAfter  = 60 * time.Second
)

// DefaultRetryPolicy returns TypeSafe's recommended retry policy: two
// retries, 500ms initial backoff doubling up to 5s, 25% jitter,
// retrying 408/429/5xx and connection errors, honoring Retry-After up
// to 60s. The durations are set explicitly even though resolved would
// fill them in, so the value prints as real numbers.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:     2,
		InitialBackoff: defaultInitialBackoff,
		MaxBackoff:     defaultMaxBackoff,
		Jitter:         0.25,
		MaxRetryAfter:  defaultMaxRetryAfter,
	}
}

// resolved returns p with every zero duration replaced by its default.
// do runs it once per request; backoff and retryDelay assume it has.
func (p RetryPolicy) resolved() RetryPolicy {
	if p.InitialBackoff == 0 {
		p.InitialBackoff = defaultInitialBackoff
	}
	if p.MaxBackoff == 0 {
		p.MaxBackoff = defaultMaxBackoff
	}
	if p.MaxRetryAfter == 0 {
		p.MaxRetryAfter = defaultMaxRetryAfter
	}
	return p
}

// validate checks the policy's numeric fields. It runs at the start of
// every request rather than in Client.WithRetry so WithRetry can't
// fail and stays chainable, while a bad policy still errors before
// anything hits the network.
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
// configured cap and jitter. It expects a resolved policy. Jitter
// comes from math/rand's package source, which is auto-seeded and safe
// for concurrent callers.
func (p RetryPolicy) backoff(attempt int) time.Duration {
	d := min(float64(p.InitialBackoff)*math.Pow(2, float64(attempt)), float64(p.MaxBackoff))
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
// server-provided Retry-After hint when the policy honors it and the
// hint is within MaxRetryAfter, otherwise the computed backoff. It
// expects a resolved policy.
func retryDelay(p RetryPolicy, h http.Header, attempt int) time.Duration {
	if !p.IgnoreRetryAfter {
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

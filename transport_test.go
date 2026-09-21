package sys1

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetry429WithRetryAfterThenSucceeds(t *testing.T) {
	var attempts atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			writeJSON(w, http.StatusTooManyRequests, `{"message": "slow down"}`, map[string]string{"Retry-After": "0"})
			return
		}
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	})
	resp, err := c.Evaluate(context.Background(), "state", Noul("q", "q"))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if resp.Model != "jev-1.13.0" {
		t.Errorf("resp.Model = %q, want jev-1.13.0", resp.Model)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("server received %d requests, want 2", got)
	}
}

func TestRetry529(t *testing.T) {
	var attempts atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 2 {
			writeJSON(w, 529, `{"message": "overloaded"}`, nil)
			return
		}
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	})
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "q"))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("server received %d requests, want 2", got)
	}
}

func TestUnauthorizedNotRetried(t *testing.T) {
	var attempts atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		writeJSON(w, http.StatusUnauthorized, `{"message": "invalid API key"}`, nil)
	})
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "q"))
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("server received %d requests, want 1 (401 must not be retried)", got)
	}
	if errors.Is(err, ErrRetriesExhausted) {
		t.Error("error wraps ErrRetriesExhausted despite no retry happening")
	}
}

func TestCtxCancellationStopsRetriesImmediately(t *testing.T) {
	var attempts atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		writeJSON(w, http.StatusInternalServerError, `{"message": "boom"}`, nil)
	}).WithRetry(RetryPolicy{
		MaxRetries:      3,
		InitialBackoff:  time.Millisecond,
		MaxBackoff:      time.Millisecond,
		RetryConnErrors: true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	// Simulate the caller canceling ctx while the client is waiting to
	// retry: the next sleep call cancels ctx itself and reports it.
	c.sleep = func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, err := c.Evaluate(ctx, "state", Noul("q", "q"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("server received %d requests, want 1 (no retry after cancellation)", got)
	}
}

func TestCtxAlreadyCanceled(t *testing.T) {
	var attempts atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Evaluate(ctx, "state", Noul("q", "q"))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if got := attempts.Load(); got != 0 {
		t.Errorf("server received %d requests, want 0", got)
	}
}

func TestPerAttemptTimeoutRetried(t *testing.T) {
	var attempts atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			time.Sleep(100 * time.Millisecond)
		}
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	}).
		WithTimeout(10 * time.Millisecond).
		WithRetry(RetryPolicy{MaxRetries: 1, InitialBackoff: time.Millisecond, RetryConnErrors: true})
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "q"))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("server received %d requests, want 2", got)
	}
}

func TestMaxRetriesZeroGivesOneAttempt(t *testing.T) {
	var attempts atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		writeJSON(w, http.StatusInternalServerError, `{"message": "boom"}`, nil)
	}).WithRetry(RetryPolicy{})
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "q"))
	if err == nil {
		t.Fatal("Evaluate() error = nil, want error")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("server received %d requests, want 1", got)
	}
	if errors.Is(err, ErrRetriesExhausted) {
		t.Error("error wraps ErrRetriesExhausted despite MaxRetries being 0")
	}
}

func TestRetriesExhaustedWraps(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, `{"message": "boom"}`, nil)
	}).WithRetry(RetryPolicy{MaxRetries: 2, InitialBackoff: time.Millisecond, RetryConnErrors: true})
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "q"))
	if !errors.Is(err, ErrRetriesExhausted) {
		t.Errorf("error = %v, want ErrRetriesExhausted", err)
	}
	if !errors.Is(err, ErrServer) {
		t.Errorf("error = %v, want it to still be ErrServer", err)
	}
}

func TestBodyResentIdenticallyOnRetry(t *testing.T) {
	var attempts atomic.Int32
	var bodies [][]byte
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, b)
		n := attempts.Add(1)
		if n < 3 {
			writeJSON(w, http.StatusInternalServerError, `{"message": "boom"}`, nil)
			return
		}
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	}).WithRetry(RetryPolicy{MaxRetries: 2, InitialBackoff: time.Millisecond, RetryConnErrors: true})
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "q"))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(bodies) != 3 {
		t.Fatalf("len(bodies) = %d, want 3", len(bodies))
	}
	for i := 1; i < len(bodies); i++ {
		if !reflect.DeepEqual(bodies[i], bodies[0]) {
			t.Errorf("body on attempt %d differs from attempt 0:\n%s\nvs\n%s", i, bodies[i], bodies[0])
		}
	}
}

func TestInvalidResponseBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, `not json`, nil)
	})
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "q"))
	if !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("error = %v, want ErrInvalidResponse", err)
	}
}

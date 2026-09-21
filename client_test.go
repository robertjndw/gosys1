package sys1

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewInvalidBaseURL(t *testing.T) {
	_, err := New(WithAPIKey("key"), WithBaseURL("not a url"))
	if err == nil {
		t.Fatal("New() error = nil, want error")
	}
}

func TestWithRetryInvalidPolicySurfacesFromEvaluateAndModels(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	}).WithRetry(RetryPolicy{MaxRetries: -1})

	if _, err := c.Evaluate(context.Background(), "state", Noul("q", "q")); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Evaluate() error = %v, want ErrInvalidRequest", err)
	}
	if _, err := c.Models(context.Background()); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Models() error = %v, want ErrInvalidRequest", err)
	}
	if called {
		t.Error("server was called despite an invalid retry policy")
	}
}

func TestDerivationMethodsReturnDistinctClientsLeavingParentUnchanged(t *testing.T) {
	c, err := New(WithAPIKey("key"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	derived := map[string]*Client{
		"WithModel":     c.WithModel("other-model"),
		"WithTimeout":   c.WithTimeout(time.Second),
		"WithRetry":     c.WithRetry(RetryPolicy{MaxRetries: 5}),
		"WithHeader":    c.WithHeader("X-Test", "v"),
		"WithExtraBody": c.WithExtraBody(map[string]any{"k": "v"}),
	}
	for name, d := range derived {
		if d == c {
			t.Errorf("%s returned the receiver, want a distinct *Client", name)
		}
	}

	if c.model != DefaultModel {
		t.Errorf("parent model = %q, want unchanged %q", c.model, DefaultModel)
	}
	if c.timeout != DefaultTimeout {
		t.Errorf("parent timeout = %v, want unchanged %v", c.timeout, DefaultTimeout)
	}
	if !reflect.DeepEqual(c.retry, DefaultRetryPolicy()) {
		t.Errorf("parent retry = %+v, want unchanged default", c.retry)
	}
	if len(c.headers) != 0 {
		t.Errorf("parent headers = %v, want empty", c.headers)
	}
	if c.extraBody != nil {
		t.Errorf("parent extraBody = %v, want nil", c.extraBody)
	}
}

func TestWithHeaderChildHeaderNeverReachesParentRequests(t *testing.T) {
	var gotHeader string
	base := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	})
	child := base.WithHeader("X-Custom", "child-value")

	if _, err := base.Evaluate(context.Background(), "state", Noul("q", "q")); err != nil {
		t.Fatalf("Evaluate (parent): %v", err)
	}
	if gotHeader != "" {
		t.Errorf("parent request sent X-Custom = %q, want empty", gotHeader)
	}

	if _, err := child.Evaluate(context.Background(), "state", Noul("q", "q")); err != nil {
		t.Fatalf("Evaluate (child): %v", err)
	}
	if gotHeader != "child-value" {
		t.Errorf("child request sent X-Custom = %q, want child-value", gotHeader)
	}
}

func TestDerivationNeverAliasesParentHeaders(t *testing.T) {
	base, err := New(WithAPIKey("key"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// child is derived through a method that never touches headers.
	// If the header map were shared rather than cloned on every
	// derivation, a header added to base afterward would leak into
	// child even though child predates it.
	child := base.WithModel("child-model")
	_ = base.WithHeader("X-Custom", "parent-value")

	if _, ok := child.headers["X-Custom"]; ok {
		t.Error("a header added to the parent after deriving a child leaked into the child")
	}
	if _, ok := base.headers["X-Custom"]; ok {
		t.Error("WithHeader mutated the receiver's own headers")
	}
}

func TestWithTimeoutZeroAndNegativeDisablePerAttemptTimeout(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		t.Run(d.String(), func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(30 * time.Millisecond)
				writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
			}).WithTimeout(5 * time.Millisecond).WithTimeout(d)
			if _, err := c.Evaluate(context.Background(), "state", Noul("q", "q")); err != nil {
				t.Fatalf("Evaluate: %v, want nil (per-attempt timeout should be disabled)", err)
			}
		})
	}
}

func TestWithUserAgentAppendsSuffix(t *testing.T) {
	var gotUA string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	}, WithUserAgent("my-app/1.0"))
	if _, err := c.Evaluate(context.Background(), "state", Noul("q", "q")); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := fmt.Sprintf("sys1-go/%s (my-app/1.0)", Version)
	if gotUA != want {
		t.Errorf("User-Agent = %q, want %q", gotUA, want)
	}
}

func TestWithLoggerDoesNotLeakSecrets(t *testing.T) {
	var buf strings.Builder
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	}, WithAPIKey("super-secret"), WithLogger(slogTestLogger(&buf)))

	if _, err := c.Evaluate(context.Background(), "state", Noul("q", "q")); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "super-secret") {
		t.Errorf("log output contains the API key: %s", out)
	}
	if !strings.Contains(out, "/v1/systemone") {
		t.Errorf("log output missing request path: %s", out)
	}
}

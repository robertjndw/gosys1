package sys1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// slogTestLogger builds a debug-level text logger writing to w, for
// asserting on sys1's own log lines in tests.
func slogTestLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// newTestClient starts a test server around handler, closed when the
// test ends, and builds a Client pointed at it with an instant sleep
// function so retry tests run fast and without flakiness.
func newTestClient(t *testing.T, handler http.HandlerFunc, opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	base := append([]Option{
		WithAPIKey("test-key"),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	}, opts...)
	c, err := New(base...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	return c
}

// decodeBody decodes a request body into a generic map, so tests can
// assert on individual fields of what the client sent.
func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decoding request body: %v", err)
	}
	return body
}

// writeJSON writes a JSON response with the given status and extra
// headers.
func writeJSON(w http.ResponseWriter, status int, body string, headers map[string]string) {
	for k, v := range headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

const noulEvaluateResponse = `{
	"model": "jev-1.13.0",
	"answers": {"q": {"type": "noul", "noul": 0.95}},
	"usage": {"input_tokens": 10, "output_tokens": 2}
}`

const choiceEvaluateResponse = `{
	"model": "jev-1.13.0",
	"answers": {"q": {"type": "choice", "choice": "billing", "probabilities": {"billing": 1}, "confidence": 0.9}},
	"usage": {"input_tokens": 10, "output_tokens": 2}
}`

const scoreEvaluateResponse = `{
	"model": "jev-1.13.0",
	"answers": {"q": {"type": "score", "score": 1, "legend": {"0": "a", "1": "b"}, "probabilities": {"0": 0, "1": 1}, "confidence": 0.9}},
	"usage": {"input_tokens": 10, "output_tokens": 2}
}`

func TestNewInvalidBaseURL(t *testing.T) {
	_, err := New(WithAPIKey("key"), WithBaseURL("not a url"))
	if err == nil {
		t.Fatal("New() error = nil, want error")
	}
}

func TestNewInvalidRetryPolicy(t *testing.T) {
	_, err := New(WithAPIKey("key"), WithRetry(RetryPolicy{MaxRetries: -1}))
	if err == nil {
		t.Fatal("New() error = nil, want error")
	}
}

func TestEvaluateSendsDocumentedBody(t *testing.T) {
	var gotBody map[string]any
	var gotAuth, gotUA, gotMethod, gotPath, gotContentType string

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotContentType = r.Header.Get("Content-Type")
		gotBody = decodeBody(t, r)
		writeJSON(w, http.StatusOK, noulEvaluateResponse, map[string]string{"x-typesafe-request-id": "req-123"})
	}, WithAPIKey("secret-key"))
	resp, err := c.Evaluate(context.Background(), "Help! My payouts have been failing for 3 days.",
		Noul("is_urgent", "Does this convey urgency?"),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/systemone" {
		t.Errorf("path = %q, want /v1/systemone", gotPath)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization = %q, want Bearer secret-key", gotAuth)
	}
	if !strings.HasPrefix(gotUA, "sys1-go/"+Version) {
		t.Errorf("User-Agent = %q, want prefix sys1-go/%s", gotUA, Version)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}

	wantBody := map[string]any{
		"state": "Help! My payouts have been failing for 3 days.",
		"model": DefaultModel,
		"questions": map[string]any{
			"is_urgent": map[string]any{
				"type":         "noul",
				"instructions": "Does this convey urgency?",
			},
		},
	}
	if !reflect.DeepEqual(gotBody, wantBody) {
		t.Errorf("request body mismatch:\n got: %#v\nwant: %#v", gotBody, wantBody)
	}

	if resp.Model != "jev-1.13.0" {
		t.Errorf("resp.Model = %q, want jev-1.13.0", resp.Model)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("resp.RequestID = %q, want req-123", resp.RequestID)
	}
	if resp.Usage != (Usage{InputTokens: 10, OutputTokens: 2}) {
		t.Errorf("resp.Usage = %+v, want {10 2}", resp.Usage)
	}
	noul, err := resp.Answers.Noul("q")
	if err != nil || noul.Noul != 0.95 {
		t.Errorf("resp.Answers.Noul(q) = %v, %v, want 0.95, nil", noul, err)
	}
}

func TestEvaluateMixesQuestionsAndOptions(t *testing.T) {
	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	})
	_, err := c.Evaluate(context.Background(), "state",
		Noul("billing", "About billing?"),
		WithRequestModel("per-call-model"),
		Choice("tone", "Tone?", Choices{"calm": nil, "angry": nil}),
		Score("urgency", "Urgency?", "low", "high"),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if got := gotBody["model"]; got != "per-call-model" {
		t.Errorf("model = %v, want per-call-model", got)
	}
	questions, _ := gotBody["questions"].(map[string]any)
	if len(questions) != 3 {
		t.Fatalf("questions = %#v, want 3 entries", questions)
	}
	for name, wantType := range map[string]string{"billing": "noul", "tone": "choice", "urgency": "score"} {
		q, _ := questions[name].(map[string]any)
		if q["type"] != wantType {
			t.Errorf("questions[%q].type = %v, want %s", name, q["type"], wantType)
		}
	}
}

func TestEvaluateRejectsDuplicateQuestionName(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	})
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "a"), Noul("q", "b"))
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Evaluate() error = %v, want ErrInvalidRequest", err)
	}
	if called {
		t.Error("server was called despite a duplicate question name")
	}
}

func TestEvaluateWithRequestModel(t *testing.T) {
	var gotModel string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotModel, _ = decodeBody(t, r)["model"].(string)
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	}, WithModel("client-default-model"))
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "q"), WithRequestModel("per-call-model"))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if gotModel != "per-call-model" {
		t.Errorf("model in body = %q, want per-call-model", gotModel)
	}
}

func TestEvaluateWithRequestExtraBody(t *testing.T) {
	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	})
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "q"),
		WithRequestExtraBody(map[string]any{
			"beam_width": float64(4),
			"model":      "overridden-model", // collides with the built-in field
		}),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := gotBody["beam_width"]; got != float64(4) {
		t.Errorf("beam_width = %v, want 4", got)
	}
	if got := gotBody["model"]; got != "overridden-model" {
		t.Errorf("model = %v, want overridden-model (extra body should win)", got)
	}
}

func TestEvaluateWithRequestHeader(t *testing.T) {
	var gotHeader string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	}, WithHeader("X-Custom", "client-value"))
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "q"), WithRequestHeader("X-Custom", "call-value"))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if gotHeader != "call-value" {
		t.Errorf("X-Custom = %q, want call-value (request header should win)", gotHeader)
	}
}

func TestEvaluateValidatesLocallyBeforeNetworkCall(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
	})
	_, err := c.Evaluate(context.Background(), "state")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Evaluate() error = %v, want ErrInvalidRequest", err)
	}
	if called {
		t.Error("server was called despite a client-side validation failure")
	}
}

func TestClientShortcuts(t *testing.T) {
	t.Run("Noul", func(t *testing.T) {
		var gotQuestions map[string]any
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotQuestions, _ = decodeBody(t, r)["questions"].(map[string]any)
			writeJSON(w, http.StatusOK, noulEvaluateResponse, nil)
		})
		prob, err := c.Noul(context.Background(), "state", "Is this urgent?")
		if err != nil {
			t.Fatalf("Noul: %v", err)
		}
		if prob != 0.95 {
			t.Errorf("Noul = %v, want 0.95", prob)
		}
		q, ok := gotQuestions["q"].(map[string]any)
		if !ok || q["type"] != "noul" {
			t.Errorf("questions = %#v, want a single noul question keyed %q", gotQuestions, "q")
		}
	})

	t.Run("Choice", func(t *testing.T) {
		var gotQuestions map[string]any
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotQuestions, _ = decodeBody(t, r)["questions"].(map[string]any)
			writeJSON(w, http.StatusOK, choiceEvaluateResponse, nil)
		})
		ans, err := c.Choice(context.Background(), "state", "Which team?", Choices{"billing": nil, "technical": nil})
		if err != nil {
			t.Fatalf("Choice: %v", err)
		}
		if ans.Choice != "billing" {
			t.Errorf("Choice = %q, want billing", ans.Choice)
		}
		q, ok := gotQuestions["q"].(map[string]any)
		if !ok || q["type"] != "choice" {
			t.Errorf("questions = %#v, want a single choice question keyed %q", gotQuestions, "q")
		}
	})

	t.Run("Score", func(t *testing.T) {
		var gotQuestions map[string]any
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotQuestions, _ = decodeBody(t, r)["questions"].(map[string]any)
			writeJSON(w, http.StatusOK, scoreEvaluateResponse, nil)
		})
		ans, err := c.Score(context.Background(), "state", "Rate this", "low", "high")
		if err != nil {
			t.Fatalf("Score: %v", err)
		}
		if ans.Score != 1 {
			t.Errorf("Score = %v, want 1", ans.Score)
		}
		q, ok := gotQuestions["q"].(map[string]any)
		if !ok || q["type"] != "score" {
			t.Errorf("questions = %#v, want a single score question keyed %q", gotQuestions, "q")
		}
	})

	t.Run("errors pass through unchanged", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusTooManyRequests, `{"message": "slow down"}`, nil)
		}, WithRetry(RetryPolicy{}))

		if _, err := c.Noul(context.Background(), "state", "q"); !errors.Is(err, ErrRateLimited) {
			t.Errorf("Noul() error = %v, want ErrRateLimited", err)
		}
		if _, err := c.Choice(context.Background(), "state", "q", Choices{"a": nil}); !errors.Is(err, ErrRateLimited) {
			t.Errorf("Choice() error = %v, want ErrRateLimited", err)
		}
		if _, err := c.Score(context.Background(), "state", "q", "a", "b"); !errors.Is(err, ErrRateLimited) {
			t.Errorf("Score() error = %v, want ErrRateLimited", err)
		}
	})
}

func TestModelsDecodesReleaseDate(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q, want /v1/models", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, `{
			"models": [
				{"name": "jev-latest", "description": "General-purpose model.", "release_date": "2026-09-15"}
			]
		}`, nil)
	})
	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(models))
	}
	m := models[0]
	if m.Name != "jev-latest" {
		t.Errorf("Name = %q, want jev-latest", m.Name)
	}
	want := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	if !m.ReleaseDate.Equal(want) {
		t.Errorf("ReleaseDate = %v, want %v", m.ReleaseDate, want)
	}
}

func TestUnprocessableEntityError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnprocessableEntity, `{
			"detail": [
				{"loc": ["body", "questions", "urgency", "criteria"], "msg": "Field required", "type": "missing"}
			]
		}`, map[string]string{"x-typesafe-request-id": "req-422"})
	}, WithRetry(RetryPolicy{}))
	_, err := c.Evaluate(context.Background(), "state", Noul("q", "q"))
	if err == nil {
		t.Fatal("Evaluate() error = nil, want error")
	}
	if !errors.Is(err, ErrUnprocessable) {
		t.Errorf("error = %v, want ErrUnprocessable", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if apiErr.StatusCode != 422 {
		t.Errorf("StatusCode = %d, want 422", apiErr.StatusCode)
	}
	if apiErr.RequestID != "req-422" {
		t.Errorf("RequestID = %q, want req-422", apiErr.RequestID)
	}
	if len(apiErr.Details) != 1 {
		t.Fatalf("len(Details) = %d, want 1", len(apiErr.Details))
	}
	if got := apiErr.Details[0].Path(); got != "questions.urgency.criteria" {
		t.Errorf("Details[0].Path() = %q, want questions.urgency.criteria", got)
	}
	if !strings.Contains(apiErr.Error(), "questions.urgency.criteria: Field required") {
		t.Errorf("Error() = %q, want it to contain the field path and message", apiErr.Error())
	}
	if !strings.Contains(apiErr.Error(), "req-422") {
		t.Errorf("Error() = %q, want it to contain the request id", apiErr.Error())
	}
}

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
	}, WithRetry(RetryPolicy{
		MaxRetries:      3,
		InitialBackoff:  time.Millisecond,
		MaxBackoff:      time.Millisecond,
		RetryConnErrors: true,
	}))

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
	},
		WithTimeout(10*time.Millisecond),
		WithRetry(RetryPolicy{MaxRetries: 1, InitialBackoff: time.Millisecond, RetryConnErrors: true}),
	)
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
	}, WithRetry(RetryPolicy{}))
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
	}, WithRetry(RetryPolicy{MaxRetries: 2, InitialBackoff: time.Millisecond, RetryConnErrors: true}))
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
	}, WithRetry(RetryPolicy{MaxRetries: 2, InitialBackoff: time.Millisecond, RetryConnErrors: true}))
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

func TestAPIErrorErrorFormat(t *testing.T) {
	err := &APIError{
		Method:     "POST",
		URL:        "https://api.typesafe.ai/v1/systemone",
		StatusCode: 422,
		Status:     "Unprocessable Entity",
		Message:    "questions.urgency.criteria: Field required",
		RequestID:  "abc",
	}
	got := err.Error()
	want := "sys1: POST https://api.typesafe.ai/v1/systemone: 422 Unprocessable Entity: questions.urgency.criteria: Field required (request id abc)"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAPIErrorIs(t *testing.T) {
	tests := []struct {
		status  int
		sentime error
	}{
		{401, ErrUnauthorized},
		{403, ErrForbidden},
		{404, ErrNotFound},
		{422, ErrUnprocessable},
		{429, ErrRateLimited},
		{529, ErrOverloaded},
		{500, ErrServer},
		{503, ErrServer},
	}
	for _, tt := range tests {
		err := &APIError{StatusCode: tt.status}
		if !errors.Is(err, tt.sentime) {
			t.Errorf("status %d: errors.Is = false, want true for %v", tt.status, tt.sentime)
		}
	}

	// A status that maps to one sentinel must not also match another.
	err := &APIError{StatusCode: 404}
	if errors.Is(err, ErrForbidden) {
		t.Error("404 matches ErrForbidden, want only ErrNotFound")
	}
}

func TestValidationErrorPath(t *testing.T) {
	tests := []struct {
		name string
		loc  []any
		want string
	}{
		{"drops leading body", []any{"body", "state"}, "state"},
		{"nested field", []any{"body", "questions", "urgency", "criteria"}, "questions.urgency.criteria"},
		{"array index", []any{"body", "questions", "urgency", "criteria", float64(2)}, "questions.urgency.criteria.2"},
		{"no body prefix", []any{"query", "limit"}, "query.limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ValidationError{Loc: tt.loc}
			if got := v.Path(); got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
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

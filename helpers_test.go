package sys1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

// newTestClient starts a test server around handler, closed when the
// test ends, and builds a Client pointed at it with an instant sleep
// function so retry tests don't actually sleep.
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

// slogTestLogger builds a debug-level text logger writing to w, for
// asserting on sys1's own log lines in tests.
func slogTestLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

const noulAskResponse = `{
	"model": "jev-1.13.0",
	"answers": {"q": {"type": "noul", "noul": 0.95}},
	"usage": {"input_tokens": 10, "output_tokens": 2}
}`

const choiceAskResponse = `{
	"model": "jev-1.13.0",
	"answers": {"q": {"type": "choice", "choice": "billing", "probabilities": {"billing": 1}, "confidence": 0.9}},
	"usage": {"input_tokens": 10, "output_tokens": 2}
}`

const scoreAskResponse = `{
	"model": "jev-1.13.0",
	"answers": {"q": {"type": "score", "score": 1, "legend": {"0": "a", "1": "b"}, "probabilities": {"0": 0, "1": 1}, "confidence": 0.9}},
	"usage": {"input_tokens": 10, "output_tokens": 2}
}`

// assertJSONEqual compares two JSON documents by decoded value rather
// than raw bytes, since map key order is not stable.
func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotVal, wantVal any
	if err := json.Unmarshal(got, &gotVal); err != nil {
		t.Fatalf("got is not valid JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("want is not valid JSON: %v\n%s", err, want)
	}
	if !reflect.DeepEqual(gotVal, wantVal) {
		t.Errorf("JSON mismatch:\n got: %s\nwant: %s", got, want)
	}
}

// mustDecodeAnswers decodes body as an Answers map, failing the test
// on a decode error.
func mustDecodeAnswers(t *testing.T, body string) Answers {
	t.Helper()
	var answers Answers
	if err := json.Unmarshal([]byte(body), &answers); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return answers
}

// customLevel is an int that marshals to a string, so validateState
// has to let it through despite its kind.
type customLevel int

func (c customLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("level-%d", c))
}

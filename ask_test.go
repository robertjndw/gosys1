package sys1

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestAskSendsDocumentedBody(t *testing.T) {
	var gotBody map[string]any
	var gotAuth, gotUA, gotMethod, gotPath, gotContentType string

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotContentType = r.Header.Get("Content-Type")
		gotBody = decodeBody(t, r)
		writeJSON(w, http.StatusOK, noulAskResponse, map[string]string{"x-typesafe-request-id": "req-123"})
	}, WithAPIKey("secret-key"))
	resp, err := c.Ask(context.Background(), "Help! My payouts have been failing for 3 days.",
		Noul("is_urgent", "Does this convey urgency?"),
	)
	if err != nil {
		t.Fatalf("Ask: %v", err)
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

func TestAskSpreadQuestionSlice(t *testing.T) {
	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		writeJSON(w, http.StatusOK, noulAskResponse, nil)
	}).WithModel("per-call-model")
	qs := []Question{
		Noul("billing", "About billing?"),
		Choice("tone", "Tone?", ChoiceCriteria{"calm": nil, "angry": nil}),
		Score("urgency", "Urgency?", Levels("low", "high")),
	}
	_, err := c.Ask(context.Background(), "state", qs...)
	if err != nil {
		t.Fatalf("Ask: %v", err)
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

func TestAskRejectsDuplicateQuestionName(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, http.StatusOK, noulAskResponse, nil)
	})
	_, err := c.Ask(context.Background(), "state", Noul("q", "a"), Noul("q", "b"))
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Ask() error = %v, want ErrInvalidRequest", err)
	}
	if called {
		t.Error("server was called despite a duplicate question name")
	}
}

func TestAskUsesDerivedModel(t *testing.T) {
	var gotModel string
	base := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotModel, _ = decodeBody(t, r)["model"].(string)
		writeJSON(w, http.StatusOK, noulAskResponse, nil)
	}).WithModel("client-default-model")
	c := base.WithModel("per-call-model")
	_, err := c.Ask(context.Background(), "state", Noul("q", "q"))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if gotModel != "per-call-model" {
		t.Errorf("model in body = %q, want per-call-model", gotModel)
	}
}

func TestAskUsesDerivedExtraBody(t *testing.T) {
	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		writeJSON(w, http.StatusOK, noulAskResponse, nil)
	}).WithExtraBody(map[string]any{"beam_width": float64(4)})
	_, err := c.Ask(context.Background(), "state", Noul("q", "q"))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got := gotBody["beam_width"]; got != float64(4) {
		t.Errorf("beam_width = %v, want 4", got)
	}
}

func TestAskRejectsExtraBodyCollidingWithBuiltinField(t *testing.T) {
	for _, key := range []string{"state", "model", "questions"} {
		t.Run(key, func(t *testing.T) {
			called := false
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				called = true
				writeJSON(w, http.StatusOK, noulAskResponse, nil)
			}).WithExtraBody(map[string]any{key: "collides"})
			_, err := c.Ask(context.Background(), "state", Noul("q", "q"))
			if !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("Ask() error = %v, want ErrInvalidRequest", err)
			}
			if called {
				t.Error("server was called despite a colliding extra body field")
			}
		})
	}
}

func TestAskUsesDerivedHeader(t *testing.T) {
	var gotHeader string
	base := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		writeJSON(w, http.StatusOK, noulAskResponse, nil)
	}).WithHeader("X-Custom", "client-value")
	c := base.WithHeader("X-Custom", "call-value")
	_, err := c.Ask(context.Background(), "state", Noul("q", "q"))
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if gotHeader != "call-value" {
		t.Errorf("X-Custom = %q, want call-value (the derived header should win)", gotHeader)
	}
}

func TestAskValidatesLocallyBeforeNetworkCall(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, http.StatusOK, noulAskResponse, nil)
	})
	_, err := c.Ask(context.Background(), "state")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Ask() error = %v, want ErrInvalidRequest", err)
	}
	if called {
		t.Error("server was called despite a client-side validation failure")
	}
}

func TestAskRejectsScalarState(t *testing.T) {
	n := 42
	var nilStr *string
	tests := []struct {
		name  string
		state Content
	}{
		{"int", 42},
		{"float64", 3.14},
		{"bool", true},
		{"pointer to int", &n},
		{"nil pointer", nilStr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				called = true
				writeJSON(w, http.StatusOK, noulAskResponse, nil)
			})
			_, err := c.Ask(context.Background(), tt.state, Noul("q", "q"))
			if !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("Ask() error = %v, want ErrInvalidRequest", err)
			}
			if called {
				t.Error("server was called despite a scalar state")
			}
		})
	}
}

func TestAskAcceptsNonScalarState(t *testing.T) {
	tests := []struct {
		name  string
		state Content
	}{
		{"string", "state"},
		{"map", map[string]any{"a": 1}},
		{"slice", []any{"a", "b"}},
		{"struct", struct{ A string }{"a"}},
		{"json.Marshaler over a scalar kind", customLevel(3)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				called = true
				writeJSON(w, http.StatusOK, noulAskResponse, nil)
			})
			_, err := c.Ask(context.Background(), tt.state, Noul("q", "q"))
			if err != nil {
				t.Fatalf("Ask() error = %v, want nil", err)
			}
			if !called {
				t.Error("server was not called for a valid state")
			}
		})
	}
}

func TestClientShortcuts(t *testing.T) {
	t.Run("Noul", func(t *testing.T) {
		var gotQuestions map[string]any
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotQuestions, _ = decodeBody(t, r)["questions"].(map[string]any)
			writeJSON(w, http.StatusOK, noulAskResponse, nil)
		})
		ans, err := c.Noul(context.Background(), "state", "Is this urgent?")
		if err != nil {
			t.Fatalf("Noul: %v", err)
		}
		if ans.Noul != 0.95 {
			t.Errorf("Noul.Noul = %v, want 0.95", ans.Noul)
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
			writeJSON(w, http.StatusOK, choiceAskResponse, nil)
		})
		ans, err := c.Choice(context.Background(), "state", "Which team?", ChoiceCriteria{"billing": nil, "technical": nil})
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
			writeJSON(w, http.StatusOK, scoreAskResponse, nil)
		})
		ans, err := c.Score(context.Background(), "state", "Rate this", Levels("low", "high"))
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

	t.Run("Score honours the derived model", func(t *testing.T) {
		var gotModel any
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotModel = decodeBody(t, r)["model"]
			writeJSON(w, http.StatusOK, scoreAskResponse, nil)
		}).WithModel("per-call-model")
		if _, err := c.Score(context.Background(), "state", "Rate this", Levels("low", "high")); err != nil {
			t.Fatalf("Score: %v", err)
		}
		if gotModel != "per-call-model" {
			t.Errorf("model = %v, want per-call-model", gotModel)
		}
	})

	t.Run("errors pass through unchanged", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusTooManyRequests, `{"message": "slow down"}`, nil)
		}).WithRetry(RetryPolicy{})

		if _, err := c.Noul(context.Background(), "state", "q"); !errors.Is(err, ErrRateLimited) {
			t.Errorf("Noul() error = %v, want ErrRateLimited", err)
		}
		if _, err := c.Choice(context.Background(), "state", "q", ChoiceCriteria{"a": nil}); !errors.Is(err, ErrRateLimited) {
			t.Errorf("Choice() error = %v, want ErrRateLimited", err)
		}
		if _, err := c.Score(context.Background(), "state", "q", Levels("a", "b")); !errors.Is(err, ErrRateLimited) {
			t.Errorf("Score() error = %v, want ErrRateLimited", err)
		}
	})
}

func TestCollectQuestions(t *testing.T) {
	t.Run("questions keyed by name", func(t *testing.T) {
		questions, err := collectQuestions([]Question{
			Noul("a", "q"),
			Score("b", "q", Levels("low", "high")),
		})
		if err != nil {
			t.Fatalf("collectQuestions: %v", err)
		}
		if len(questions) != 2 {
			t.Fatalf("got %d questions, want 2", len(questions))
		}
		if _, ok := questions["a"].(NoulQuestion); !ok {
			t.Errorf("questions[a] = %T, want NoulQuestion", questions["a"])
		}
		if _, ok := questions["b"].(ScoreQuestion); !ok {
			t.Errorf("questions[b] = %T, want ScoreQuestion", questions["b"])
		}
	})

	t.Run("a []Question built at runtime spreads like literal questions", func(t *testing.T) {
		labels := []string{"billing", "refund"}
		var qs []Question
		for _, l := range labels {
			qs = append(qs, Noul(l, "Is this about "+l+"?"))
		}
		qs = append(qs, Score("urgency", "q", Levels("low", "high")))
		questions, err := collectQuestions(qs)
		if err != nil {
			t.Fatalf("collectQuestions: %v", err)
		}
		for _, name := range []string{"billing", "refund", "urgency"} {
			if _, ok := questions[name]; !ok {
				t.Errorf("questions is missing %q: %v", name, questions)
			}
		}
		if len(questions) != 3 {
			t.Errorf("got %d questions, want 3", len(questions))
		}
	})

	for _, tt := range []struct {
		name string
		args []Question
	}{
		{"empty name", []Question{Noul("", "q")}},
		{"blank name", []Question{Noul("  ", "q")}},
		{"duplicate name", []Question{Noul("a", "q"), Score("a", "q", Levels("x", "y"))}},
		{"nil question", []Question{Noul("a", "q"), nil}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := collectQuestions(tt.args)
			if !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("collectQuestions = %v, want error wrapping ErrInvalidRequest", err)
			}
		})
	}
}

func TestValidateAskEmptyQuestions(t *testing.T) {
	err := validateAsk("state", map[string]Question{}, "model")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("validateAsk with empty questions = %v, want error wrapping ErrInvalidRequest", err)
	}
}

func TestValidateAskNilState(t *testing.T) {
	err := validateAsk(nil, map[string]Question{"q": Noul("q", "q")}, "model")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("validateAsk with nil state = %v, want error wrapping ErrInvalidRequest", err)
	}
}

func TestValidateAskEmptyModel(t *testing.T) {
	err := validateAsk("state", map[string]Question{"q": Noul("q", "q")}, "  ")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("validateAsk with blank model = %v, want error wrapping ErrInvalidRequest", err)
	}
}

func TestValidateAskPropagatesQuestionError(t *testing.T) {
	err := validateAsk("state", map[string]Question{"bad": ScoreQuestion{Name: "bad", Instructions: "q", Criteria: ScoreCriteria{"only one"}}}, "model")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("validateAsk with invalid question = %v, want error wrapping ErrInvalidRequest", err)
	}
}

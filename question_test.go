package sys1

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
)

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

func TestNoulQuestionMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		q    NoulQuestion
		want string
	}{
		{
			name: "string instructions, no criteria",
			q:    Noul("q", "Does this convey urgency?"),
			want: `{"type":"noul","instructions":"Does this convey urgency?"}`,
		},
		{
			name: "nil instructions",
			q:    Noul("q", nil),
			want: `{"type":"noul","instructions":null}`,
		},
		{
			name: "object instructions",
			q:    Noul("q", map[string]any{"question": "Is this spam?", "context": "email"}),
			want: `{"type":"noul","instructions":{"question":"Is this spam?","context":"email"}}`,
		},
		{
			name: "array instructions",
			q:    Noul("q", []any{"part one", "part two"}),
			want: `{"type":"noul","instructions":["part one","part two"]}`,
		},
		{
			name: "with criteria",
			q:    Noul("q", "Is this spam?").WithCriteria("Unsolicited advertising", "A legitimate conversation"),
			want: `{"type":"noul","instructions":"Is this spam?","criteria":{"true":"Unsolicited advertising","false":"A legitimate conversation"}}`,
		},
		{
			name: "with criteria, one side nil",
			q:    Noul("q", "Is this spam?").WithCriteria("Unsolicited advertising", nil),
			want: `{"type":"noul","instructions":"Is this spam?","criteria":{"true":"Unsolicited advertising"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.q)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			assertJSONEqual(t, got, tt.want)
		})
	}
}

func TestChoiceQuestionMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		q    ChoiceQuestion
		want string
	}{
		{
			name: "descriptions",
			q: Choice("q", "Which team should handle this?", Choices{
				"billing":   "Payments, invoicing, refunds",
				"technical": "Bugs, outages, integrations",
			}),
			want: `{"type":"choice","instructions":"Which team should handle this?","criteria":{"billing":"Payments, invoicing, refunds","technical":"Bugs, outages, integrations"}}`,
		},
		{
			name: "nil description means name only",
			q:    Choice("q", "What is the tone?", Choices{"calm": nil, "angry": nil}),
			want: `{"type":"choice","instructions":"What is the tone?","criteria":{"calm":null,"angry":null}}`,
		},
		{
			name: "object instructions",
			q:    Choice("q", map[string]any{"question": "Pick one"}, Choices{"a": nil, "b": nil}),
			want: `{"type":"choice","instructions":{"question":"Pick one"},"criteria":{"a":null,"b":null}}`,
		},
		{
			name: "array instructions",
			q:    Choice("q", []any{"a", "b"}, Choices{"x": nil, "y": nil}),
			want: `{"type":"choice","instructions":["a","b"],"criteria":{"x":null,"y":null}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.q)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			assertJSONEqual(t, got, tt.want)
		})
	}
}

func TestScoreQuestionMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		q    ScoreQuestion
		want string
	}{
		{
			name: "levels",
			q:    Score("q", "How frustrated is the customer?", Levels("Calm", "Frustrated", "Very angry")),
			want: `{"type":"score","instructions":"How frustrated is the customer?","criteria":["Calm","Frustrated","Very angry"]}`,
		},
		{
			name: "object instructions",
			q:    Score("q", map[string]any{"question": "Rate urgency"}, Levels("low", "high")),
			want: `{"type":"score","instructions":{"question":"Rate urgency"},"criteria":["low","high"]}`,
		},
		{
			name: "array instructions",
			q:    Score("q", []any{"a", "b"}, Levels("low", "high")),
			want: `{"type":"score","instructions":["a","b"],"criteria":["low","high"]}`,
		},
		{
			name: "structured levels",
			q:    Score("q", "Rate this", []Content{map[string]any{"label": "low"}, map[string]any{"label": "high"}}),
			want: `{"type":"score","instructions":"Rate this","criteria":[{"label":"low"},{"label":"high"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.q)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			assertJSONEqual(t, got, tt.want)
		})
	}
}

func TestQuestionValidate(t *testing.T) {
	tests := []struct {
		name    string
		q       Question
		wantErr bool
	}{
		{"noul, no criteria", Noul("q", "q"), false},
		{"noul, with criteria", Noul("q", "q").WithCriteria("y", "n"), false},
		{"choice, 0 options", ChoiceQuestion{Instructions: "q", Choices: Choices{}}, true},
		{"choice, 1 option", ChoiceQuestion{Instructions: "q", Choices: Choices{"a": nil}}, false},
		{"choice, 255 options", ChoiceQuestion{Instructions: "q", Choices: manyChoices(255)}, false},
		{"choice, 256 options", ChoiceQuestion{Instructions: "q", Choices: manyChoices(256)}, true},
		{"choice, empty option name", ChoiceQuestion{Instructions: "q", Choices: Choices{"": nil}}, true},
		{"score, 0 levels", ScoreQuestion{Instructions: "q", Levels: nil}, true},
		{"score, 1 level", ScoreQuestion{Instructions: "q", Levels: []Content{"a"}}, true},
		{"score, 2 levels", ScoreQuestion{Instructions: "q", Levels: []Content{"a", "b"}}, false},
		{"score, 10 levels", ScoreQuestion{Instructions: "q", Levels: manyLevels(10)}, false},
		{"score, 11 levels", ScoreQuestion{Instructions: "q", Levels: manyLevels(11)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.q.validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validate() = nil, want error")
				}
				if !errors.Is(err, ErrInvalidRequest) {
					t.Errorf("validate() error %v does not wrap ErrInvalidRequest", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate() = %v, want nil", err)
			}
		})
	}
}

func manyChoices(n int) Choices {
	o := make(Choices, n)
	for i := 0; i < n; i++ {
		o["opt"+strconv.Itoa(i)] = nil
	}
	return o
}

func manyLevels(n int) []Content {
	levels := make([]Content, n)
	for i := range levels {
		levels[i] = "level" + strconv.Itoa(i)
	}
	return levels
}

func TestRawQuestion(t *testing.T) {
	t.Run("marshals fields verbatim", func(t *testing.T) {
		q := Raw("billing", map[string]any{"type": "noul", "instructions": "About billing?", "weight": 2.0})
		got, err := json.Marshal(q)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		assertJSONEqual(t, got, `{"type":"noul","instructions":"About billing?","weight":2}`)
	})

	t.Run("Name", func(t *testing.T) {
		q := Raw("billing", map[string]any{"type": "noul"})
		if q.Name != "billing" {
			t.Errorf("Name = %q, want billing", q.Name)
		}
	})

	t.Run("Type from fields", func(t *testing.T) {
		q := Raw("q", map[string]any{"type": "choice"})
		if got := q.Type(); got != QuestionChoice {
			t.Errorf("Type() = %q, want %q", got, QuestionChoice)
		}
	})

	t.Run("Type missing", func(t *testing.T) {
		q := Raw("q", nil)
		if got := q.Type(); got != "" {
			t.Errorf("Type() = %q, want empty", got)
		}
	})

	t.Run("validate fails without type", func(t *testing.T) {
		q := Raw("q", map[string]any{"instructions": "x"})
		if err := q.validate(); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("validate() = %v, want error wrapping ErrInvalidRequest", err)
		}
	})

	t.Run("validate fails with empty type", func(t *testing.T) {
		q := Raw("q", map[string]any{"type": ""})
		if err := q.validate(); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("validate() = %v, want error wrapping ErrInvalidRequest", err)
		}
	})

	t.Run("validate succeeds with type", func(t *testing.T) {
		q := Raw("q", map[string]any{"type": "noul"})
		if err := q.validate(); err != nil {
			t.Errorf("validate() = %v, want nil", err)
		}
	})

	t.Run("satisfies Question", func(t *testing.T) {
		var _ Question = Raw("q", map[string]any{"type": "noul"})
	})
}

func TestQuestionName(t *testing.T) {
	tests := []struct {
		name string
		q    Question
	}{
		{"noul", Noul("noul_q", "q")},
		{"choice", Choice("choice_q", "q", Choices{"a": nil})},
		{"score", Score("score_q", "q", Levels("a", "b"))},
		{"raw", Raw("raw_q", map[string]any{"type": "noul"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.q.name(); got != tt.name+"_q" {
				t.Errorf("name() = %q, want %q", got, tt.name+"_q")
			}
		})
	}
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

func TestValidateEvaluateEmptyQuestions(t *testing.T) {
	err := validateEvaluate("state", map[string]Question{}, "model")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("validateEvaluate with empty questions = %v, want error wrapping ErrInvalidRequest", err)
	}
}

func TestValidateEvaluateNilState(t *testing.T) {
	err := validateEvaluate(nil, map[string]Question{"q": Noul("q", "q")}, "model")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("validateEvaluate with nil state = %v, want error wrapping ErrInvalidRequest", err)
	}
}

func TestValidateEvaluateEmptyModel(t *testing.T) {
	err := validateEvaluate("state", map[string]Question{"q": Noul("q", "q")}, "  ")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("validateEvaluate with blank model = %v, want error wrapping ErrInvalidRequest", err)
	}
}

func TestValidateEvaluatePropagatesQuestionError(t *testing.T) {
	err := validateEvaluate("state", map[string]Question{"bad": ScoreQuestion{Name: "bad", Instructions: "q", Levels: []Content{"only one"}}}, "model")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("validateEvaluate with invalid question = %v, want error wrapping ErrInvalidRequest", err)
	}
}

func TestNames(t *testing.T) {
	got := Names("calm", "angry")
	want := Choices{"calm": nil, "angry": nil}
	if len(got) != len(want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for name := range want {
		if v, ok := got[name]; !ok || v != nil {
			t.Errorf("Names()[%q] = %v, %v, want nil, true", name, v, ok)
		}
	}
	if got := Names(); len(got) != 0 {
		t.Errorf("Names() with no names = %v, want empty", got)
	}
}

func TestLevels(t *testing.T) {
	// The point of Levels is that an existing []string can be splatted
	// into it, which a []Content parameter alone would reject.
	names := []string{"low", "medium", "high"}
	got := Levels(names...)
	if len(got) != 3 {
		t.Fatalf("Levels() has %d entries, want 3", len(got))
	}
	for i, name := range names {
		if got[i] != name {
			t.Errorf("Levels()[%d] = %v, want %q", i, got[i], name)
		}
	}
	if got := Levels(); len(got) != 0 {
		t.Errorf("Levels() with no names = %v, want empty", got)
	}
}

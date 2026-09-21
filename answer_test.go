package sys1

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"
)

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

func TestAnswersUnmarshalJSON_Noul(t *testing.T) {
	body := `{
		"is_urgent": {"type": "noul", "noul": 0.95}
	}`
	answers := mustDecodeAnswers(t, body)
	a, err := answers.Noul("is_urgent")
	if err != nil {
		t.Fatalf("Noul: %v", err)
	}
	if a.Noul != 0.95 {
		t.Errorf("Noul.Noul = %v, want 0.95", a.Noul)
	}
}

func TestAnswersUnmarshalJSON_Choice(t *testing.T) {
	body := `{
		"department": {
			"type": "choice",
			"choice": "billing",
			"probabilities": {"billing": 0.88, "technical": 0.12, "sales": 0.0},
			"confidence": 0.81
		}
	}`
	answers := mustDecodeAnswers(t, body)
	a, err := answers.Choice("department")
	if err != nil {
		t.Fatalf("Choice: %v", err)
	}
	if a.Choice != "billing" {
		t.Errorf("Choice = %q, want billing", a.Choice)
	}
	if a.Confidence != 0.81 {
		t.Errorf("Confidence = %v, want 0.81", a.Confidence)
	}
	if got := a.Probabilities["billing"]; got != 0.88 {
		t.Errorf("Probabilities[billing] = %v, want 0.88", got)
	}
}

func TestAnswersUnmarshalJSON_Score(t *testing.T) {
	body := `{
		"frustration": {
			"type": "score",
			"score": 1.05,
			"legend": {"0": "Calm", "1": "Frustrated", "2": "Very angry"},
			"probabilities": {"0": 0.0, "1": 0.95, "2": 0.05},
			"confidence": 0.92
		}
	}`
	answers := mustDecodeAnswers(t, body)
	a, err := answers.Score("frustration")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if a.Score != 1.05 {
		t.Errorf("Score = %v, want 1.05", a.Score)
	}
	if a.Levels() != 3 {
		t.Errorf("Levels() = %d, want 3", a.Levels())
	}
	if got := a.Probability(1); got != 0.95 {
		t.Errorf("Probability(1) = %v, want 0.95", got)
	}
	if got := a.Probability(99); got != 0 {
		t.Errorf("Probability(99) = %v, want 0", got)
	}
}

func TestAnswersUnmarshalJSON_UnknownType(t *testing.T) {
	body := `{
		"future": {"type": "sentiment", "sentiment": "positive", "magnitude": 0.7}
	}`
	answers := mustDecodeAnswers(t, body)
	ans, ok := answers["future"]
	if !ok {
		t.Fatalf("missing answer for %q", "future")
	}
	raw, ok := ans.(RawAnswer)
	if !ok {
		t.Fatalf("answer is %T, want RawAnswer", ans)
	}
	if raw.Kind != "sentiment" {
		t.Errorf("Kind = %q, want sentiment", raw.Kind)
	}

	var roundTrip map[string]any
	if err := json.Unmarshal(raw.Data, &roundTrip); err != nil {
		t.Fatalf("raw.Data does not round-trip: %v", err)
	}
	if roundTrip["sentiment"] != "positive" {
		t.Errorf("raw.Data sentiment = %v, want positive", roundTrip["sentiment"])
	}

	// The typed accessors report a type mismatch rather than an error
	// about the unknown kind, and never panic.
	if _, err := answers.Noul("future"); !errors.Is(err, ErrAnswerType) {
		t.Errorf("Noul(future) error = %v, want ErrAnswerType", err)
	}
}

func TestAnswersUnmarshalJSON_UnknownFieldsIgnored(t *testing.T) {
	body := `{
		"is_urgent": {"type": "noul", "noul": 0.5, "extra_field": "ignored", "model_debug": {"foo": 1}}
	}`
	answers := mustDecodeAnswers(t, body)
	a, err := answers.Noul("is_urgent")
	if err != nil {
		t.Fatalf("Noul: %v", err)
	}
	if a.Noul != 0.5 {
		t.Errorf("Noul.Noul = %v, want 0.5", a.Noul)
	}
}

func TestAnswersAccessors(t *testing.T) {
	answers := Answers{
		"noul_key":   NoulAnswer{Noul: 0.1},
		"choice_key": ChoiceAnswer{Choice: "a", Probabilities: map[string]float64{"a": 1}, Confidence: 1},
		"score_key":  ScoreAnswer{Score: 1, Legend: []Content{"a"}, Probabilities: []float64{1}, Confidence: 1},
	}

	t.Run("hit", func(t *testing.T) {
		if _, err := answers.Noul("noul_key"); err != nil {
			t.Errorf("Noul(noul_key) = %v, want nil", err)
		}
		if _, err := answers.Choice("choice_key"); err != nil {
			t.Errorf("Choice(choice_key) = %v, want nil", err)
		}
		if _, err := answers.Score("score_key"); err != nil {
			t.Errorf("Score(score_key) = %v, want nil", err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		_, err := answers.Noul("nope")
		if !errors.Is(err, ErrNoAnswer) {
			t.Errorf("Noul(nope) error = %v, want ErrNoAnswer", err)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		_, err := answers.Choice("noul_key")
		if !errors.Is(err, ErrAnswerType) {
			t.Errorf("Choice(noul_key) error = %v, want ErrAnswerType", err)
		}
		_, err = answers.Score("choice_key")
		if !errors.Is(err, ErrAnswerType) {
			t.Errorf("Score(choice_key) error = %v, want ErrAnswerType", err)
		}
		_, err = answers.Noul("score_key")
		if !errors.Is(err, ErrAnswerType) {
			t.Errorf("Noul(score_key) error = %v, want ErrAnswerType", err)
		}
	})
}

func TestResponseRoundTrip(t *testing.T) {
	original := Response{
		Model: "jev-1.13.0",
		Answers: Answers{
			"is_urgent": NoulAnswer{Noul: 0.95},
			"department": ChoiceAnswer{
				Choice:        "billing",
				Probabilities: map[string]float64{"billing": 0.88, "technical": 0.12},
				Confidence:    0.81,
			},
			"frustration": ScoreAnswer{
				Score:         1.05,
				Legend:        []Content{"Calm", "Frustrated", "Very angry"},
				Probabilities: []float64{0.0, 0.95, 0.05},
				Confidence:    0.92,
			},
		},
		Usage: Usage{InputTokens: 296, OutputTokens: 20},
	}

	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Model != original.Model {
		t.Errorf("Model = %q, want %q", decoded.Model, original.Model)
	}
	if decoded.Usage != original.Usage {
		t.Errorf("Usage = %+v, want %+v", decoded.Usage, original.Usage)
	}
	noul, err := decoded.Answers.Noul("is_urgent")
	if err != nil || noul.Noul != 0.95 {
		t.Errorf("Answers.Noul(is_urgent) = %v, %v, want 0.95, nil", noul, err)
	}
	choice, err := decoded.Answers.Choice("department")
	if err != nil || choice.Choice != "billing" {
		t.Errorf("Answers.Choice(department) = %v, %v, want billing, nil", choice, err)
	}
	score, err := decoded.Answers.Score("frustration")
	if err != nil || score.Score != 1.05 {
		t.Errorf("Answers.Score(frustration) = %v, %v, want 1.05, nil", score, err)
	}
}

func TestRawAnswerMarshalJSON_EmptyData(t *testing.T) {
	b, err := json.Marshal(RawAnswer{Kind: "unknown"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != "null" {
		t.Errorf("Marshal = %s, want null", b)
	}
}

func TestChoiceAnswerRanked(t *testing.T) {
	a := ChoiceAnswer{
		Choice: "billing",
		Probabilities: map[string]float64{
			"technical": 0.2,
			"billing":   0.6,
			"sales":     0.2,
		},
	}
	got := a.Ranked()
	// Equal probabilities fall back to name order so the result is
	// stable across runs.
	want := []string{"billing", "sales", "technical"}
	if !slices.Equal(got, want) {
		t.Errorf("Ranked() = %v, want %v", got, want)
	}
	if got := (ChoiceAnswer{}).Ranked(); len(got) != 0 {
		t.Errorf("Ranked() on an empty answer = %v, want empty", got)
	}
}

func TestScoreAnswerLevelHelpers(t *testing.T) {
	a := ScoreAnswer{
		Score:         1.6,
		Legend:        []Content{"Calm", "Frustrated", "Very angry"},
		Probabilities: []float64{0.05, 0.3, 0.65},
	}
	if got := a.Nearest(); got != 2 {
		t.Errorf("Nearest() = %d, want 2", got)
	}
	if got := a.Label(a.Nearest()); got != "Very angry" {
		t.Errorf("Label(%d) = %v, want Very angry", a.Nearest(), got)
	}
	if got := a.Label(7); got != nil {
		t.Errorf("Label(7) = %v, want nil", got)
	}
	if got := a.Label(-1); got != nil {
		t.Errorf("Label(-1) = %v, want nil", got)
	}
	if got := a.Probability(2); got != 0.65 {
		t.Errorf("Probability(2) = %v, want 0.65", got)
	}
	if got := a.Probability(7); got != 0 {
		t.Errorf("Probability(7) = %v, want 0", got)
	}
	if got := a.Probability(-1); got != 0 {
		t.Errorf("Probability(-1) = %v, want 0", got)
	}
	if got := a.Levels(); got != 3 {
		t.Errorf("Levels() = %d, want 3", got)
	}
}

func TestScoreAnswerNearestClamps(t *testing.T) {
	tests := []struct {
		name string
		a    ScoreAnswer
		want int
	}{
		{"below range", ScoreAnswer{Score: -3, Legend: []Content{"a", "b", "c"}}, 0},
		{"above range", ScoreAnswer{Score: 12, Legend: []Content{"a", "b", "c"}}, 2},
		{"in range", ScoreAnswer{Score: 1.4, Legend: []Content{"a", "b", "c"}}, 1},
		{"no levels", ScoreAnswer{Score: 7}, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Nearest(); got != tt.want {
				t.Errorf("Nearest() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestScoreAnswerUnmarshalJSON(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		body := `{"score": 1.05, "legend": {"1": "Frustrated", "0": "Calm", "2": "Very angry"}, "probabilities": {"0": 0.0, "2": 0.05, "1": 0.95}, "confidence": 0.92}`
		var a ScoreAnswer
		if err := json.Unmarshal([]byte(body), &a); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		wantLegend := []Content{"Calm", "Frustrated", "Very angry"}
		if !reflect.DeepEqual(a.Legend, wantLegend) {
			t.Errorf("Legend = %v, want %v", a.Legend, wantLegend)
		}
		wantProbabilities := []float64{0.0, 0.95, 0.05}
		if !reflect.DeepEqual(a.Probabilities, wantProbabilities) {
			t.Errorf("Probabilities = %v, want %v", a.Probabilities, wantProbabilities)
		}
	})

	t.Run("gap in indices fails", func(t *testing.T) {
		body := `{"score": 0, "legend": {"0": "a", "2": "c"}, "probabilities": {"0": 0, "2": 1}, "confidence": 0}`
		var a ScoreAnswer
		if err := json.Unmarshal([]byte(body), &a); err == nil {
			t.Fatal("Unmarshal() = nil, want error")
		}
	})

	t.Run("mismatched lengths fails", func(t *testing.T) {
		body := `{"score": 0, "legend": {"0": "a", "1": "b"}, "probabilities": {"0": 0}, "confidence": 0}`
		var a ScoreAnswer
		if err := json.Unmarshal([]byte(body), &a); err == nil {
			t.Fatal("Unmarshal() = nil, want error")
		}
	})

	t.Run("negative key fails", func(t *testing.T) {
		body := `{"score": 0, "legend": {"-1": "a", "0": "b"}, "probabilities": {"-1": 0, "0": 1}, "confidence": 0}`
		var a ScoreAnswer
		if err := json.Unmarshal([]byte(body), &a); err == nil {
			t.Fatal("Unmarshal() = nil, want error")
		}
	})

	t.Run("non-integer key fails", func(t *testing.T) {
		body := `{"score": 0, "legend": {"a": "a", "0": "b"}, "probabilities": {"a": 0, "0": 1}, "confidence": 0}`
		var a ScoreAnswer
		if err := json.Unmarshal([]byte(body), &a); err == nil {
			t.Fatal("Unmarshal() = nil, want error")
		}
	})
}

func TestScoreAnswerMarshalJSONRoundTripsWireShape(t *testing.T) {
	a := ScoreAnswer{
		Score:         1.05,
		Legend:        []Content{"Calm", "Frustrated", "Very angry"},
		Probabilities: []float64{0.0, 0.95, 0.05},
		Confidence:    0.92,
	}
	got, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{
		"type": "score",
		"score": 1.05,
		"legend": {"0": "Calm", "1": "Frustrated", "2": "Very angry"},
		"probabilities": {"0": 0.0, "1": 0.95, "2": 0.05},
		"confidence": 0.92
	}`
	assertJSONEqual(t, got, want)

	var decoded ScoreAnswer
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(decoded, a) {
		t.Errorf("round trip = %+v, want %+v", decoded, a)
	}
}

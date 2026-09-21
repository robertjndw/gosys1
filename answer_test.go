package sys1

import (
	"encoding/json"
	"errors"
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
		t.Errorf("Noul = %v, want 0.95", a.Noul)
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
		t.Errorf("Noul = %v, want 0.5", a.Noul)
	}
}

func TestAnswersAccessors(t *testing.T) {
	answers := Answers{
		"noul_key":   NoulAnswer{Noul: 0.1},
		"choice_key": ChoiceAnswer{Choice: "a", Probabilities: map[string]float64{"a": 1}, Confidence: 1},
		"score_key":  ScoreAnswer{Score: 1, Legend: map[string]Content{"0": "a"}, Probabilities: map[string]float64{"0": 1}, Confidence: 1},
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
				Legend:        map[string]Content{"0": "Calm", "1": "Frustrated", "2": "Very angry"},
				Probabilities: map[string]float64{"0": 0.0, "1": 0.95, "2": 0.05},
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

package sys1

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Content is the payload carried by state, instructions and criteria
// values sent to the API. It is documented as string, map[string]any,
// []any, or a struct that marshals to one of those; nil is allowed
// wherever the API accepts JSON null.
type Content any

// QuestionType identifies which of the three question shapes a
// Question is.
type QuestionType string

// The question types the API understands.
const (
	QuestionNoul   QuestionType = "noul"
	QuestionChoice QuestionType = "choice"
	QuestionScore  QuestionType = "score"
)

// EvaluateArg is anything Client.Evaluate accepts after state: a named
// Question, or a RequestOption that adjusts the call itself. Both
// share one variadic list so a call reads as "this state, these
// questions, these overrides" without a second signature. EvaluateArg
// is sealed to this package.
type EvaluateArg interface {
	evaluateArg()
}

// Question is a single named question sent alongside state. It is
// sealed to this package: NoulQuestion, ChoiceQuestion, ScoreQuestion
// and RawQuestion are the only implementations, so a type switch over
// Question is exhaustive.
type Question interface {
	EvaluateArg

	// Name reports the key the question is sent under. Its answer
	// comes back under the same key in Response.Answers.
	Name() string

	// Type reports the question's wire type.
	Type() QuestionType

	// validate checks the question's own invariants, independent of
	// its name or the rest of the request. It is unexported to keep
	// Question sealed.
	validate() error
}

// NoulQuestion is a yes/no question. The answer is the probability
// that the answer is yes.
type NoulQuestion struct {
	// Key is the name the question is sent under.
	Key string
	// Instructions is the yes/no question or statement to evaluate.
	Instructions Content
	// Criteria optionally clarifies what counts as a yes or no answer.
	Criteria *NoulCriteria
}

// NoulCriteria clarifies what a yes and a no answer mean for a
// NoulQuestion.
type NoulCriteria struct {
	// True describes what counts as a yes (value near 1) answer.
	True Content `json:"true,omitempty"`
	// False describes what counts as a no (value near 0) answer.
	False Content `json:"false,omitempty"`
}

// Noul builds a yes/no question named name. Use WithCriteria to
// clarify what counts as yes or no.
//
// This is the question constructor; for a one-shot yes/no call that
// skips Evaluate and the Answers map entirely, see Client.Noul.
func Noul(name string, instructions Content) NoulQuestion {
	return NoulQuestion{Key: name, Instructions: instructions}
}

// WithCriteria returns a copy of q with criteria describing what
// counts as a yes (yes) and a no (no) answer.
func (q NoulQuestion) WithCriteria(yes, no Content) NoulQuestion {
	q.Criteria = &NoulCriteria{True: yes, False: no}
	return q
}

// Name implements Question.
func (q NoulQuestion) Name() string { return q.Key }

// Type implements Question.
func (q NoulQuestion) Type() QuestionType { return QuestionNoul }

func (q NoulQuestion) validate() error { return nil }

func (q NoulQuestion) evaluateArg() {}

// MarshalJSON implements json.Marshaler.
func (q NoulQuestion) MarshalJSON() ([]byte, error) {
	// A nil *NoulCriteria must become an untyped nil so omitempty
	// drops the key instead of emitting null.
	var criteria Content
	if q.Criteria != nil {
		criteria = q.Criteria
	}
	return marshalQuestion(QuestionNoul, q.Instructions, criteria)
}

// ChoiceQuestion picks one option from a fixed set. Choices is a map
// of option name to a description; a nil description means the option
// is interpreted by its name alone.
type ChoiceQuestion struct {
	// Key is the name the question is sent under.
	Key string
	// Instructions is what the model should decide.
	Instructions Content
	// Criteria maps each option name to a description, or nil for
	// "name only". It must have between 1 and 255 entries.
	Criteria Choices
}

// Choices maps a Choice question's option names to their descriptions.
// A nil value means the option needs no extra detail.
type Choices map[string]Content

// Choice builds a choice question named name over the given choices.
//
// This is the question constructor; for a one-shot choice call that
// skips Evaluate and the Answers map entirely, see Client.Choice.
func Choice(name string, instructions Content, choices Choices) ChoiceQuestion {
	return ChoiceQuestion{Key: name, Instructions: instructions, Criteria: choices}
}

// Name implements Question.
func (q ChoiceQuestion) Name() string { return q.Key }

// Type implements Question.
func (q ChoiceQuestion) Type() QuestionType { return QuestionChoice }

func (q ChoiceQuestion) validate() error {
	n := len(q.Criteria)
	if n < 1 || n > 255 {
		return fmt.Errorf("%w: choice question needs 1 to 255 options, got %d", ErrInvalidRequest, n)
	}
	for name := range q.Criteria {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%w: choice question option name must not be empty", ErrInvalidRequest)
		}
	}
	return nil
}

func (q ChoiceQuestion) evaluateArg() {}

// MarshalJSON implements json.Marshaler.
func (q ChoiceQuestion) MarshalJSON() ([]byte, error) {
	return marshalQuestion(QuestionChoice, q.Instructions, q.Criteria)
}

// ScoreQuestion rates state along an ordered rubric. Criteria is the
// ordered list of level descriptions; the answer is a
// probability-weighted position among them, starting at 0.
type ScoreQuestion struct {
	// Key is the name the question is sent under.
	Key string
	// Instructions is what the model should rate.
	Instructions Content
	// Criteria is the ordered list of level descriptions. It must have
	// between 2 and 10 entries.
	Criteria []Content
}

// Score builds a score question named name over the given ordered
// levels.
//
// This is the question constructor; for a one-shot score call that
// skips Evaluate and the Answers map entirely, see Client.Score.
func Score(name string, instructions Content, levels ...Content) ScoreQuestion {
	return ScoreQuestion{Key: name, Instructions: instructions, Criteria: levels}
}

// Name implements Question.
func (q ScoreQuestion) Name() string { return q.Key }

// Type implements Question.
func (q ScoreQuestion) Type() QuestionType { return QuestionScore }

func (q ScoreQuestion) validate() error {
	n := len(q.Criteria)
	if n < 2 || n > 10 {
		return fmt.Errorf("%w: score question needs 2 to 10 levels, got %d", ErrInvalidRequest, n)
	}
	return nil
}

func (q ScoreQuestion) evaluateArg() {}

// MarshalJSON implements json.Marshaler.
func (q ScoreQuestion) MarshalJSON() ([]byte, error) {
	return marshalQuestion(QuestionScore, q.Instructions, q.Criteria)
}

// marshalQuestion emits the wire object shared by the typed questions,
// with the "type" discriminator the API expects. The name is not part
// of the object; it is the key the object sits under in the request.
// criteria is omitted only when it is an untyped nil, so a typed nil
// map or slice still marshals as null.
func marshalQuestion(t QuestionType, instructions, criteria Content) ([]byte, error) {
	return json.Marshal(struct {
		Type         QuestionType `json:"type"`
		Instructions Content      `json:"instructions"`
		Criteria     Content      `json:"criteria,omitempty"`
	}{t, instructions, criteria})
}

// RawQuestion is an escape hatch for question fields this library
// predates: Fields marshals verbatim and is accepted anywhere a
// Question is, as long as it carries a non-empty string "type" key.
type RawQuestion struct {
	// Key is the name the question is sent under.
	Key string
	// Fields is the complete question object as sent to the API.
	Fields map[string]any
}

// Raw builds a RawQuestion named name from a verbatim question object.
func Raw(name string, fields map[string]any) RawQuestion {
	return RawQuestion{Key: name, Fields: fields}
}

// Name implements Question.
func (q RawQuestion) Name() string { return q.Key }

// Type implements Question, reading the "type" key from Fields. It
// returns "" if the key is missing or not a string.
func (q RawQuestion) Type() QuestionType {
	if t, ok := q.Fields["type"].(string); ok {
		return QuestionType(t)
	}
	return ""
}

func (q RawQuestion) validate() error {
	t, ok := q.Fields["type"].(string)
	if !ok || strings.TrimSpace(t) == "" {
		return fmt.Errorf(`%w: raw question must have a non-empty string "type" field`, ErrInvalidRequest)
	}
	return nil
}

func (q RawQuestion) evaluateArg() {}

// MarshalJSON implements json.Marshaler, emitting Fields unchanged.
func (q RawQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(q.Fields)
}

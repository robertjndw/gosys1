package sys1

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Content is the payload carried by state, instructions and criteria
// values sent to the API. It is documented as string, map[string]any,
// []any, or a struct that marshals to one of those; nil is allowed for
// optional values such as a choice description or a criteria side, but
// Ask rejects a nil state.
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

// Question is a single named question sent alongside state. It is
// sealed to this package: NoulQuestion, ChoiceQuestion, ScoreQuestion
// and RawQuestion are the only implementations, so a type switch over
// Question is exhaustive.
type Question interface {
	// name reports the key the question is sent under. Its answer
	// comes back under the same key in Response.Answers. It is
	// unexported because a struct with an exported Name field cannot
	// also have an exported Name method.
	name() string

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
	// Name is the name the question is sent under.
	Name string
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
// For a one-shot call that skips Ask and the Answers map, see
// Client.Noul.
func Noul(name string, instructions Content) NoulQuestion {
	return NoulQuestion{Name: name, Instructions: instructions}
}

// WithCriteria returns a copy of q with criteria describing what
// counts as a yes (yes) and a no (no) answer.
func (q NoulQuestion) WithCriteria(yes, no Content) NoulQuestion {
	q.Criteria = &NoulCriteria{True: yes, False: no}
	return q
}

// name implements Question.
func (q NoulQuestion) name() string { return q.Name }

// Type implements Question.
func (q NoulQuestion) Type() QuestionType { return QuestionNoul }

func (q NoulQuestion) validate() error { return nil }

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

// ChoiceQuestion picks one option from a fixed set. Criteria is a map
// of option name to a description; a nil description means the option
// is interpreted by its name alone.
type ChoiceQuestion struct {
	// Name is the name the question is sent under.
	Name string
	// Instructions is what the model should decide.
	Instructions Content
	// Criteria maps each option name to a description, or nil for
	// "name only". It must have between 1 and 255 entries.
	Criteria ChoiceCriteria
}

// ChoiceCriteria maps a Choice question's option names to their
// descriptions. A nil value means the option needs no extra detail;
// see [Choices] for building one from names alone.
type ChoiceCriteria map[string]Content

// Choices builds the ChoiceCriteria for a choice question from option
// names alone, with no descriptions:
//
//	sys1.Choice("tone", "What is the tone?", sys1.Choices("calm", "angry"))
//
// Use a ChoiceCriteria literal when some options need a description.
func Choices(names ...string) ChoiceCriteria {
	c := make(ChoiceCriteria, len(names))
	for _, name := range names {
		c[name] = nil
	}
	return c
}

// Choice builds a choice question named name over the given options.
//
// For a one-shot call that skips Ask and the Answers map, see
// Client.Choice.
func Choice(name string, instructions Content, criteria ChoiceCriteria) ChoiceQuestion {
	return ChoiceQuestion{Name: name, Instructions: instructions, Criteria: criteria}
}

// name implements Question.
func (q ChoiceQuestion) name() string { return q.Name }

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

// MarshalJSON implements json.Marshaler.
func (q ChoiceQuestion) MarshalJSON() ([]byte, error) {
	return marshalQuestion(QuestionChoice, q.Instructions, q.Criteria)
}

// ScoreQuestion rates state along an ordered rubric. Criteria is the
// ordered list of level descriptions; the answer is a
// probability-weighted position among them, starting at 0.
type ScoreQuestion struct {
	// Name is the name the question is sent under.
	Name string
	// Instructions is what the model should rate.
	Instructions Content
	// Criteria is the ordered list of level descriptions, lowest
	// first. It must have between 2 and 10 entries.
	Criteria ScoreCriteria
}

// ScoreCriteria is a Score question's ordered list of level
// descriptions, lowest first. Each entry's position is its score,
// starting at 0. See [Levels] for building one from plain strings.
type ScoreCriteria []Content

// Score builds a score question named name over the given ordered
// levels, lowest first. [Levels] builds the list from plain strings;
// a ScoreCriteria literal allows structured level descriptions.
//
// For a one-shot call that skips Ask and the Answers map, see
// [Client.Score].
func Score(name string, instructions Content, criteria ScoreCriteria) ScoreQuestion {
	return ScoreQuestion{Name: name, Instructions: instructions, Criteria: criteria}
}

// Levels builds the ScoreCriteria for a score question from plain
// strings, lowest first:
//
//	sys1.Score("urgency", "How urgent is this?", sys1.Levels("low", "medium", "high"))
//
// It accepts an existing []string via levels..., which a []Content
// parameter alone would not.
func Levels(levels ...string) ScoreCriteria {
	out := make(ScoreCriteria, len(levels))
	for i, l := range levels {
		out[i] = l
	}
	return out
}

// name implements Question.
func (q ScoreQuestion) name() string { return q.Name }

// Type implements Question.
func (q ScoreQuestion) Type() QuestionType { return QuestionScore }

func (q ScoreQuestion) validate() error {
	n := len(q.Criteria)
	if n < 2 || n > 10 {
		return fmt.Errorf("%w: score question needs 2 to 10 levels, got %d", ErrInvalidRequest, n)
	}
	return nil
}

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
	// Name is the name the question is sent under.
	Name string
	// Fields is the complete question object as sent to the API.
	Fields map[string]any
}

// Raw builds a RawQuestion named name from a verbatim question object.
func Raw(name string, fields map[string]any) RawQuestion {
	return RawQuestion{Name: name, Fields: fields}
}

// name implements Question.
func (q RawQuestion) name() string { return q.Name }

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

// MarshalJSON implements json.Marshaler, emitting Fields unchanged.
func (q RawQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(q.Fields)
}

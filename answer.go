package sys1

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// AnswerType identifies which of the three answer shapes an Answer is,
// or carries an unrecognized wire type for a RawAnswer.
type AnswerType string

// The answer types the API returns, matching the question types.
const (
	AnswerNoul   AnswerType = "noul"
	AnswerChoice AnswerType = "choice"
	AnswerScore  AnswerType = "score"
)

// Answer is a single named answer in a Response. It is sealed to this
// package: NoulAnswer, ChoiceAnswer, ScoreAnswer and RawAnswer are the
// only implementations, so a type switch over Answer is exhaustive.
type Answer interface {
	// Type reports the answer's wire type.
	Type() AnswerType

	// answer is unexported to keep Answer sealed.
	answer()
}

// NoulAnswer is the answer to a NoulQuestion.
type NoulAnswer struct {
	// Noul is the probability of a yes answer, from 0 to 1.
	Noul float64 `json:"noul"`
}

// Type implements Answer.
func (a NoulAnswer) Type() AnswerType { return AnswerNoul }

func (a NoulAnswer) answer() {}

// MarshalJSON implements json.Marshaler, emitting the "type"
// discriminator so a Response round-trips.
func (a NoulAnswer) MarshalJSON() ([]byte, error) {
	type plain NoulAnswer
	return json.Marshal(struct {
		Type AnswerType `json:"type"`
		plain
	}{AnswerNoul, plain(a)})
}

// ChoiceAnswer is the answer to a ChoiceQuestion.
type ChoiceAnswer struct {
	// Choice is the option with the highest probability.
	Choice string `json:"choice"`
	// Probabilities maps every option to its probability; values sum
	// to approximately 1.
	Probabilities map[string]float64 `json:"probabilities"`
	// Confidence is how certain the model is, from 0 to 1.
	Confidence float64 `json:"confidence"`
}

// Type implements Answer.
func (a ChoiceAnswer) Type() AnswerType { return AnswerChoice }

func (a ChoiceAnswer) answer() {}

// MarshalJSON implements json.Marshaler, emitting the "type"
// discriminator so a Response round-trips.
func (a ChoiceAnswer) MarshalJSON() ([]byte, error) {
	type plain ChoiceAnswer
	return json.Marshal(struct {
		Type AnswerType `json:"type"`
		plain
	}{AnswerChoice, plain(a)})
}

// ScoreAnswer is the answer to a ScoreQuestion.
type ScoreAnswer struct {
	// Score is the probability-weighted answer across the levels; it
	// can land between levels.
	Score float64 `json:"score"`
	// Legend maps each level index (as a string key, "0", "1", ...) to
	// its description.
	Legend map[string]Content `json:"legend"`
	// Probabilities maps each level index (as a string key, matching
	// Legend) to its probability.
	Probabilities map[string]float64 `json:"probabilities"`
	// Confidence is how certain the model is, from 0 to 1.
	Confidence float64 `json:"confidence"`
}

// Type implements Answer.
func (a ScoreAnswer) Type() AnswerType { return AnswerScore }

func (a ScoreAnswer) answer() {}

// MarshalJSON implements json.Marshaler, emitting the "type"
// discriminator so a Response round-trips.
func (a ScoreAnswer) MarshalJSON() ([]byte, error) {
	type plain ScoreAnswer
	return json.Marshal(struct {
		Type AnswerType `json:"type"`
		plain
	}{AnswerScore, plain(a)})
}

// Levels reports the number of score levels in the answer's legend.
func (a ScoreAnswer) Levels() int { return len(a.Legend) }

// Probability returns the probability of the given level index. It
// returns 0 if the level is not present, e.g. because the index is out
// of range.
func (a ScoreAnswer) Probability(level int) float64 {
	return a.Probabilities[strconv.Itoa(level)]
}

// RawAnswer holds an answer whose "type" this library does not
// recognize, keeping the complete answer object rather than dropping
// it. Decoding a Response never fails because of an unknown answer
// type; it becomes a RawAnswer instead.
type RawAnswer struct {
	// Kind is the unrecognized "type" value from the wire.
	Kind AnswerType
	// Data is the complete, undecoded answer object.
	Data json.RawMessage
}

// Type implements Answer, returning Kind.
func (a RawAnswer) Type() AnswerType { return a.Kind }

func (a RawAnswer) answer() {}

// MarshalJSON implements json.Marshaler, re-emitting the original
// answer bytes unchanged.
func (a RawAnswer) MarshalJSON() ([]byte, error) {
	if len(a.Data) == 0 {
		return []byte("null"), nil
	}
	return a.Data, nil
}

// Answers is a batch of named answers, keyed by the names the questions
// were sent under.
type Answers map[string]Answer

// UnmarshalJSON implements json.Unmarshaler. Each entry is dispatched
// on its "type" field into the matching concrete Answer type; an
// unrecognized type becomes a RawAnswer rather than an error, so a
// newer server answer type never breaks decoding.
func (a *Answers) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	out := make(Answers, len(raw))
	for key, msg := range raw {
		var head struct {
			Type AnswerType `json:"type"`
		}
		if err := json.Unmarshal(msg, &head); err != nil {
			return fmt.Errorf("sys1: decoding answer %q: %w", key, err)
		}

		var v Answer
		var err error
		switch head.Type {
		case AnswerNoul:
			v, err = decodeAnswer[NoulAnswer](key, msg)
		case AnswerChoice:
			v, err = decodeAnswer[ChoiceAnswer](key, msg)
		case AnswerScore:
			v, err = decodeAnswer[ScoreAnswer](key, msg)
		default:
			// msg is already a private copy: decoding into a
			// json.RawMessage clones the bytes.
			v = RawAnswer{Kind: head.Type, Data: msg}
		}
		if err != nil {
			return err
		}
		out[key] = v
	}

	*a = out
	return nil
}

// decodeAnswer unmarshals one answer object into the concrete type T.
func decodeAnswer[T Answer](key string, msg json.RawMessage) (Answer, error) {
	var v T
	if err := json.Unmarshal(msg, &v); err != nil {
		return nil, fmt.Errorf("sys1: decoding answer %q: %w", key, err)
	}
	return v, nil
}

// answerAs looks up name and asserts its answer is of type T, wrapping
// ErrNoAnswer or ErrAnswerType on failure.
func answerAs[T Answer](a Answers, name string) (T, error) {
	var zero T
	ans, ok := a[name]
	if !ok {
		return zero, fmt.Errorf("%w: %q", ErrNoAnswer, name)
	}
	v, ok := ans.(T)
	if !ok {
		return zero, fmt.Errorf("%w: %q is %s, not %s", ErrAnswerType, name, ans.Type(), zero.Type())
	}
	return v, nil
}

// Noul returns the NoulAnswer for name, or an error wrapping
// ErrNoAnswer if name is absent or ErrAnswerType if it is a different
// answer type.
func (a Answers) Noul(name string) (NoulAnswer, error) {
	return answerAs[NoulAnswer](a, name)
}

// Choice returns the ChoiceAnswer for name, or an error wrapping
// ErrNoAnswer if name is absent or ErrAnswerType if it is a different
// answer type.
func (a Answers) Choice(name string) (ChoiceAnswer, error) {
	return answerAs[ChoiceAnswer](a, name)
}

// Score returns the ScoreAnswer for name, or an error wrapping
// ErrNoAnswer if name is absent or ErrAnswerType if it is a different
// answer type.
func (a Answers) Score(name string) (ScoreAnswer, error) {
	return answerAs[ScoreAnswer](a, name)
}

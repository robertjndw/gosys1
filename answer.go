package sys1

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
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
	// Noul is the probability that the answer is yes, on a scale from
	// 0 (no) to 1 (yes).
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
	Score float64
	// Legend holds each level's description, lowest level first,
	// exactly as the question sent it.
	Legend []Content
	// Probabilities holds each level's probability, indexed like
	// Legend.
	Probabilities []float64
	// Confidence is how certain the model is, from 0 to 1.
	Confidence float64
}

// Type implements Answer.
func (a ScoreAnswer) Type() AnswerType { return AnswerScore }

func (a ScoreAnswer) answer() {}

// scoreAnswerWire is the wire shape of a ScoreAnswer: legend and
// probabilities keyed by level index as a string ("0", "1", ...)
// rather than held as ordered slices.
type scoreAnswerWire struct {
	Score         float64            `json:"score"`
	Legend        map[string]Content `json:"legend"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
}

// UnmarshalJSON implements json.Unmarshaler, converting the
// string-keyed wire maps into Legend and Probabilities ordered by
// level index.
func (a *ScoreAnswer) UnmarshalJSON(b []byte) error {
	var wire scoreAnswerWire
	if err := json.Unmarshal(b, &wire); err != nil {
		return err
	}
	if len(wire.Legend) != len(wire.Probabilities) {
		return fmt.Errorf("sys1: score answer: legend has %d levels, probabilities has %d", len(wire.Legend), len(wire.Probabilities))
	}
	legend, err := indexedLevels("legend", wire.Legend)
	if err != nil {
		return err
	}
	probabilities, err := indexedLevels("probabilities", wire.Probabilities)
	if err != nil {
		return err
	}

	a.Score = wire.Score
	a.Legend = legend
	a.Probabilities = probabilities
	a.Confidence = wire.Confidence
	return nil
}

// indexedLevels converts a wire map keyed by level index (as a string,
// "0", "1", ...) into a slice ordered by index. field names the map in
// error messages. It fails unless the keys are exactly the
// non-negative integers 0..n-1 with no gaps or duplicates.
func indexedLevels[T any](field string, m map[string]T) ([]T, error) {
	n := len(m)
	out := make([]T, n)
	seen := make([]bool, n)
	for k, v := range m {
		i, err := strconv.Atoi(k)
		if err != nil || i < 0 {
			return nil, fmt.Errorf("sys1: score answer: %s has invalid level key %q", field, k)
		}
		if i >= n || seen[i] {
			return nil, fmt.Errorf("sys1: score answer: %s levels are not a contiguous 0..%d range", field, n-1)
		}
		seen[i] = true
		out[i] = v
	}
	return out, nil
}

// MarshalJSON implements json.Marshaler, emitting the "type"
// discriminator and the string-keyed legend and probabilities shape,
// so a Response round-trips.
func (a ScoreAnswer) MarshalJSON() ([]byte, error) {
	legend := make(map[string]Content, len(a.Legend))
	for i, v := range a.Legend {
		legend[strconv.Itoa(i)] = v
	}
	probabilities := make(map[string]float64, len(a.Probabilities))
	for i, v := range a.Probabilities {
		probabilities[strconv.Itoa(i)] = v
	}
	return json.Marshal(struct {
		Type AnswerType `json:"type"`
		scoreAnswerWire
	}{AnswerScore, scoreAnswerWire{a.Score, legend, probabilities, a.Confidence}})
}

// Ranked returns the options ordered from most to least probable,
// with ties broken by name so the order is deterministic.
func (a ChoiceAnswer) Ranked() []string {
	options := slices.Collect(maps.Keys(a.Probabilities))
	slices.SortFunc(options, func(x, y string) int {
		return cmp.Or(cmp.Compare(a.Probabilities[y], a.Probabilities[x]), cmp.Compare(x, y))
	})
	return options
}

// Levels reports the number of score levels in the answer's legend.
func (a ScoreAnswer) Levels() int { return len(a.Legend) }

// Probability returns the probability of the given level index. It
// returns 0 if the level is not present, e.g. because the index is out
// of range.
func (a ScoreAnswer) Probability(level int) float64 {
	if level < 0 || level >= len(a.Probabilities) {
		return 0
	}
	return a.Probabilities[level]
}

// Label returns the description of the given level index from Legend,
// or nil if the level is not present. It is the level as the question
// sent it, so a level built with [Levels] comes back as a string.
func (a ScoreAnswer) Label(level int) Content {
	if level < 0 || level >= len(a.Legend) {
		return nil
	}
	return a.Legend[level]
}

// Nearest returns the index of the whole level closest to Score, for
// callers who want a discrete level rather than the weighted position.
// The result is clamped to [0, Levels()-1] whenever the answer has any
// levels, so it is always a valid index even if Score itself lies
// outside that range. Use Label to read its description.
func (a ScoreAnswer) Nearest() int {
	nearest := int(math.Round(a.Score))
	if n := a.Levels(); n > 0 {
		nearest = min(max(nearest, 0), n-1)
	}
	return nearest
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

// answersOf copies the entries of a whose answer is a T into a fresh
// map, so the typed views below never alias a and never return nil.
func answersOf[T Answer](a Answers) map[string]T {
	out := make(map[string]T)
	for name, ans := range a {
		if v, ok := ans.(T); ok {
			out[name] = v
		}
	}
	return out
}

// NoulAnswers returns every NoulAnswer in a, keyed by question name.
// It suits a battery built at runtime, such as one Noul per label,
// where ranging over all of them beats a Noul lookup per name.
func (a Answers) NoulAnswers() map[string]NoulAnswer {
	return answersOf[NoulAnswer](a)
}

// ChoiceAnswers returns every ChoiceAnswer in a, keyed by question
// name. See NoulAnswers.
func (a Answers) ChoiceAnswers() map[string]ChoiceAnswer {
	return answersOf[ChoiceAnswer](a)
}

// ScoreAnswers returns every ScoreAnswer in a, keyed by question name.
// See NoulAnswers.
func (a Answers) ScoreAnswers() map[string]ScoreAnswer {
	return answersOf[ScoreAnswer](a)
}

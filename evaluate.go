package sys1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
)

// Response is the result of a successful Client.Evaluate call.
type Response struct {
	// Model is the model that performed the evaluation. It may differ
	// from the alias used in the request.
	Model string `json:"model"`
	// Answers holds one answer per question, keyed by the name each
	// question was sent under.
	Answers Answers `json:"answers"`
	// Usage reports token usage for the request.
	Usage Usage `json:"usage"`
	// RequestID is the value of the x-typesafe-request-id response
	// header, useful when reporting an issue to TypeSafe.
	RequestID string `json:"-"`
}

// Usage reports token usage for a single Evaluate call.
type Usage struct {
	// InputTokens is the number of billable input tokens used.
	InputTokens int `json:"input_tokens"`
	// OutputTokens is the number of output tokens used.
	OutputTokens int `json:"output_tokens"`
}

// Evaluate asks any number of named questions about state in a single
// round trip. Each question is built with the matching constructor
// ([Noul], [Choice], [Score], or [Raw] for forward compatibility);
// answers come back under each question's name, via resp.Answers's
// typed accessors.
//
// Evaluate validates state, the questions and the client's model
// before making any network call, returning an error wrapping
// ErrInvalidRequest for a problem it can catch locally: a nil state,
// zero questions, a nil question, a blank or duplicate question name,
// a question whose own limits are violated (see each question type),
// an empty model, or a WithExtraBody field that collides with a
// built-in request field.
func (c *Client) Evaluate(ctx context.Context, state Content, questions ...Question) (*Response, error) {
	qmap, err := collectQuestions(questions)
	if err != nil {
		return nil, err
	}

	if err := validateEvaluate(state, qmap, c.model); err != nil {
		return nil, err
	}

	for _, key := range [...]string{"state", "model", "questions"} {
		if _, collides := c.extraBody[key]; collides {
			return nil, fmt.Errorf("%w: extra body field %q collides with a built-in request field", ErrInvalidRequest, key)
		}
	}

	// The wire body of POST /v1/systemone. Questions are keyed by name,
	// which is how the API wants them and how the answers come back.
	// Extra fields (WithExtraBody) are shallow-merged on top; the check
	// above rules out a collision with a built-in key.
	payload := map[string]any{"state": state, "model": c.model, "questions": qmap}
	maps.Copy(payload, c.extraBody)
	body, err := encodeJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("sys1: encoding request body: %w", err)
	}

	var result Response
	header, err := c.do(ctx, http.MethodPost, "/v1/systemone", body, &result)
	if err != nil {
		return nil, err
	}
	result.RequestID = header.Get(requestIDHeader)
	return &result, nil
}

// collectQuestions keys questions by name, catching a nil question or
// a blank or duplicate name here rather than in validateEvaluate,
// because once the questions sit in a map a duplicate is no longer
// visible.
func collectQuestions(questions []Question) (map[string]Question, error) {
	out := make(map[string]Question, len(questions))
	for _, q := range questions {
		if q == nil {
			return nil, fmt.Errorf("%w: question must not be nil", ErrInvalidRequest)
		}
		name := q.name()
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("%w: question name must not be empty", ErrInvalidRequest)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("%w: duplicate question name %q", ErrInvalidRequest, name)
		}
		out[name] = q
	}
	return out, nil
}

// validateEvaluate checks Evaluate's inputs before any network call.
func validateEvaluate(state Content, questions map[string]Question, model string) error {
	if state == nil {
		return fmt.Errorf("%w: state must not be nil", ErrInvalidRequest)
	}
	if len(questions) == 0 {
		return fmt.Errorf("%w: at least one question is required", ErrInvalidRequest)
	}
	for name, q := range questions {
		if err := q.validate(); err != nil {
			return fmt.Errorf("sys1: question %q: %w", name, err)
		}
	}
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("%w: model must not be empty", ErrInvalidRequest)
	}
	return nil
}

// encodeJSON marshals v with HTML escaping disabled, so state
// containing '<', '>' or '&' reaches the model unmangled.
func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// shortcutQuestionKey is the fixed question key used by the
// single-question shortcut methods (Client.Noul, Client.Choice,
// Client.Score).
const shortcutQuestionKey = "q"

// Noul asks a single yes/no question and returns the NoulAnswer
// directly, skipping the Answers map. Any error from the underlying
// Evaluate call, including an *APIError, passes through unchanged.
//
// Noul does not expose criteria (what counts as yes or no); use
// Evaluate with sys1.Noul(name, instructions).WithCriteria(yes, no)
// for that.
func (c *Client) Noul(ctx context.Context, state, instructions Content) (NoulAnswer, error) {
	resp, err := c.Evaluate(ctx, state, Noul(shortcutQuestionKey, instructions))
	if err != nil {
		return NoulAnswer{}, err
	}
	return resp.Answers.Noul(shortcutQuestionKey)
}

// Choice asks a single choice question and returns the ChoiceAnswer
// directly, skipping the Answers map. Any error from the underlying
// Evaluate call, including an *APIError, passes through unchanged.
func (c *Client) Choice(ctx context.Context, state, instructions Content, choices Choices) (ChoiceAnswer, error) {
	resp, err := c.Evaluate(ctx, state, Choice(shortcutQuestionKey, instructions, choices))
	if err != nil {
		return ChoiceAnswer{}, err
	}
	return resp.Answers.Choice(shortcutQuestionKey)
}

// Score asks a single score question over the given ordered levels
// (see [Levels]) and returns the ScoreAnswer directly, skipping the
// Answers map. Any error from the underlying Evaluate call, including
// an *APIError, passes through unchanged.
func (c *Client) Score(ctx context.Context, state, instructions Content, levels []Content) (ScoreAnswer, error) {
	resp, err := c.Evaluate(ctx, state, Score(shortcutQuestionKey, instructions, levels))
	if err != nil {
		return ScoreAnswer{}, err
	}
	return resp.Answers.Score(shortcutQuestionKey)
}

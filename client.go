package sys1

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxResponseBodyBytes bounds how much of a response body is read into
// memory, guarding against a misbehaving or malicious server.
const maxResponseBodyBytes = 16 * 1024 * 1024

// shortcutQuestionKey is the fixed question key used by the
// single-question shortcut methods (Client.Noul, Client.Choice,
// Client.Score).
const shortcutQuestionKey = "q"

// requestIDHeader is the response header carrying TypeSafe's request
// id, surfaced on Response.RequestID and APIError.RequestID.
const requestIDHeader = "x-typesafe-request-id"

// Client is a TypeSafe SystemOne API client. Construct one with New;
// a Client is safe for concurrent use.
type Client struct {
	apiKey          string
	baseURL         *url.URL
	model           string
	httpClient      *http.Client
	timeout         time.Duration
	retry           RetryPolicy
	headers         http.Header
	userAgentSuffix string
	logger          *slog.Logger

	// sleep backs the retry backoff. Tests inject an instant
	// implementation so retry behavior can be verified without waiting.
	sleep sleepFunc
}

// parseBaseURL trims trailing slashes and validates that raw is an
// absolute URL.
func parseBaseURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return nil, fmt.Errorf("sys1: base URL must not be empty")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("sys1: invalid base URL %q: %w", raw, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("sys1: invalid base URL %q: must be absolute", raw)
	}
	return u, nil
}

// Evaluate asks any number of named questions about state in a single
// round trip. Each argument is either a Question (sys1.Noul,
// sys1.Choice, sys1.Score, sys1.Raw) or a RequestOption
// (WithRequestModel, WithRequestHeader, ...), in any order. Answers
// come back under each question's name, via resp.Answers's typed
// accessors.
//
// Evaluate validates state, the questions and the effective model
// before making any network call, returning an error wrapping
// ErrInvalidRequest for a problem it can catch locally, including a
// blank or duplicate question name.
func (c *Client) Evaluate(ctx context.Context, state Content, args ...EvaluateArg) (*Response, error) {
	questions, rc, err := collectEvaluateArgs(args)
	if err != nil {
		return nil, err
	}

	model := cmp.Or(rc.model, c.model)
	if err := validateEvaluate(state, questions, model); err != nil {
		return nil, err
	}

	// The wire body of POST /v1/systemone. Questions are keyed by name,
	// which is how the API wants them and how the answers come back.
	// Extra fields (WithRequestExtraBody) are shallow-merged on top, so
	// one that collides with a built-in key replaces it.
	payload := map[string]any{"state": state, "model": model, "questions": questions}
	maps.Copy(payload, rc.extraBody)
	body, err := encodeJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("sys1: encoding request body: %w", err)
	}

	var result Response
	header, err := c.do(ctx, http.MethodPost, "/v1/systemone", body, rc, &result)
	if err != nil {
		return nil, err
	}
	result.RequestID = header.Get(requestIDHeader)
	return &result, nil
}

// collectEvaluateArgs splits Evaluate's variadic list into the
// questions keyed by name and the per-call overrides. Name problems
// are caught here rather than in validateEvaluate because once the
// questions sit in a map a duplicate is no longer visible.
func collectEvaluateArgs(args []EvaluateArg) (map[string]Question, *requestConfig, error) {
	rc := &requestConfig{}
	questions := make(map[string]Question)
	for _, arg := range args {
		switch v := arg.(type) {
		case Question:
			name := v.Name()
			if strings.TrimSpace(name) == "" {
				return nil, nil, fmt.Errorf("%w: question name must not be empty", ErrInvalidRequest)
			}
			if _, dup := questions[name]; dup {
				return nil, nil, fmt.Errorf("%w: duplicate question name %q", ErrInvalidRequest, name)
			}
			questions[name] = v
		case RequestOption:
			rc.apply(v)
		case nil:
			return nil, nil, fmt.Errorf("%w: argument must not be nil", ErrInvalidRequest)
		}
	}
	return questions, rc, nil
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

// shortcutArgs builds Evaluate's argument list for a single-question
// shortcut: the question itself plus the caller's per-call options.
func shortcutArgs(q Question, opts []RequestOption) []EvaluateArg {
	args := make([]EvaluateArg, 0, len(opts)+1)
	args = append(args, q)
	for _, opt := range opts {
		args = append(args, opt)
	}
	return args
}

// Noul asks a single yes/no question and returns the probability of
// yes directly, skipping the Answers map. Any error from the
// underlying Evaluate call, including an *APIError, passes through
// unchanged.
//
// Noul does not expose criteria (what counts as yes or no); use
// Evaluate with sys1.Noul(name, instructions).WithCriteria(yes, no)
// for that.
func (c *Client) Noul(ctx context.Context, state, instructions Content, opts ...RequestOption) (float64, error) {
	resp, err := c.Evaluate(ctx, state, shortcutArgs(Noul(shortcutQuestionKey, instructions), opts)...)
	if err != nil {
		return 0, err
	}
	ans, err := resp.Answers.Noul(shortcutQuestionKey)
	if err != nil {
		return 0, err
	}
	return ans.Noul, nil
}

// Choice asks a single choice question and returns the ChoiceAnswer
// directly, skipping the Answers map. Any error from the underlying
// Evaluate call, including an *APIError, passes through unchanged.
func (c *Client) Choice(ctx context.Context, state, instructions Content, choices Choices, opts ...RequestOption) (ChoiceAnswer, error) {
	resp, err := c.Evaluate(ctx, state, shortcutArgs(Choice(shortcutQuestionKey, instructions, choices), opts)...)
	if err != nil {
		return ChoiceAnswer{}, err
	}
	return resp.Answers.Choice(shortcutQuestionKey)
}

// Score asks a single score question and returns the ScoreAnswer
// directly, skipping the Answers map. Any error from the underlying
// Evaluate call, including an *APIError, passes through unchanged.
//
// Score takes levels variadically, so it has no RequestOption
// parameter: a per-call model or header after a variadic list would be
// ambiguous. Callers who need either use Evaluate with sys1.Score(...)
// instead.
func (c *Client) Score(ctx context.Context, state, instructions Content, levels ...Content) (ScoreAnswer, error) {
	resp, err := c.Evaluate(ctx, state, Score(shortcutQuestionKey, instructions, levels...))
	if err != nil {
		return ScoreAnswer{}, err
	}
	return resp.Answers.Score(shortcutQuestionKey)
}

// Models lists the models and aliases available to the authenticated
// account. A returned Model's Name is valid as a request model or as
// the argument to WithModel / WithRequestModel.
func (c *Client) Models(ctx context.Context, opts ...RequestOption) ([]Model, error) {
	var result modelsResponse
	if _, err := c.do(ctx, http.MethodGet, "/v1/models", nil, newRequestConfig(opts), &result); err != nil {
		return nil, err
	}
	return result.Models, nil
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

// attemptResult is the outcome of a single HTTP attempt inside do's
// retry loop.
type attemptResult struct {
	body   []byte
	header http.Header
	status int
	// err is a transport-level failure: building the request, the
	// round trip itself, or reading the body. It is nil whenever an
	// HTTP response was received and its body read, even for a
	// non-2xx status.
	err error
}

// doAttempt performs one HTTP attempt, applying the per-attempt
// timeout and this client's headers.
func (c *Client) doAttempt(ctx context.Context, method, reqURL string, body []byte, extraHeaders http.Header) attemptResult {
	attemptCtx := ctx
	if c.timeout > 0 {
		var cancel context.CancelFunc
		attemptCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(attemptCtx, method, reqURL, reader)
	if err != nil {
		return attemptResult{err: fmt.Errorf("sys1: building request: %w", err)}
	}
	c.setHeaders(req, extraHeaders, body != nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return attemptResult{err: fmt.Errorf("sys1: request failed: %w", err)}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return attemptResult{header: resp.Header, err: fmt.Errorf("sys1: reading response body: %w", err)}
	}

	return attemptResult{body: data, header: resp.Header, status: resp.StatusCode}
}

// setHeaders applies sys1's own headers, then the client's default
// headers (WithHeader), then this call's headers (WithRequestHeader),
// each layer overriding the last on a key collision.
func (c *Client) setHeaders(req *http.Request, extra http.Header, hasBody bool) {
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", c.userAgent())

	for _, layer := range []http.Header{c.headers, extra} {
		for k, vs := range layer {
			for _, v := range vs {
				req.Header.Set(k, v)
			}
		}
	}
}

// userAgent renders the User-Agent header value.
func (c *Client) userAgent() string {
	if c.userAgentSuffix == "" {
		return userAgent
	}
	return userAgent + " (" + c.userAgentSuffix + ")"
}

// do performs an HTTP request with retries per the effective policy and
// decodes a 2xx response body into out. body is nil for requests
// without one (GET). It returns the response headers, or an error: the
// caller's ctx error if the caller canceled, an *APIError for a non-2xx
// response, ErrInvalidResponse if the body does not decode, or a
// transport error, each wrapped with ErrRetriesExhausted if at least
// one retry was attempted.
func (c *Client) do(ctx context.Context, method, path string, body []byte, rc *requestConfig, out any) (http.Header, error) {
	retry, err := rc.effectiveRetry(c.retry)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	reqURL := c.baseURL.JoinPath(path)
	reqURLStr := reqURL.String()
	attempts := retry.MaxRetries + 1

	var last attemptResult
	attempt := 0
	for ; attempt < attempts; attempt++ {
		start := time.Now()
		last = c.doAttempt(ctx, method, reqURLStr, body, rc.headers)
		c.logAttempt(method, path, last.status, attempt, time.Since(start), last.header.Get(requestIDHeader))

		var retryable bool
		var reason string
		switch {
		case last.err != nil:
			// A canceled or expired caller context is returned as-is
			// and never retried; a per-attempt timeout is a different
			// context and falls through to the retry logic below.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			retryable, reason = retry.RetryConnErrors, last.err.Error()
		case last.status >= 200 && last.status < 300:
			if err := json.Unmarshal(last.body, out); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrInvalidResponse, err)
			}
			return last.header, nil
		default:
			retryable, reason = retry.shouldRetryStatus(last.status), "status "+strconv.Itoa(last.status)
		}
		if !retryable || attempt == attempts-1 {
			break
		}
		if err := c.wait(ctx, retry, last.header, attempt, reason); err != nil {
			return nil, err
		}
	}

	// The APIError is built only for the attempt that ends the loop, so
	// a retried failure whose retry succeeds never pays for parsing its
	// body.
	err = last.err
	if err == nil {
		err = c.buildAPIError(method, reqURL, last.status, last.header, last.body)
	}
	if attempt > 0 {
		return nil, fmt.Errorf("%w: %w", ErrRetriesExhausted, err)
	}
	return nil, err
}

// wait sleeps for the retry delay before the given attempt.
func (c *Client) wait(ctx context.Context, retry RetryPolicy, header http.Header, attempt int, reason string) error {
	delay := retryDelay(retry, header, attempt)
	c.logRetry(attempt+1, retry.MaxRetries, delay, reason)
	return c.sleep(ctx, delay)
}

// logRetry logs a scheduled retry at info level when a logger is
// configured. Retries are rarer and more actionable than individual
// attempts, which is why they sit one level above logAttempt's debug
// lines.
func (c *Client) logRetry(retryNum, maxRetries int, delay time.Duration, reason string) {
	if c.logger == nil {
		return
	}
	c.logger.Info("sys1: retrying request",
		"retry", retryNum,
		"max_retries", maxRetries,
		"delay", delay,
		"reason", reason,
	)
}

// logAttempt logs one HTTP attempt at debug level when a logger is
// configured. It never logs headers or bodies.
func (c *Client) logAttempt(method, path string, status, attempt int, duration time.Duration, requestID string) {
	if c.logger == nil {
		return
	}
	c.logger.Debug("sys1: request",
		"method", method,
		"path", path,
		"status", status,
		"attempt", attempt,
		"duration", duration,
		"request_id", requestID,
	)
}

// buildAPIError turns a non-2xx response into an *APIError, parsing
// 422 validation details and best-effort human messages.
func (c *Client) buildAPIError(method string, reqURL *url.URL, status int, header http.Header, body []byte) *APIError {
	// The recorded URL drops the query string and fragment so an
	// APIError never leaks a query parameter.
	safeURL := *reqURL
	safeURL.RawQuery, safeURL.Fragment = "", ""

	apiErr := &APIError{
		StatusCode: status,
		Status:     statusText(status),
		Method:     method,
		URL:        safeURL.String(),
		RequestID:  header.Get(requestIDHeader),
		Body:       body,
		Header:     header,
	}
	apiErr.RetryAfter, _ = retryAfter(header)

	if status == http.StatusUnprocessableEntity {
		var verr struct {
			Detail []ValidationError `json:"detail"`
		}
		if err := json.Unmarshal(body, &verr); err == nil && len(verr.Detail) > 0 {
			apiErr.Details = verr.Detail
			apiErr.Message = verr.Detail[0].Msg
			if p := verr.Detail[0].Path(); p != "" {
				apiErr.Message = p + ": " + apiErr.Message
			}
		}
	}

	if apiErr.Message == "" {
		var generic struct {
			Message string `json:"message"`
			Error   string `json:"error"`
			Detail  string `json:"detail"`
		}
		if err := json.Unmarshal(body, &generic); err == nil {
			apiErr.Message = cmp.Or(generic.Message, generic.Error, generic.Detail)
		}
	}

	if apiErr.Message == "" {
		apiErr.Message = apiErr.Status
	}

	return apiErr
}

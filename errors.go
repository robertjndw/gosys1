package sys1

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors for use with errors.Is. APIError.Is maps HTTP status
// codes onto the relevant sentinel so callers never need to compare
// status codes by hand.
var (
	// ErrMissingAPIKey is returned by New when no API key was
	// supplied via WithAPIKey or the TYPESAFE_API_KEY environment
	// variable.
	ErrMissingAPIKey = errors.New("sys1: missing API key")

	// ErrInvalidRequest is wrapped by client-side validation failures,
	// caught before any network call is made.
	ErrInvalidRequest = errors.New("sys1: invalid request")

	// ErrUnauthorized corresponds to HTTP 401: the API key is missing
	// or invalid.
	ErrUnauthorized = errors.New("sys1: unauthorized")

	// ErrForbidden corresponds to HTTP 403.
	ErrForbidden = errors.New("sys1: forbidden")

	// ErrNotFound corresponds to HTTP 404.
	ErrNotFound = errors.New("sys1: not found")

	// ErrUnprocessable corresponds to HTTP 422: the request body failed
	// validation. See APIError.Details for the offending fields.
	ErrUnprocessable = errors.New("sys1: unprocessable entity")

	// ErrRateLimited corresponds to HTTP 429.
	ErrRateLimited = errors.New("sys1: rate limited")

	// ErrOverloaded corresponds to HTTP 529: TypeSafe is temporarily
	// overloaded.
	ErrOverloaded = errors.New("sys1: overloaded")

	// ErrServer corresponds to any 5xx status that has no dedicated
	// sentinel, so 529 matches ErrOverloaded and not ErrServer.
	ErrServer = errors.New("sys1: server error")

	// ErrNoAnswer is returned by an Answers accessor when the requested
	// key is not present in the response.
	ErrNoAnswer = errors.New("sys1: no answer for question")

	// ErrAnswerType is returned by an Answers accessor when the answer
	// for the requested key is not of the expected type.
	ErrAnswerType = errors.New("sys1: answer has a different type")

	// ErrInvalidResponse is returned when a 2xx response body cannot be
	// decoded.
	ErrInvalidResponse = errors.New("sys1: invalid response body")

	// ErrRetriesExhausted wraps the last error seen once at least one
	// retry has been attempted and none succeeded.
	ErrRetriesExhausted = errors.New("sys1: retries exhausted")
)

// APIError is returned for any HTTP response TypeSafe treats as an
// error. Use errors.Is against the sentinel errors above to branch on
// the status category, or errors.As to inspect the full detail.
type APIError struct {
	// StatusCode is the HTTP status code of the response.
	StatusCode int
	// Status is http.StatusText(StatusCode), or "Overloaded" for 529
	// which the standard library has no text for.
	Status string
	// Method is the HTTP method of the request that failed.
	Method string
	// URL is the request URL without query string or fragment; it
	// never includes credentials.
	URL string
	// RequestID is the value of the x-typesafe-request-id response
	// header, if present.
	RequestID string
	// Body is the raw response body. It may be empty.
	Body []byte
	// Message is a best-effort human-readable summary: the first
	// validation error's path and message for 422 responses, the
	// "message", "error" or "detail" string field when the body has
	// one, or the HTTP status text otherwise.
	Message string
	// Details holds the parsed validation errors for 422 responses.
	Details []ValidationError
	// RetryAfter is the delay parsed from the Retry-After or
	// retry-after-ms response header, or zero if neither was present.
	RetryAfter time.Duration
	// Header holds the full set of response headers.
	Header http.Header
}

// Error implements the error interface.
func (e *APIError) Error() string {
	var b strings.Builder
	b.WriteString("sys1: ")
	if e.Method != "" {
		b.WriteString(e.Method)
		b.WriteByte(' ')
	}
	b.WriteString(e.URL)
	b.WriteString(": ")
	b.WriteString(strconv.Itoa(e.StatusCode))
	if e.Status != "" {
		b.WriteByte(' ')
		b.WriteString(e.Status)
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.RequestID != "" {
		b.WriteString(" (request id ")
		b.WriteString(e.RequestID)
		b.WriteByte(')')
	}
	return b.String()
}

// statusOverloaded is TypeSafe's non-standard 529 status, which
// net/http knows nothing about.
const statusOverloaded = 529

// statusSentinels maps each status code with a dedicated sentinel to
// it. Any other 5xx status maps to ErrServer in APIError.Is.
var statusSentinels = map[int]error{
	http.StatusUnauthorized:        ErrUnauthorized,
	http.StatusForbidden:           ErrForbidden,
	http.StatusNotFound:            ErrNotFound,
	http.StatusUnprocessableEntity: ErrUnprocessable,
	http.StatusTooManyRequests:     ErrRateLimited,
	statusOverloaded:               ErrOverloaded,
}

// Is reports whether target is the sentinel error matching e's status
// code, so callers can write errors.Is(err, sys1.ErrRateLimited)
// instead of comparing StatusCode by hand.
func (e *APIError) Is(target error) bool {
	if sentinel, ok := statusSentinels[e.StatusCode]; ok {
		return target == sentinel
	}
	return e.StatusCode >= 500 && e.StatusCode < 600 && target == ErrServer
}

// statusText returns the HTTP status text for code, with "Overloaded"
// for 529 since net/http has no text for it.
func statusText(code int) string {
	if code == statusOverloaded {
		return "Overloaded"
	}
	if t := http.StatusText(code); t != "" {
		return t
	}
	return fmt.Sprintf("status %d", code)
}

// newAPIError turns a non-2xx response into an *APIError, parsing
// 422 validation details and best-effort human messages.
func newAPIError(method string, reqURL *url.URL, status int, header http.Header, body []byte) *APIError {
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

// ValidationError describes a single invalid field or value in a
// request, as reported by a 422 response.
type ValidationError struct {
	// Loc is the path to the invalid value: the request location
	// followed by field names and array indices, e.g.
	// ["body", "questions", "urgency", "criteria"].
	Loc []any `json:"loc"`
	// Msg is a human-readable explanation of the failure.
	Msg string `json:"msg"`
	// Type is a machine-readable validation error code.
	Type string `json:"type"`
	// Input is the value that failed validation, when reported.
	Input any `json:"input,omitempty"`
	// Ctx holds additional context explaining the failure, when
	// reported.
	Ctx map[string]any `json:"ctx,omitempty"`
}

// Path renders Loc as a dotted path, e.g. "questions.urgency.criteria",
// dropping the leading "body" element the API always includes. Loc is
// decoded from JSON as []any, so array indices arrive as float64;
// fmt.Sprint renders those without a fractional part.
func (v ValidationError) Path() string {
	parts := make([]string, 0, len(v.Loc))
	for i, l := range v.Loc {
		if i == 0 {
			if s, ok := l.(string); ok && s == "body" {
				continue
			}
		}
		parts = append(parts, fmt.Sprint(l))
	}
	return strings.Join(parts, ".")
}

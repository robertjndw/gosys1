package sys1

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestUnprocessableEntityError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnprocessableEntity, `{
			"detail": [
				{"loc": ["body", "questions", "urgency", "criteria"], "msg": "Field required", "type": "missing"}
			]
		}`, map[string]string{"x-typesafe-request-id": "req-422"})
	}).WithRetry(RetryPolicy{})
	_, err := c.Ask(context.Background(), "state", Noul("q", "q"))
	if err == nil {
		t.Fatal("Ask() error = nil, want error")
	}
	if !errors.Is(err, ErrUnprocessable) {
		t.Errorf("error = %v, want ErrUnprocessable", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if apiErr.StatusCode != 422 {
		t.Errorf("StatusCode = %d, want 422", apiErr.StatusCode)
	}
	if apiErr.RequestID != "req-422" {
		t.Errorf("RequestID = %q, want req-422", apiErr.RequestID)
	}
	if len(apiErr.Details) != 1 {
		t.Fatalf("len(Details) = %d, want 1", len(apiErr.Details))
	}
	if got := apiErr.Details[0].Path(); got != "questions.urgency.criteria" {
		t.Errorf("Details[0].Path() = %q, want questions.urgency.criteria", got)
	}
	if !strings.Contains(apiErr.Error(), "questions.urgency.criteria: Field required") {
		t.Errorf("Error() = %q, want it to contain the field path and message", apiErr.Error())
	}
	if !strings.Contains(apiErr.Error(), "req-422") {
		t.Errorf("Error() = %q, want it to contain the request id", apiErr.Error())
	}
}

func TestAPIErrorErrorFormat(t *testing.T) {
	err := &APIError{
		Method:     "POST",
		URL:        "https://api.typesafe.ai/v1/systemone",
		StatusCode: 422,
		Status:     "Unprocessable Entity",
		Message:    "questions.urgency.criteria: Field required",
		RequestID:  "abc",
	}
	got := err.Error()
	want := "sys1: POST https://api.typesafe.ai/v1/systemone: 422 Unprocessable Entity: questions.urgency.criteria: Field required (request id abc)"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAPIErrorIs(t *testing.T) {
	tests := []struct {
		status  int
		sentime error
	}{
		{400, ErrBadRequest},
		{401, ErrUnauthorized},
		{403, ErrForbidden},
		{404, ErrNotFound},
		{422, ErrUnprocessable},
		{429, ErrRateLimited},
		{529, ErrOverloaded},
		{500, ErrServer},
		{503, ErrServer},
	}
	for _, tt := range tests {
		err := &APIError{StatusCode: tt.status}
		if !errors.Is(err, tt.sentime) {
			t.Errorf("status %d: errors.Is = false, want true for %v", tt.status, tt.sentime)
		}
	}

	// A status that maps to one sentinel must not also match another.
	err := &APIError{StatusCode: 404}
	if errors.Is(err, ErrForbidden) {
		t.Error("404 matches ErrForbidden, want only ErrNotFound")
	}
}

func TestValidationErrorPath(t *testing.T) {
	tests := []struct {
		name string
		loc  []any
		want string
	}{
		{"drops leading body", []any{"body", "state"}, "state"},
		{"nested field", []any{"body", "questions", "urgency", "criteria"}, "questions.urgency.criteria"},
		{"array index", []any{"body", "questions", "urgency", "criteria", float64(2)}, "questions.urgency.criteria.2"},
		{"no body prefix", []any{"query", "limit"}, "query.limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ValidationError{Loc: tt.loc}
			if got := v.Path(); got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}

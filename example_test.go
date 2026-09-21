package sys1_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	sys1 "github.com/robertjndw/gosys1"
)

// jsonServer returns an httptest server that answers every request
// with status and body, so the examples below have deterministic
// output without a real API key.
func jsonServer(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-typesafe-request-id", "example-request-id")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
}

func ExampleNew() {
	client, err := sys1.New(sys1.WithAPIKey("sk-example"))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(client != nil)
	// Output:
	// true
}

func ExampleClient_Evaluate() {
	srv := jsonServer(http.StatusOK, `{
		"model": "jev-1.13.0",
		"answers": {
			"is_urgent": {"type": "noul", "noul": 0.95},
			"team": {"type": "choice", "choice": "billing", "probabilities": {"billing": 0.9, "technical": 0.1}, "confidence": 0.8}
		},
		"usage": {"input_tokens": 42, "output_tokens": 9}
	}`)
	defer srv.Close()

	client, err := sys1.New(sys1.WithAPIKey("sk-example"), sys1.WithBaseURL(srv.URL))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	resp, err := client.Evaluate(context.Background(), "Help! My payouts have been failing for 3 days.",
		sys1.Noul("is_urgent", "Does this convey urgency?"),
		sys1.Choice("team", "Which team should handle this?", sys1.Choices{
			"billing":   "Payments, invoicing, refunds",
			"technical": "Bugs, outages, integrations",
		}),
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	urgent, _ := resp.Answers.Noul("is_urgent")
	team, _ := resp.Answers.Choice("team")
	fmt.Printf("urgent=%.2f team=%s\n", urgent.Noul, team.Choice)
	// Output:
	// urgent=0.95 team=billing
}

func ExampleClient_Noul() {
	srv := jsonServer(http.StatusOK, `{
		"model": "jev-1.13.0",
		"answers": {"q": {"type": "noul", "noul": 0.95}},
		"usage": {"input_tokens": 10, "output_tokens": 2}
	}`)
	defer srv.Close()

	client, err := sys1.New(sys1.WithAPIKey("sk-example"), sys1.WithBaseURL(srv.URL))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	answer, err := client.Noul(context.Background(), "Help! My payouts have been failing for 3 days.", "Does this convey urgency?")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%.2f\n", answer.Noul)
	// Output:
	// 0.95
}

func ExampleClient_Choice() {
	srv := jsonServer(http.StatusOK, `{
		"model": "jev-1.13.0",
		"answers": {"q": {"type": "choice", "choice": "billing", "probabilities": {"billing": 0.88, "technical": 0.12}, "confidence": 0.81}},
		"usage": {"input_tokens": 10, "output_tokens": 2}
	}`)
	defer srv.Close()

	client, err := sys1.New(sys1.WithAPIKey("sk-example"), sys1.WithBaseURL(srv.URL))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	answer, err := client.Choice(context.Background(), "Help! My payouts have been failing for 3 days.", "Which team should handle this?", sys1.Choices{
		"billing":   "Payments, invoicing, refunds",
		"technical": "Bugs, outages, integrations",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(answer.Choice)
	// Output:
	// billing
}

func ExampleClient_Score() {
	srv := jsonServer(http.StatusOK, `{
		"model": "jev-1.13.0",
		"answers": {"q": {"type": "score", "score": 1.05, "legend": {"0": "Calm", "1": "Frustrated", "2": "Very angry"}, "probabilities": {"0": 0.0, "1": 0.95, "2": 0.05}, "confidence": 0.92}},
		"usage": {"input_tokens": 10, "output_tokens": 2}
	}`)
	defer srv.Close()

	client, err := sys1.New(sys1.WithAPIKey("sk-example"), sys1.WithBaseURL(srv.URL))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	answer, err := client.Score(context.Background(), "Help! My payouts have been failing for 3 days.", "How frustrated is the customer?", sys1.Levels("Calm", "Frustrated", "Very angry"))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%.2f\n", answer.Score)
	fmt.Println(answer.Label(answer.Nearest()))
	// Output:
	// 1.05
	// Frustrated
}

func ExampleClient_WithModel() {
	srv := jsonServer(http.StatusOK, `{
		"model": "jev-1.13.0",
		"answers": {"q": {"type": "noul", "noul": 0.95}},
		"usage": {"input_tokens": 10, "output_tokens": 2}
	}`)
	defer srv.Close()

	client, err := sys1.New(sys1.WithAPIKey("sk-example"), sys1.WithBaseURL(srv.URL))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	// WithModel returns a copy; client itself keeps its own default
	// model.
	versioned := client.WithModel("jev-1.13.0")

	answer, err := versioned.Noul(context.Background(), "Help! My payouts have been failing for 3 days.", "Does this convey urgency?")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%.2f\n", answer.Noul)
	// Output:
	// 0.95
}

func ExampleAnswers_Choice() {
	srv := jsonServer(http.StatusOK, `{
		"model": "jev-1.13.0",
		"answers": {"team": {"type": "choice", "choice": "billing", "probabilities": {"billing": 0.9, "technical": 0.1}, "confidence": 0.8}},
		"usage": {"input_tokens": 10, "output_tokens": 2}
	}`)
	defer srv.Close()

	client, err := sys1.New(sys1.WithAPIKey("sk-example"), sys1.WithBaseURL(srv.URL))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	resp, err := client.Evaluate(context.Background(), "state",
		sys1.Choice("team", "Which team should handle this?", sys1.Choices{"billing": nil, "technical": nil}),
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	team, err := resp.Answers.Choice("team")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(team.Choice)
	// Output:
	// billing
}

func ExampleAPIError() {
	srv := jsonServer(http.StatusUnprocessableEntity, `{"detail": [{"loc": ["body", "questions", "urgency", "criteria"], "msg": "Field required", "type": "missing"}]}`)
	defer srv.Close()

	client, err := sys1.New(sys1.WithAPIKey("sk-example"), sys1.WithBaseURL(srv.URL))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	client = client.WithRetry(sys1.RetryPolicy{}) // no retries, for a deterministic example

	_, err = client.Evaluate(context.Background(), "state", sys1.Noul("q", "q"))

	var apiErr *sys1.APIError
	if errors.As(err, &apiErr) {
		fmt.Println("status:", apiErr.StatusCode)
		for _, d := range apiErr.Details {
			fmt.Printf("%s: %s\n", d.Path(), d.Msg)
		}
	}
	// Output:
	// status: 422
	// questions.urgency.criteria: Field required
}

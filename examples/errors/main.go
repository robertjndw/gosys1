// Command errors demonstrates a client-side validation failure (a Score
// with a single level, rejected before any HTTP call) and how to inspect
// an error from a live call with errors.As and errors.Is. It requires
// TYPESAFE_API_KEY in the environment.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	sys1 "github.com/robertjndw/gosys1"
)

func main() {
	client, err := sys1.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: creating client:", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// A score question needs at least two levels; this one is rejected
	// locally and never reaches the network.
	_, err = client.Evaluate(ctx, "state",
		sys1.Score("urgency", "How urgent is this?", "only one level"),
	)
	if !errors.Is(err, sys1.ErrInvalidRequest) {
		fmt.Fprintln(os.Stderr, "sys1: expected a client-side validation error, got:", err)
		os.Exit(1)
	}
	fmt.Println("client-side validation caught:", err)

	// sys1.Raw skips this library's own validation, so a request
	// missing "criteria" reaches the server and comes back as a 422.
	_, err = client.Evaluate(ctx, "state",
		sys1.Raw("team", map[string]any{"type": "choice", "instructions": "Which team?"}),
	)

	// A 429 or 401 is itself an *APIError, so the sentinel checks have
	// to come before the general errors.As branch or they never match.
	var apiErr *sys1.APIError
	switch {
	case errors.Is(err, sys1.ErrRateLimited):
		fmt.Println("rate limited, retry later")
	case errors.Is(err, sys1.ErrUnauthorized):
		fmt.Println("check your API key")
	case errors.As(err, &apiErr):
		fmt.Println("status:", apiErr.StatusCode)
		fmt.Println("request id:", apiErr.RequestID)
		for _, d := range apiErr.Details {
			fmt.Printf("%s: %s\n", d.Path(), d.Msg)
		}
	case err != nil:
		fmt.Fprintln(os.Stderr, "sys1:", err)
		os.Exit(1)
	default:
		fmt.Println("no error: the server accepted the request")
	}
}

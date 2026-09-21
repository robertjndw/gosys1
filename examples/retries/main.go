// Command retries builds a client-wide RetryPolicy once via WithRetry,
// chains a one-off override onto a single call, and bounds the whole
// thing with a context.WithTimeout total budget. It requires
// TYPESAFE_API_KEY in the environment.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	sys1 "github.com/robertjndw/gosys1"
)

func main() {
	client, err := sys1.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: creating client:", err)
		os.Exit(1)
	}

	// Built once and reused for every call that should retry this way
	// instead of the package default.
	client = client.WithRetry(sys1.RetryPolicy{
		MaxRetries: 3,
		MaxBackoff: 200 * time.Millisecond,
	})

	// The context is the total budget across every attempt and retry;
	// WithRetry only bounds each individual backoff.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// A one-off override chained straight onto this call; client keeps
	// its own retry policy for every other call.
	resp, err := client.WithRetry(sys1.RetryPolicy{MaxRetries: 1, MaxBackoff: 100 * time.Millisecond}).Ask(ctx, "I was charged twice. Please help ASAP.",
		sys1.Noul("billing", "Is this about a billing issue?"),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: evaluate:", err)
		os.Exit(1)
	}

	billing, err := resp.Answers.Noul("billing")
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1:", err)
		os.Exit(1)
	}
	fmt.Printf("billing=%.2f\n", billing.Noul)
}

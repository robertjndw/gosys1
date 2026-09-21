// Command retries configures a client-wide RetryPolicy plus a per-call
// override via WithRequestRetry, bounded by a context.WithTimeout total
// budget. It requires TYPESAFE_API_KEY in the environment.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	sys1 "github.com/robertjndw/gosys1"
)

func main() {
	client, err := sys1.New(sys1.WithRetry(sys1.RetryPolicy{
		MaxRetries: 3,
		MaxBackoff: 200 * time.Millisecond,
	}))
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: creating client:", err)
		os.Exit(1)
	}

	// The context is the total budget across every attempt and retry;
	// the client's RetryPolicy only bounds each individual backoff.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Evaluate(ctx, "I was charged twice. Please help ASAP.",
		sys1.Noul("billing", "Is this about a billing issue?"),
		sys1.WithRequestRetry(sys1.RetryPolicy{MaxRetries: 1, MaxBackoff: 100 * time.Millisecond}),
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

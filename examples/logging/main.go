// Command logging enables debug-level request logging via WithLogger and
// makes one call so the log lines (method, path, status, attempt, duration,
// request id - never the Authorization header) appear on stderr. Setting
// TYPESAFE_LOG_LEVEL=debug with New gives the same result without
// code. It requires TYPESAFE_API_KEY in the environment.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	sys1 "github.com/robertjndw/gosys1"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client, err := sys1.New(sys1.WithLogger(logger))
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: creating client:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	billing, err := client.Noul(ctx, "I was charged twice. Please help ASAP.", "Is this about a billing issue?")
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: noul:", err)
		os.Exit(1)
	}

	fmt.Printf("billing=%.2f\n", billing)
}

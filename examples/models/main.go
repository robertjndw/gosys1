// Command models lists the models available to the account, then shows how
// client.WithModel derives a copy that defaults to a specific model,
// both built once and reused and chained straight onto a single call.
// It requires TYPESAFE_API_KEY in the environment.
package main

import (
	"context"
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

	models, err := client.Models(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: models:", err)
		os.Exit(1)
	}
	for _, m := range models {
		// A zero ReleaseDate means the API reported no release_date
		// for this model or alias.
		released := "unknown"
		if !m.ReleaseDate.IsZero() {
			released = m.ReleaseDate.Format("2006-01-02")
		}
		fmt.Printf("%s: %s (released %s)\n", m.Name, m.Description, released)
	}

	// Built once and reused for every call that should default to this
	// model instead of the account's.
	versioned := client.WithModel("jev-1.13.0")
	resp, err := versioned.Ask(ctx, "I was charged twice. Please help ASAP.",
		sys1.Noul("billing", "Is this about a billing issue?"),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: evaluate:", err)
		os.Exit(1)
	}
	fmt.Println("client-wide model override answered as:", resp.Model)

	// Or chain WithModel straight onto a single call; client itself is
	// untouched and keeps using its own default model afterward.
	resp, err = client.WithModel("jev-1.13.0").Ask(ctx, "I was charged twice. Please help ASAP.",
		sys1.Noul("billing", "Is this about a billing issue?"),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: evaluate:", err)
		os.Exit(1)
	}
	fmt.Println("per-call model override answered as:", resp.Model)
}

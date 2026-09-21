// Command models lists the models available to the account, then shows how
// WithModel (a client-wide default) and WithRequestModel (a per-call
// override) each select which model answers. It requires TYPESAFE_API_KEY
// in the environment.
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
		fmt.Printf("%s: %s (released %s)\n", m.Name, m.Description, m.ReleaseDate.Format("2006-01-02"))
	}

	versioned, err := sys1.New(sys1.WithModel("jev-1.13.0"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: creating versioned client:", err)
		os.Exit(1)
	}
	resp, err := versioned.Evaluate(ctx, "I was charged twice. Please help ASAP.",
		sys1.Noul("billing", "Is this about a billing issue?"),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: evaluate:", err)
		os.Exit(1)
	}
	fmt.Println("client default model answered as:", resp.Model)

	resp, err = client.Evaluate(ctx, "I was charged twice. Please help ASAP.",
		sys1.Noul("billing", "Is this about a billing issue?"),
		sys1.WithRequestModel("jev-1.13.0"),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: evaluate:", err)
		os.Exit(1)
	}
	fmt.Println("per-call model override answered as:", resp.Model)
}

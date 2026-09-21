// Command forwardcompat sends fields this library predates - a sys1.Raw
// question with an extra "weight" key and a top-level "beam_width" via
// client.WithExtraBody - and shows an unrecognized answer type decoding as
// a RawAnswer instead of failing. The extra fields are illustrative only:
// the live API may reject them with a 422. Requires TYPESAFE_API_KEY.
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

	client = client.WithExtraBody(map[string]any{"beam_width": 4})

	ctx := context.Background()

	resp, err := client.Ask(ctx, "I was charged twice. Please help ASAP.",
		sys1.Raw("billing", map[string]any{
			"type":         "noul",
			"instructions": "About billing?",
			"weight":       2,
		}),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: evaluate:", err)
		os.Exit(1)
	}

	for key, answer := range resp.Answers {
		switch a := answer.(type) {
		case sys1.RawAnswer:
			fmt.Printf("%s: unrecognized type %q, raw: %s\n", key, a.Kind, a.Data)
		case sys1.NoulAnswer:
			fmt.Printf("%s: noul=%.2f\n", key, a.Noul)
		default:
			fmt.Printf("%s: %s\n", key, answer.Type())
		}
	}
}

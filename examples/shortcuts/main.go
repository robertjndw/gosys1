// Command shortcuts asks the same three questions as examples/basic
// through the single-question shortcuts (Noul, Choice, Score), one round
// trip each. It requires TYPESAFE_API_KEY in the environment.
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
	state := "I was charged twice. Please help ASAP."

	billing, err := client.Noul(ctx, state, "Is this about a billing issue?")
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: noul:", err)
		os.Exit(1)
	}

	tone, err := client.Choice(ctx, state, "What is the tone of this message?", sys1.Choices{
		"calm":  nil,
		"angry": nil,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: choice:", err)
		os.Exit(1)
	}

	urgency, err := client.Score(ctx, state, "How urgent is this message?", "low", "medium", "high")
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: score:", err)
		os.Exit(1)
	}

	fmt.Printf("billing=%.2f tone=%s urgency=%.2f\n", billing, tone.Choice, urgency.Score)
}

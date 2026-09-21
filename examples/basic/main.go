// Command basic sends one state with three question types - noul, choice
// and score - in a single Ask call and prints the typed answers.
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
	state := "I was charged twice. Please help ASAP."

	resp, err := client.Ask(ctx, state,
		sys1.Noul("billing", "Is this about a billing issue?"),
		sys1.Choice("tone", "What is the tone of this message?", sys1.Choices("calm", "angry")),
		sys1.Score("urgency", "How urgent is this message?", sys1.Levels("low", "medium", "high")),
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
	tone, err := resp.Answers.Choice("tone")
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1:", err)
		os.Exit(1)
	}
	urgency, err := resp.Answers.Score("urgency")
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1:", err)
		os.Exit(1)
	}

	fmt.Printf("billing=%.2f tone=%s urgency=%.2f\n", billing.Noul, tone.Choice, urgency.Score)
	fmt.Println("tone, most likely first:", tone.Ranked())
	fmt.Printf("nearest urgency level: %d (%v)\n", urgency.Nearest(), urgency.Label(urgency.Nearest()))
	fmt.Println("model:", resp.Model)
	fmt.Printf("usage: input=%d output=%d\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	fmt.Println("request id:", resp.RequestID)
}

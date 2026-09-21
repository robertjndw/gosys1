// Command typed decodes an Evaluate response into a caller-owned Triage
// struct via the typed Answers accessors, so downstream code never touches
// the answers map directly. It requires TYPESAFE_API_KEY in the environment.
package main

import (
	"context"
	"fmt"
	"os"

	sys1 "github.com/robertjndw/gosys1"
)

// Triage is what this program's callers work with instead of sys1.Answers.
type Triage struct {
	Billing float64
	Tone    sys1.ChoiceAnswer
	Urgency sys1.ScoreAnswer
}

func decodeTriage(resp *sys1.Response) (Triage, error) {
	billing, err := resp.Answers.Noul("billing")
	if err != nil {
		return Triage{}, err
	}
	tone, err := resp.Answers.Choice("tone")
	if err != nil {
		return Triage{}, err
	}
	urgency, err := resp.Answers.Score("urgency")
	if err != nil {
		return Triage{}, err
	}
	return Triage{Billing: billing.Noul, Tone: tone, Urgency: urgency}, nil
}

func main() {
	client, err := sys1.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: creating client:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	resp, err := client.Evaluate(ctx, "I was charged twice. Please help ASAP.",
		sys1.Noul("billing", "Is this about a billing issue?"),
		sys1.Choice("tone", "What is the tone of this message?", sys1.Choices{
			"calm":  nil,
			"angry": nil,
		}),
		sys1.Score("urgency", "How urgent is this message?", "low", "medium", "high"),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: evaluate:", err)
		os.Exit(1)
	}

	triage, err := decodeTriage(resp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1:", err)
		os.Exit(1)
	}

	fmt.Printf("billing=%.2f tone=%s urgency=%.2f (confidence=%.2f)\n",
		triage.Billing, triage.Tone.Choice, triage.Urgency.Score, triage.Urgency.Confidence)
}

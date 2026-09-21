// Command labels builds a battery at runtime, one Noul per label, as a
// []sys1.Question spread into Ask. Several labels can apply
// to one message at once, which is why each is its own yes/no question
// rather than one Choice. It requires TYPESAFE_API_KEY in the environment.
package main

import (
	"context"
	"fmt"
	"os"

	sys1 "github.com/robertjndw/gosys1"
)

var labels = map[string]string{
	"billing":  "Is this about a charge, invoice or refund?",
	"bug":      "Does this report something not working as intended?",
	"churn":    "Does the customer threaten to leave or cancel?",
	"praise":   "Does the customer express satisfaction?",
	"question": "Is the customer asking how to do something?",
}

func main() {
	client, err := sys1.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: creating client:", err)
		os.Exit(1)
	}

	var questions []sys1.Question
	for name, instructions := range labels {
		questions = append(questions, sys1.Noul(name, instructions))
	}

	ctx := context.Background()
	resp, err := client.Ask(ctx, "I was charged twice. Fix this or I'm cancelling.", questions...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: evaluate:", err)
		os.Exit(1)
	}

	// The threshold lives here, in code, so it can change without
	// re-running inference.
	const threshold = 0.5
	for name := range labels {
		answer, err := resp.Answers.Noul(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sys1:", err)
			os.Exit(1)
		}
		if answer.Noul >= threshold {
			fmt.Printf("%-9s %.2f\n", name, answer.Noul)
		}
	}
}

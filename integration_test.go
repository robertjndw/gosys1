//go:build integration

package sys1_test

import (
	"context"
	"os"
	"testing"
	"time"

	sys1 "github.com/robertjndw/gosys1"
)

// TestIntegration exercises a real TypeSafe account: one Evaluate call
// covering all three question types, plus one Models call. It is
// skipped unless TYPESAFE_API_KEY is set, so `go test ./...` never
// needs network access or a live key.
//
// Run explicitly with:
//
//	TYPESAFE_API_KEY=... go test -tags integration -run Integration ./...
func TestIntegration(t *testing.T) {
	if os.Getenv("TYPESAFE_API_KEY") == "" {
		t.Skip("TYPESAFE_API_KEY not set; skipping integration test")
	}

	client, err := sys1.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	state := "Help! My payouts have been failing for 3 days."

	resp, err := client.Evaluate(ctx, state,
		sys1.Noul("is_urgent", "Does this convey urgency?"),
		sys1.Choice("team", "Which team should handle this?", sys1.Choices{
			"billing":   "Payments, invoicing, refunds",
			"technical": "Bugs, outages, integrations",
			"sales":     "Pricing, upgrades, new accounts",
		}),
		sys1.Score("frustration", "How frustrated is the customer?", "Calm", "Frustrated", "Very angry"),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if resp.Model == "" {
		t.Error("resp.Model is empty")
	}
	if resp.RequestID == "" {
		t.Error("resp.RequestID is empty")
	}
	if resp.Usage.InputTokens == 0 {
		t.Error("resp.Usage.InputTokens is 0")
	}

	if _, err := resp.Answers.Noul("is_urgent"); err != nil {
		t.Errorf("Answers.Noul(is_urgent): %v", err)
	}
	if _, err := resp.Answers.Choice("team"); err != nil {
		t.Errorf("Answers.Choice(team): %v", err)
	}
	if _, err := resp.Answers.Score("frustration"); err != nil {
		t.Errorf("Answers.Score(frustration): %v", err)
	}

	models, err := client.Models(ctx)
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) == 0 {
		t.Error("Models returned no models")
	}
}

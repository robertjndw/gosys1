# gosys1

Go client for TypeSafe's SystemOne API (https://docs.typesafe.ai/api), standard library only.

## Install

```sh
go get github.com/robertjndw/gosys1
```

Requires Go 1.24 or later.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	sys1 "github.com/robertjndw/gosys1"
)

func main() {
	client, err := sys1.New()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	state := "I was charged twice. Please help ASAP."

	resp, err := client.Evaluate(ctx, state,
		sys1.Noul("billing", "Is this about a billing issue?"),
		sys1.Choice("tone", "What is the tone of this message?", sys1.Names("calm", "angry")),
		sys1.Score("urgency", "How urgent is this message?", sys1.Levels("low", "medium", "high")),
	)
	if err != nil {
		log.Fatal(err)
	}

	billing, _ := resp.Answers.Noul("billing")
	tone, _ := resp.Answers.Choice("tone")
	urgency, _ := resp.Answers.Score("urgency")

	fmt.Printf("billing: %.2f\n", billing.Noul)
	fmt.Printf("tone: %s (confidence %.2f)\n", tone.Choice, tone.Confidence)
	fmt.Printf("urgency: %.2f, nearest level %v\n", urgency.Score, urgency.Label(urgency.Nearest()))
}
```

`New()` reads `TYPESAFE_API_KEY` and the other `TYPESAFE_*` variables from the environment, so no options are needed in the common case.

`Evaluate` takes one state and as many questions as you like. Each question is built with `sys1.Noul`, `sys1.Choice` or `sys1.Score` and carries the name its answer comes back under, which is also how the API keys them on the wire. A blank or duplicate name is rejected locally with `ErrInvalidRequest`.

## The three questions

| Question | Answer | Use it for |
|---|---|---|
| `Noul` | a `NoulAnswer` whose `Noul` field is the probability that the answer is yes, from 0 to 1 | whether a condition holds; one per label when several can apply at once |
| `Choice` | one option plus the full distribution | picking from a set you define |
| `Score` | a probability-weighted position across ordered levels | a degree along a described dimension |

`sys1.Names("calm", "angry")` builds the choices for a `Choice` whose option names speak for themselves; use a `sys1.Choices{"billing": "Payments, invoicing, refunds", ...}` literal when they need descriptions. `sys1.Levels("low", "medium", "high")` builds a `Score`'s ordered levels from plain strings and accepts an existing `[]string` via `levels...`. `sys1.Noul(name, q).WithCriteria(yes, no)` sharpens a yes/no question by describing what each side means.

A battery built at runtime, such as one `Noul` per label, is a plain `[]sys1.Question` spread with `qs...`:

```go
var qs []sys1.Question
for _, label := range labels {
	qs = append(qs, sys1.Noul(label, "Is this about "+label+"?"))
}
resp, err := client.Evaluate(ctx, state, qs...)
```

For a single question, skip `Evaluate` and the `Answers` map with the shortcuts:

```go
billingAnswer, err := client.Noul(ctx, state, "Is this about a billing issue?")

toneAnswer, err := client.Choice(ctx, state, "What is the tone of this message?", sys1.Names("calm", "angry"))

urgencyAnswer, err := client.Score(ctx, state, "How urgent is this message?", sys1.Levels("low", "medium", "high"))
```

`Noul`, `Choice` and `Score` all return their answer struct directly, skipping the `Answers` map. `resp.Model`, `resp.Usage` and `resp.RequestID` are only available through `Evaluate`.

## Reading answers

```go
p, err := resp.Answers.Noul("billing")     // NoulAnswer
c, err := resp.Answers.Choice("tone")      // Choice, Probabilities, Confidence
s, err := resp.Answers.Score("urgency")    // Score, Legend, Probabilities, Confidence

fmt.Println(p.Noul)                    // probability of yes, 0 to 1
ranked := c.Ranked()                   // options, most probable first
level := s.Nearest()                   // closest whole level, e.g. 2
label := s.Label(level)                // its description, e.g. "high"
prob := s.Probability(level)           // its probability
```

`ScoreAnswer.Legend` and `ScoreAnswer.Probabilities` are slices ordered by level, lowest first, so `s.Legend[level]` and `s.Probabilities[level]` line up with each other and with `s.Nearest()`.

A missing name returns an error wrapping `ErrNoAnswer`, and reading an answer as the wrong type returns one wrapping `ErrAnswerType`; neither panics. Ranging over `resp.Answers` directly gives the concrete `NoulAnswer`, `ChoiceAnswer`, `ScoreAnswer` or `RawAnswer` values for a type switch.

Keep the thresholds that act on these values in your own code. The model reports what it found; your policy decides what to do about it, and can change without re-running inference.

## Configuration

`sys1.New(opts...)` reads the `TYPESAFE_*` environment variables shared with TypeSafe's Python and JavaScript SDKs, then applies any options on top.

Options:

- `WithAPIKey(key string)`
- `WithBaseURL(raw string)`
- `WithHTTPClient(hc *http.Client)`
- `WithUserAgent(ua string)`
- `WithLogger(l *slog.Logger)`

Derived clients cover everything else - model, per-attempt timeout, retry policy, headers and extra body fields. `WithModel(name string)`, `WithTimeout(d time.Duration)`, `WithRetry(p RetryPolicy)`, `WithHeader(key, value string)` and `WithExtraBody(fields map[string]any)` are methods on `*Client` that return a modified copy and leave the receiver unchanged, the same shape as `context.WithTimeout` or `slog.Logger.With`. Chain a one-off override straight onto a call:

```go
resp, err := client.WithModel("jev-1.13.0").Evaluate(ctx, state,
	sys1.Noul("billing", "Is this about a billing issue?"),
)
```

or build one once and reuse it:

```go
fast := client.WithModel("jev-1.13.0").WithRetry(sys1.RetryPolicy{})
```

Each derived client is as safe for concurrent use as its parent.

Environment variables read by `New`:

| Env var | Purpose | Default |
|---|---|---|
| `TYPESAFE_API_KEY` | API key sent as a bearer token | none, required |
| `TYPESAFE_BASE_URL` | API base URL | `https://api.typesafe.ai` |
| `TYPESAFE_DEFAULT_MODEL` | default model for `Evaluate` and the shortcuts | `jev-latest` |
| `TYPESAFE_LOG_LEVEL` | `debug`, `info`, `warning`, `error` or `off`; logs to stderr at that level | unset, no logging |

Precedence is explicit option, then environment variable, then default. A blank or whitespace-only environment variable is treated as unset. `debug` logs every attempt; `info` logs each scheduled retry. `TYPESAFE_DEFAULT_MODEL` sets the model at `New` time; to change it in code instead, derive with `client.WithModel(...)`.

## Retries

`Evaluate` and `Models` retry retryable failures under a `RetryPolicy`. `DefaultRetryPolicy()` is used unless overridden:

| Field | Default |
|---|---|
| `MaxRetries` | 2 |
| `InitialBackoff` | 500ms |
| `MaxBackoff` | 5s |
| `Jitter` | 0.25 |
| `RespectRetryAfter` | true |
| `MaxRetryAfter` | 60s |
| `RetryConnErrors` | true |

Retried: 408, 429, any 5xx status and connection errors. A caller's context cancellation stops retries immediately, and the caller's context is the total time budget across every attempt and backoff, not just `WithTimeout`, which bounds a single HTTP round trip.

Override client-wide:

```go
client = client.WithRetry(sys1.RetryPolicy{MaxRetries: 5})
```

Override per use:

```go
resp, err := client.WithRetry(sys1.RetryPolicy{MaxRetries: 0}).Evaluate(ctx, state,
	sys1.Noul("billing", "Is this about a billing issue?"),
)
```

## Errors

```go
var apiErr *sys1.APIError
resp, err := client.Evaluate(ctx, state, sys1.Noul("billing", "Is this about a billing issue?"))
switch {
case errors.Is(err, sys1.ErrRateLimited):
	// back off and retry later
case errors.As(err, &apiErr):
	fmt.Println(apiErr.StatusCode, apiErr.RequestID)
	for _, d := range apiErr.Details {
		fmt.Println(d.Path(), d.Msg)
	}
}
```

Sentinel errors for `errors.Is`: `ErrMissingAPIKey`, `ErrInvalidRequest`, `ErrUnauthorized`, `ErrForbidden`, `ErrNotFound`, `ErrUnprocessable`, `ErrRateLimited`, `ErrOverloaded`, `ErrServer`, `ErrNoAnswer`, `ErrAnswerType`, `ErrInvalidResponse` and `ErrRetriesExhausted`. `APIError.Is` maps a response's status code onto the matching sentinel, so callers never compare status codes by hand.

A 422 response fills `APIError.Details` with `[]ValidationError`, one per invalid field. Each has `Msg`, `Type` and a `Path()` method that renders `Loc` as a dotted path, such as `questions.urgency.criteria`.

## Forward compatibility

`sys1.Raw(name, fields)` builds a `RawQuestion` whose `Fields` map marshals verbatim, for question fields this library predates. `client.WithExtraBody(fields)` derives a client that adds extra top-level fields to a request body. Fields must not be named `state`, `model` or `questions`; `Evaluate` rejects a field with one of those names with an error wrapping `ErrInvalidRequest`, before any network call, since it would collide with a built-in field. Any answer with a `"type"` this library does not recognize decodes as a `RawAnswer` holding the raw JSON, instead of failing.

## Examples

| Directory | Shows |
|---|---|
| `examples/basic` | One `Evaluate` call with the guide's billing example: `Noul` billing, `Choice` tone, `Score` urgency, read with the typed accessors |
| `examples/shortcuts` | The same three questions asked through `client.Noul`, `client.Choice` and `client.Score` |
| `examples/labels` | A battery built at runtime, one `Noul` per label, sent as a `[]sys1.Question` spread with `questions...` |
| `examples/typed` | Decoding answers into a caller-owned struct instead of touching the `Answers` map directly |
| `examples/models` | `client.Models`, plus `client.WithModel` to pick a specific version, both built once and reused and chained onto a single call |
| `examples/retries` | A client-wide `RetryPolicy` built once with `WithRetry`, a one-off chained override, and a `context.WithTimeout` as the total budget |
| `examples/errors` | `errors.As` and `errors.Is`, reading `ValidationError.Path()` for a 422, and a client-side `ErrInvalidRequest` |
| `examples/logging` | `WithLogger` and the per-request debug lines it produces |
| `examples/config` | `New()` picking up environment variables, then `client.WithModel` and `client.WithTimeout` deriving from it |
| `examples/forwardcompat` | `sys1.Raw`, `client.WithExtraBody` and handling `RawAnswer` in a type switch |

Each runs the same way:

```sh
TYPESAFE_API_KEY=... go run ./examples/basic
```

## Testing your code

`Client` is a concrete type with no interface to mock. Point it at an `httptest.Server` with `WithBaseURL` and assert on the request body the fake receives, or supply an `*http.Client` with a custom `RoundTripper` through `WithHTTPClient`:

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"model":"jev-1.13.0","answers":{"billing":{"type":"noul","noul":0.95}},"usage":{"input_tokens":1,"output_tokens":1}}`)
}))
defer srv.Close()

client, err := sys1.New(sys1.WithAPIKey("test"), sys1.WithBaseURL(srv.URL))
client = client.WithRetry(sys1.RetryPolicy{})
```

## Development

```sh
gofmt -l .
go vet ./...
staticcheck ./...
go test -race ./...
```

The integration test is opt-in and makes real API calls:

```sh
TYPESAFE_API_KEY=... go test -tags integration -run Integration ./...
```

## License

MIT, see [LICENSE](LICENSE).

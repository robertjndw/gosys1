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

	"github.com/robertjndw/gosys1"
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
		sys1.Choice("tone", "What is the tone of this message?", sys1.Choices{
			"calm":  nil,
			"angry": nil,
		}),
		sys1.Score("urgency", "How urgent is this message?", "low", "medium", "high"),
	)
	if err != nil {
		log.Fatal(err)
	}

	billing, _ := resp.Answers.Noul("billing")
	tone, _ := resp.Answers.Choice("tone")
	urgency, _ := resp.Answers.Score("urgency")

	fmt.Printf("billing: %.2f\n", billing.Noul)
	fmt.Printf("tone: %s (confidence %.2f)\n", tone.Choice, tone.Confidence)
	fmt.Printf("urgency: %.2f\n", urgency.Score)
}
```

`New()` reads `TYPESAFE_API_KEY` and the other `TYPESAFE_*` variables from the environment, so no options are needed in the common case.

`Evaluate` takes one state and as many questions as you like. Each question is built with `sys1.Noul`, `sys1.Choice` or `sys1.Score` and carries the name its answer comes back under, which is also how the API keys them on the wire. A blank or duplicate name is rejected locally with `ErrInvalidRequest`.

For a single question, skip `Evaluate` and the `Answers` map with the shortcuts:

```go
billingProb, err := client.Noul(ctx, state, "Is this about a billing issue?")

toneAnswer, err := client.Choice(ctx, state, "What is the tone of this message?", sys1.Choices{
	"calm":  nil,
	"angry": nil,
})

urgencyAnswer, err := client.Score(ctx, state, "How urgent is this message?", "low", "medium", "high")
```

`Noul` returns the bare probability. `Choice` and `Score` return the answer struct, since their confidence and probabilities matter for routing. `resp.Model`, `resp.Usage` and `resp.RequestID` are only available through `Evaluate`.

## Configuration

`sys1.New(opts...)` reads the `TYPESAFE_*` environment variables shared with TypeSafe's Python and JavaScript SDKs, then applies any options on top.

Options:

- `WithAPIKey(key string)`
- `WithBaseURL(raw string)`
- `WithModel(name string)`
- `WithHTTPClient(hc *http.Client)`
- `WithTimeout(d time.Duration)` - per attempt, not the total call
- `WithRetry(p RetryPolicy)`
- `WithHeader(key, value string)`
- `WithUserAgent(ua string)`
- `WithLogger(l *slog.Logger)`

`RequestOption`s override the client's defaults for a single `Evaluate` or `Models` call: `WithRequestModel`, `WithRequestHeader`, `WithRequestRetry`, `WithRequestExtraBody`. In `Evaluate` they sit in the same variadic list as the questions, in any position:

```go
resp, err := client.Evaluate(ctx, state,
	sys1.Noul("billing", "Is this about a billing issue?"),
	sys1.WithRequestModel("jev-1.13.0"),
)
```

Environment variables read by `New`:

| Env var | Purpose | Default |
|---|---|---|
| `TYPESAFE_API_KEY` | API key sent as a bearer token | none, required |
| `TYPESAFE_BASE_URL` | API base URL | `https://api.typesafe.ai` |
| `TYPESAFE_DEFAULT_MODEL` | default model for `Evaluate` and the shortcuts | `jev-latest` |
| `TYPESAFE_LOG_LEVEL` | `debug`, `info`, `warning`, `error` or `off`; logs to stderr at that level | unset, no logging |

Precedence is explicit option, then environment variable, then default. A blank or whitespace-only environment variable is treated as unset. `debug` logs every attempt; `info` logs each scheduled retry.

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
client, err := sys1.New(sys1.WithRetry(sys1.RetryPolicy{MaxRetries: 5}))
```

Override per call:

```go
resp, err := client.Evaluate(ctx, state,
	sys1.Noul("billing", "Is this about a billing issue?"),
	sys1.WithRequestRetry(sys1.RetryPolicy{MaxRetries: 0}),
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

`sys1.Raw(name, fields)` builds a `RawQuestion` whose `Fields` map marshals verbatim, for question fields this library predates. `WithRequestExtraBody` adds extra top-level fields to a request body, shallow-merged over `state`, `model` and `questions`. Any answer with a `"type"` this library does not recognize decodes as a `RawAnswer` holding the raw JSON, instead of failing.

## Examples

| Directory | Shows |
|---|---|
| `examples/basic` | One `Evaluate` call with the guide's billing example: `Noul` billing, `Choice` tone, `Score` urgency, read with the typed accessors |
| `examples/shortcuts` | The same three questions asked through `client.Noul`, `client.Choice` and `client.Score` |
| `examples/typed` | Decoding answers into a caller-owned struct instead of touching the `Answers` map directly |
| `examples/models` | `client.Models`, plus `WithModel` and `WithRequestModel` to pick a specific version |
| `examples/retries` | A client-wide `RetryPolicy`, a per-call override with `WithRequestRetry`, and a `context.WithTimeout` as the total budget |
| `examples/errors` | `errors.As` and `errors.Is`, reading `ValidationError.Path()` for a 422, and a client-side `ErrInvalidRequest` |
| `examples/logging` | `WithLogger` and the per-request debug lines it produces |
| `examples/config` | `New()` picking up environment variables, then explicit options overriding them |
| `examples/forwardcompat` | `sys1.Raw`, `WithRequestExtraBody` and handling `RawAnswer` in a type switch |

Each runs the same way:

```sh
TYPESAFE_API_KEY=... go run ./examples/basic
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

// Package sys1 is a client for TypeSafe's SystemOne API
// (https://docs.typesafe.ai/api): send a piece of content as "state"
// plus one or more typed questions, and get back structured,
// typed answers instead of hand-written JSON.
//
// # Construction
//
// New reads the TYPESAFE_* environment variables the official SDKs
// share (TYPESAFE_API_KEY, TYPESAFE_BASE_URL, TYPESAFE_DEFAULT_MODEL,
// TYPESAFE_LOG_LEVEL), so in the common case no options are needed:
//
//	client, err := sys1.New()
//	if err != nil {
//		// missing API key, or a bad base URL / log level / retry policy
//	}
//
// Functional options override the environment (WithAPIKey, WithBaseURL,
// WithModel, WithHTTPClient, WithTimeout, WithRetry, WithHeader,
// WithUserAgent and WithLogger):
//
//	client, err := sys1.New(sys1.WithAPIKey(key), sys1.WithModel("jev-1.13.0"))
//
// # Asking a single question
//
// For the common case of one question, Client.Noul, Client.Choice and
// Client.Score send it and return the typed answer directly, with no
// map or accessor to unwrap:
//
//	prob, err := client.Noul(ctx, state, "Is this message urgent?")
//
//	choice, err := client.Choice(ctx, state, "Which team should handle this?",
//		sys1.Choices{"billing": nil, "technical": nil, "sales": nil})
//
//	score, err := client.Score(ctx, state, "How frustrated is the customer?",
//		"Calm", "Frustrated", "Very angry")
//
// # Asking several questions about one state
//
// Client.Evaluate takes one state and any number of named questions,
// answering all of them in one round trip. Build each with the
// matching constructor (sys1.Noul, sys1.Choice, sys1.Score, or
// sys1.Raw for forward compatibility), giving it the name its answer
// comes back under:
//
//	resp, err := client.Evaluate(ctx, state,
//		sys1.Noul("urgent", "Does this convey urgency?"),
//		sys1.Choice("team", "Which team should handle this?", sys1.Choices{
//			"billing":   "Payments, invoicing, refunds",
//			"technical": "Bugs, outages, integrations",
//		}),
//		sys1.Score("frustration", "How frustrated is the customer?",
//			"Calm", "Frustrated", "Very angry"),
//	)
//	if err != nil {
//		// handle error
//	}
//	urgent, err := resp.Answers.Noul("urgent")
//	team, err := resp.Answers.Choice("team")
//
// Per-call overrides (WithRequestModel, WithRequestHeader,
// WithRequestRetry, WithRequestExtraBody) go in the same list, in any
// position:
//
//	resp, err := client.Evaluate(ctx, state,
//		sys1.Noul("urgent", "Does this convey urgency?"),
//		sys1.WithRequestModel("jev-1.13.0"),
//	)
//
// resp.Model, resp.Usage and resp.RequestID are only available through
// Evaluate; the single-question shortcuts trade them for brevity.
//
// # Error handling
//
// Every error from a network call can be inspected with errors.As for
// full detail, or errors.Is against a sentinel for the status
// category:
//
//	resp, err := client.Evaluate(ctx, state, questions)
//	var apiErr *sys1.APIError
//	switch {
//	case errors.As(err, &apiErr):
//		// apiErr.StatusCode, apiErr.RequestID, apiErr.Details (for 422)
//	case errors.Is(err, sys1.ErrInvalidRequest):
//		// caught locally before any network call
//	}
//
// Client-side validation failures (no questions, a blank or duplicate
// question name, a Score question with one level, a Choice question
// with 256 options, ...) wrap ErrInvalidRequest and never reach the
// network.
//
// # Retries
//
// Client.Evaluate and Client.Models retry retryable failures
// (408/429/5xx responses, connection errors, per-attempt timeouts)
// under RetryPolicy, honoring a server's Retry-After hint. The default,
// DefaultRetryPolicy, matches TypeSafe's other SDKs. Configure it
// client-wide with WithRetry or per call with WithRequestRetry; a
// caller's context cancellation always stops retries immediately.
package sys1

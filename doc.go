// Package sys1 is a client for TypeSafe's SystemOne API
// (https://docs.typesafe.ai/api): send a piece of content as "state"
// plus one or more typed questions, and get back structured,
// typed answers instead of hand-written JSON.
//
// # Construction
//
// [New] reads the TYPESAFE_* environment variables the official SDKs
// share (TYPESAFE_API_KEY, TYPESAFE_BASE_URL, TYPESAFE_DEFAULT_MODEL,
// TYPESAFE_LOG_LEVEL), so in the common case no options are needed:
//
//	client, err := sys1.New()
//	if err != nil {
//		// missing API key, or a bad base URL / log level
//	}
//
// Functional options override the environment ([WithAPIKey],
// [WithBaseURL], [WithHTTPClient], [WithUserAgent] and [WithLogger]):
//
//	client, err := sys1.New(sys1.WithAPIKey(key))
//
// Everything else - the default model, per-attempt timeout, retry
// policy, extra headers and extra body fields - lives on [Client]
// itself, as methods that return a modified copy rather than options
// passed to New: [Client.WithModel], [Client.WithTimeout],
// [Client.WithRetry], [Client.WithHeader] and [Client.WithExtraBody].
// TYPESAFE_DEFAULT_MODEL sets the model at New time; to change it in
// code instead, derive:
//
//	client = client.WithModel("jev-1.13.0")
//
// # Asking a single question
//
// For the common case of one question, [Client.Noul], [Client.Choice]
// and [Client.Score] send it and return the typed answer directly, with
// no map or accessor to unwrap:
//
//	prob, err := client.Noul(ctx, state, "Is this message urgent?")
//
//	choice, err := client.Choice(ctx, state, "Which team should handle this?",
//		sys1.Choices("billing", "technical", "sales"))
//
//	score, err := client.Score(ctx, state, "How frustrated is the customer?",
//		sys1.Levels("Calm", "Frustrated", "Very angry"))
//
// # Asking several questions about one state
//
// [Client.Ask] takes one state and any number of named questions,
// answering all of them in one round trip. Build each with the
// matching constructor ([Noul], [Choice], [Score], or [Raw] for forward
// compatibility), giving it the name its answer comes back under:
//
//	resp, err := client.Ask(ctx, state,
//		sys1.Noul("urgent", "Does this convey urgency?"),
//		sys1.Choice("team", "Which team should handle this?", sys1.ChoiceCriteria{
//			"billing":   "Payments, invoicing, refunds",
//			"technical": "Bugs, outages, integrations",
//		}),
//		sys1.Score("frustration", "How frustrated is the customer?",
//			sys1.Levels("Calm", "Frustrated", "Very angry")),
//	)
//	if err != nil {
//		// handle error
//	}
//	urgent, err := resp.Answers.Noul("urgent")
//	team, err := resp.Answers.Choice("team")
//
// [Choices] builds the [ChoiceCriteria] for a choice question whose option
// names need no description, and [Levels] builds a score question's
// ordered levels from plain strings. A battery built at runtime is a
// plain []Question, spread as the variadic argument:
//
//	var qs []sys1.Question
//	for _, label := range labels {
//		qs = append(qs, sys1.Noul(label, "Is this about "+label+"?"))
//	}
//	resp, err := client.Ask(ctx, state, qs...)
//
// A per-call override derives a client and calls Ask on the
// copy directly ([Client.WithModel], [Client.WithHeader],
// [Client.WithRetry], [Client.WithExtraBody]):
//
//	resp, err := client.WithModel("jev-1.13.0").Ask(ctx, state,
//		sys1.Noul("urgent", "Does this convey urgency?"),
//	)
//
// resp.Model, resp.Usage and resp.RequestID are only available through
// Ask; the single-question shortcuts trade them for brevity.
//
// # Reading answers
//
// [Answers.Noul] returns a [NoulAnswer], whose Noul field is the
// probability of yes. [Answers.Choice] returns a [ChoiceAnswer] whose
// Ranked method lists the options most probable first. [Answers.Score]
// returns a [ScoreAnswer]; its Nearest and Label methods turn the weighted
// position into a discrete level and its description. Keep the
// thresholds that act on these values in your own code, so policy can
// change without re-running inference.
//
// # Error handling
//
// Every error from a network call can be inspected with [errors.As]
// for full detail, or [errors.Is] against a sentinel for the status
// category:
//
//	resp, err := client.Ask(ctx, state, questions...)
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
// with 256 options, ...) wrap [ErrInvalidRequest] and never reach the
// network.
//
// # Retries
//
// [Client.Ask] and [Client.Models] retry retryable failures
// (408/429/5xx responses, connection errors, per-attempt timeouts)
// under [RetryPolicy], honoring a server's Retry-After hint. The
// default, [DefaultRetryPolicy], matches TypeSafe's other SDKs.
// Configure it client-wide by deriving with [Client.WithRetry], or
// override it for one call by chaining WithRetry straight onto
// Ask; a caller's context cancellation always stops retries
// immediately.
//
// # Testing
//
// Client is a concrete type with no interface to mock. Point it at an
// [net/http/httptest.Server] with [WithBaseURL], or supply an
// [net/http.Client] with a custom RoundTripper via [WithHTTPClient],
// and assert on the request body the fake receives.
package sys1

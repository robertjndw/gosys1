package sys1

// Response is the result of a successful Client.Evaluate call.
type Response struct {
	// Model is the model that performed the evaluation. It may differ
	// from the alias used in the request.
	Model string `json:"model"`
	// Answers holds one answer per question, keyed by the name each
	// question was sent under.
	Answers Answers `json:"answers"`
	// Usage reports token usage for the request.
	Usage Usage `json:"usage"`
	// RequestID is the value of the x-typesafe-request-id response
	// header, useful when reporting an issue to TypeSafe.
	RequestID string `json:"-"`
}

// Usage reports token usage for a single Evaluate call.
type Usage struct {
	// InputTokens is the number of billable input tokens used.
	InputTokens int `json:"input_tokens"`
	// OutputTokens is the number of output tokens used.
	OutputTokens int `json:"output_tokens"`
}

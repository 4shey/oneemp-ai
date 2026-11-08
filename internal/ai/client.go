// Package ai defines the provider-agnostic chat completion contract.
package ai

import "context"

// Role values used by providers.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is a single conversation turn.
type Message struct {
	Role    string
	Content string
}

// Request is a provider-neutral completion request. System is optional; when
// empty the prompt file is expected to carry the full instructions.
type Request struct {
	System   string
	Messages []Message
	JSON     bool
}

// Response is a provider-neutral completion result.
type Response struct {
	Content string
}

// Client is implemented by every AI provider.
type Client interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

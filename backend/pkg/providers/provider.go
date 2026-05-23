// Package providers defines the interface and implementations for LLM providers.
package providers

import "context"

// Provider defines the interface for an LLM provider.
type Provider interface {
	// Chat sends a prompt to the LLM and returns the response string.
	Chat(ctx context.Context, prompt string) (string, error)
	// ChatStream sends a prompt to the LLM scoped to a repository and returns a stream of tokens.
	ChatStream(ctx context.Context, repoID string, prompt string) (<-chan string, <-chan error, error)
	// Name returns the provider's identifier.
	Name() string
}

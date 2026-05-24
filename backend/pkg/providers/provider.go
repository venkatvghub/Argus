// Package providers defines the interface and implementations for LLM providers.
package providers

import (
	"context"
	"errors"
)

// Provider defines the interface for an LLM provider.
type Provider interface {
	// Chat sends a prompt to the LLM and returns the response string.
	Chat(ctx context.Context, prompt string) (string, error)
	// ChatStream sends a prompt to the LLM scoped to a repository and returns a stream of tokens.
	ChatStream(ctx context.Context, repoID string, prompt string) (<-chan string, <-chan error, error)
	// Name returns the provider's identifier.
	Name() string
}

// ModelValidator validates API key reachability and model existence.
// Providers that support discovery implement this interface.
type ModelValidator interface {
	// Validate checks the API key is accepted and the configured model exists.
	// Returns nil on success. If the model is not found, returns ErrModelNotFound
	// wrapping a list of available model IDs.
	Validate(ctx context.Context) error
	// ListModels returns all model IDs available to this API key.
	ListModels(ctx context.Context) ([]string, error)
}

// ErrModelNotFound is returned by Validate when the configured model is not in the available list.
var ErrModelNotFound = errors.New("model not found")

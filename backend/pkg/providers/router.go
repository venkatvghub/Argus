package providers

import (
	"context"
	"fmt"

	"github.com/venkatvghub/argus/pkg/config"
)

// Router handles selection and execution across multiple LLM providers.
type Router struct {
	providers map[string]Provider
	active    string
}

// NewRouter initializes a router with providers based on configuration.
func NewRouter(cfg *config.Config) *Router {
	r := &Router{
		providers: make(map[string]Provider),
		active:    cfg.LLMProvider,
	}

	// Register known providers
	r.Register(NewOpenAIProvider(cfg))
	r.Register(NewAnthropicProvider(cfg))
	r.Register(NewGeminiProvider(cfg))

	return r
}

// Register adds a provider to the router.
func (r *Router) Register(p Provider) {
	r.providers[p.Name()] = p
}

// Chat delegates the chat request to the active provider.
func (r *Router) Chat(ctx context.Context, prompt string) (string, error) {
	p, ok := r.providers[r.active]
	if !ok {
		return "", fmt.Errorf("provider %s not registered", r.active)
	}
	return p.Chat(ctx, prompt)
}

// ChatStream delegates the chat stream request to the active provider.
func (r *Router) ChatStream(ctx context.Context, prompt string) (<-chan string, <-chan error, error) {
	p, ok := r.providers[r.active]
	if !ok {
		return nil, nil, fmt.Errorf("provider %s not registered", r.active)
	}
	return p.ChatStream(ctx, prompt)
}

// GetProvider returns a registered provider by name.

func (r *Router) GetProvider(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

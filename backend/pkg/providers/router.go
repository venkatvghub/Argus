package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/venkatvghub/argus/pkg/config"
)

// Router handles selection and execution across multiple LLM providers.
type Router struct {
	providers map[string]Provider
	active    string
}

// NewRouter initializes a router with providers based on configuration.
func NewRouter(cfg *config.Config) *Router {
	if cfg == nil {
		cfg = &config.Config{}
	}
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
func (r *Router) ChatStream(ctx context.Context, repoID string, prompt string) (<-chan string, <-chan error, error) {
	p, ok := r.providers[r.active]
	if !ok {
		return nil, nil, fmt.Errorf("provider %s not registered", r.active)
	}
	return p.ChatStream(ctx, repoID, prompt)
}

// GetProvider returns a registered provider by name.
func (r *Router) GetProvider(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// ValidateProvider runs Validate on the active provider if it implements ModelValidator.
// If the model is not found, it calls ListModels and returns an error listing available models.
func (r *Router) ValidateProvider(ctx context.Context) error {
	p, ok := r.providers[r.active]
	if !ok {
		return fmt.Errorf("provider %q not registered", r.active)
	}
	mv, ok := p.(ModelValidator)
	if !ok {
		return nil // provider doesn't support validation
	}
	if err := mv.Validate(ctx); err != nil {
		if errors.Is(err, ErrModelNotFound) {
			models, listErr := mv.ListModels(ctx)
			if listErr != nil {
				return fmt.Errorf("%w (could not list models: %v)", err, listErr)
			}
			return fmt.Errorf("%w\navailable models:\n  %s", err, strings.Join(models, "\n  "))
		}
		return err
	}
	return nil
}

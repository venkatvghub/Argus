package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/venkatvghub/argus/pkg/config"
)

// TieredConfig holds the provider and model selections for each generation tier.
type TieredConfig struct {
	ProviderName   string
	CheapModel     string
	MediumModel    string
	PremiumModel   string
}

// TieredRouter routes LLM calls to different models based on generation tier.
type TieredRouter struct {
	cheap   Provider
	medium  Provider
	premium Provider
}

// NewTieredRouter creates a TieredRouter by constructing three Provider instances
// from the same config, each overriding the model field for its tier.
func NewTieredRouter(cfg *config.Config, tc TieredConfig) (*TieredRouter, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	makeProvider := func(model string) (Provider, error) {
		c := *cfg // copy
		switch tc.ProviderName {
		case "anthropic":
			c.AnthropicModel = model
			return NewAnthropicProvider(&c), nil
		case "openai":
			c.OpenAIModel = model
			return NewOpenAIProvider(&c), nil
		case "gemini":
			c.GeminiModel = model
			return NewGeminiProvider(&c), nil
		default:
			return nil, fmt.Errorf("unknown provider %q", tc.ProviderName)
		}
	}
	cheap, err := makeProvider(tc.CheapModel)
	if err != nil {
		return nil, err
	}
	medium, err := makeProvider(tc.MediumModel)
	if err != nil {
		return nil, err
	}
	premium, err := makeProvider(tc.PremiumModel)
	if err != nil {
		return nil, err
	}

	retryCfg := retryConfigFromConfig(cfg)
	return &TieredRouter{
		cheap:   newRetryingProvider(cheap, retryCfg),
		medium:  newRetryingProvider(medium, retryCfg),
		premium: newRetryingProvider(premium, retryCfg),
	}, nil
}

// retryConfigFromConfig converts config fields to a RetryConfig.
func retryConfigFromConfig(cfg *config.Config) RetryConfig {
	return RetryConfig{
		MaxRetries:         cfg.LLMMaxRetries,
		InitialInterval:    time.Duration(cfg.LLMRetryInitialDelayMS) * time.Millisecond,
		MaxInterval:        time.Duration(cfg.LLMRetryMaxDelayMS) * time.Millisecond,
		Multiplier:         cfg.LLMRetryMultiplier,
		FailureThreshold:   cfg.LLMCircuitFailureThreshold,
		ResetTimeout:       time.Duration(cfg.LLMCircuitResetTimeoutS) * time.Second,
	}
}

// ChatTier calls the appropriate provider for the given tier ("cheap", "medium", "premium").
func (t *TieredRouter) ChatTier(ctx context.Context, tier, prompt string) (string, error) {
	switch tier {
	case TierCheap:
		return t.cheap.Chat(ctx, prompt)
	case TierMedium:
		return t.medium.Chat(ctx, prompt)
	case TierPremium:
		return t.premium.Chat(ctx, prompt)
	default:
		return "", fmt.Errorf("unknown tier %q", tier)
	}
}

// usageProvider is optionally implemented by concrete providers to return token counts.
type usageProvider interface {
	ChatWithUsage(ctx context.Context, prompt string) (string, int, int, error)
}

// ChatTierWithUsage calls ChatTier and also returns input/output token counts if the underlying
// provider supports it. Falls back to estimation (len/4) when not supported.
func (t *TieredRouter) ChatTierWithUsage(ctx context.Context, tier, prompt string) (content string, inputTokens, outputTokens int, err error) {
	var p Provider
	switch tier {
	case TierCheap:
		p = t.cheap
	case TierMedium:
		p = t.medium
	case TierPremium:
		p = t.premium
	default:
		return "", 0, 0, fmt.Errorf("unknown tier %q", tier)
	}
	// Unwrap retrying wrapper to reach the concrete provider.
	if rp, ok := p.(*retryingProvider); ok {
		if up, ok := rp.inner.(usageProvider); ok {
			return up.ChatWithUsage(ctx, prompt)
		}
	}
	if up, ok := p.(usageProvider); ok {
		return up.ChatWithUsage(ctx, prompt)
	}
	// Fallback: call Chat and estimate tokens.
	content, err = p.Chat(ctx, prompt)
	if err != nil {
		return "", 0, 0, err
	}
	return content, len(prompt) / 4, len(content) / 4, nil
}

// ProviderForTier returns the Provider for a given tier (for direct use).
func (t *TieredRouter) ProviderForTier(tier string) (Provider, error) {
	switch tier {
	case TierCheap:
		return t.cheap, nil
	case TierMedium:
		return t.medium, nil
	case TierPremium:
		return t.premium, nil
	default:
		return nil, fmt.Errorf("unknown tier %q", tier)
	}
}

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

// Active returns the name of the currently active provider.
func (r *Router) Active() string {
	return r.active
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

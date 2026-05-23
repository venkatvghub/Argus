package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/venkatvghub/argus/pkg/config"
)

// AnthropicProvider implements the Provider interface for Anthropic.
type AnthropicProvider struct {
	apiKey      string
	model       string
	streamDelay time.Duration
}

// NewAnthropicProvider creates a new Anthropic provider instance.
func NewAnthropicProvider(cfg *config.Config) *AnthropicProvider {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &AnthropicProvider{
		apiKey:      cfg.AnthropicKey,
		model:       cfg.AnthropicModel,
		streamDelay: mockStreamDelay(cfg),
	}
}

// Chat sends a prompt to Anthropic.
func (p *AnthropicProvider) Chat(ctx context.Context, prompt string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("Anthropic API key is missing")
	}
	// Mocking API call for Phase 3.1
	return fmt.Sprintf("[Anthropic %s] Response to: %s", p.model, prompt), nil
}

// ChatStream sends a prompt to Anthropic and returns a stream of tokens.
func (p *AnthropicProvider) ChatStream(ctx context.Context, repoID string, prompt string) (<-chan string, <-chan error, error) {
	if p.apiKey == "" {
		return nil, nil, fmt.Errorf("Anthropic API key is missing")
	}
	out := make(chan string)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		response := fmt.Sprintf("[Anthropic %s repo:%s] Response to: %s", p.model, repoID, prompt)
		words := strings.Fields(response)
		for _, word := range words {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case out <- word + " ":
				time.Sleep(p.streamDelay)
			}
		}
	}()

	return out, errCh, nil
}

// Name returns the provider name.
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

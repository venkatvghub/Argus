package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/venkatvghub/argus/pkg/config"
)

// OpenAIProvider implements the Provider interface for OpenAI.
type OpenAIProvider struct {
	apiKey string
	model  string
}

// NewOpenAIProvider creates a new OpenAI provider instance.
func NewOpenAIProvider(cfg *config.Config) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey: cfg.OpenAIKey,
		model:  cfg.OpenAIModel,
	}
}

// Chat sends a prompt to OpenAI.
func (p *OpenAIProvider) Chat(ctx context.Context, prompt string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("OpenAI API key is missing")
	}
	// For Phase 3.1, we implement the structure.
	// Real API calls would be integrated here.
	return fmt.Sprintf("[OpenAI %s] Response to: %s", p.model, prompt), nil
}

// ChatStream sends a prompt to OpenAI and returns a stream of tokens.
func (p *OpenAIProvider) ChatStream(ctx context.Context, prompt string) (<-chan string, <-chan error, error) {
	out := make(chan string)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		response := fmt.Sprintf("[OpenAI %s] Response to: %s", p.model, prompt)
		words := strings.Fields(response)
		for _, word := range words {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case out <- word + " ":
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	return out, errCh, nil
}

// Name returns the provider name.
func (p *OpenAIProvider) Name() string {
	return "openai"
}

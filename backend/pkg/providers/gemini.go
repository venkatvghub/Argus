package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/venkatvghub/argus/pkg/config"
)

// GeminiProvider implements the Provider interface for Google Gemini.
type GeminiProvider struct {
	apiKey      string
	model       string
	streamDelay time.Duration
}

// NewGeminiProvider creates a new Gemini provider instance.
func NewGeminiProvider(cfg *config.Config) *GeminiProvider {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &GeminiProvider{
		apiKey:      cfg.GeminiKey,
		model:       cfg.GeminiModel,
		streamDelay: mockStreamDelay(cfg),
	}
}

// Chat sends a prompt to Gemini.
func (p *GeminiProvider) Chat(ctx context.Context, prompt string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("Gemini API key is missing")
	}
	// Mocking API call for Phase 3.1
	return fmt.Sprintf("[Gemini %s] Response to: %s", p.model, prompt), nil
}

// ChatStream sends a prompt to Gemini and returns a stream of tokens.
func (p *GeminiProvider) ChatStream(ctx context.Context, repoID string, prompt string) (<-chan string, <-chan error, error) {
	if p.apiKey == "" {
		return nil, nil, fmt.Errorf("Gemini API key is missing")
	}
	out := make(chan string)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		response := fmt.Sprintf("[Gemini %s repo:%s] Response to: %s", p.model, repoID, prompt)
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
func (p *GeminiProvider) Name() string {
	return "gemini"
}

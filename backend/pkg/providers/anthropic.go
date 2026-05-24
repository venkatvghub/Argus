package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/venkatvghub/argus/pkg/config"
)

// AnthropicProvider implements the Provider interface for Anthropic.
type AnthropicProvider struct {
	apiKey string
	model  string
}

// NewAnthropicProvider creates a new Anthropic provider instance.
func NewAnthropicProvider(cfg *config.Config) *AnthropicProvider {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &AnthropicProvider{
		apiKey: cfg.AnthropicKey,
		model:  cfg.AnthropicModel,
	}
}

// anthropicRequest is the JSON body for the Messages API.
type anthropicRequest struct {
	Model     string                `json:"model"`
	MaxTokens int                   `json:"max_tokens"`
	Messages  []anthropicMessage    `json:"messages"`
	Stream    bool                  `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse is the non-streaming response envelope.
type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

// anthropicStreamData is the data payload for content_block_delta SSE events.
type anthropicStreamData struct {
	Delta struct {
		Text string `json:"text"`
	} `json:"delta"`
}

func (p *AnthropicProvider) newRequest(ctx context.Context, body []byte, stream bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	return req, nil
}

// Chat sends a prompt to the Anthropic Messages API and returns the full response.
func (p *AnthropicProvider) Chat(ctx context.Context, prompt string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("Anthropic API key is missing")
	}

	payload := anthropicRequest{
		Model:     p.model,
		MaxTokens: defaultMaxTokens,
		Messages:  []anthropicMessage{{Role: "user", Content: prompt}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("anthropic chat: marshal: %w", err)
	}

	req, err := p.newRequest(ctx, body, false)
	if err != nil {
		return "", fmt.Errorf("anthropic chat: request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic chat: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic chat: status %d: %s", resp.StatusCode, string(raw))
	}

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("anthropic chat: decode: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("anthropic chat: empty content in response")
	}
	return result.Content[0].Text, nil
}

// ChatStream sends a prompt to Anthropic with streaming enabled and emits tokens on the returned channel.
func (p *AnthropicProvider) ChatStream(ctx context.Context, repoID string, prompt string) (<-chan string, <-chan error, error) {
	if p.apiKey == "" {
		return nil, nil, fmt.Errorf("Anthropic API key is missing")
	}

	payload := anthropicRequest{
		Model:     p.model,
		MaxTokens: defaultMaxTokens,
		Messages:  []anthropicMessage{{Role: "user", Content: prompt}},
		Stream:    true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("anthropic stream: marshal: %w", err)
	}

	req, err := p.newRequest(ctx, body, true)
	if err != nil {
		return nil, nil, fmt.Errorf("anthropic stream: request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("anthropic stream: http: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, nil, fmt.Errorf("anthropic stream: status %d: %s", resp.StatusCode, string(raw))
	}

	out := make(chan string)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)
		defer resp.Body.Close()

		// Anthropic SSE format:
		//   event: <type>
		//   data: <json>
		//   (blank line)
		// We emit text only for event: content_block_delta.
		scanner := bufio.NewScanner(resp.Body)
		var currentEvent string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				currentEvent = strings.TrimPrefix(line, "event: ")
				continue
			}
			if line == "event: message_stop" || currentEvent == "message_stop" {
				return
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			if currentEvent != "content_block_delta" {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var chunk anthropicStreamData
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue // skip malformed chunks
			}
			text := chunk.Delta.Text
			if text == "" {
				continue
			}
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case out <- text:
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("anthropic stream: scan: %w", err)
		}
	}()

	return out, errCh, nil
}

// Name returns the provider name.
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// anthropicModelsResponse is the JSON response for GET /v1/models.
type anthropicModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// ListModels fetches all model IDs available to the configured API key.
func (p *AnthropicProvider) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, anthropicModelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("anthropic list models: request: %w", err)
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic list models: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic list models: status %d: %s", resp.StatusCode, string(raw))
	}

	var result anthropicModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("anthropic list models: decode: %w", err)
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	sort.Strings(models)
	return models, nil
}

// Validate checks that the API key is accepted and the configured model exists.
func (p *AnthropicProvider) Validate(ctx context.Context) error {
	if p.apiKey == "" {
		return fmt.Errorf("anthropic: API key is missing")
	}
	models, err := p.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("anthropic: could not reach API: %w", err)
	}
	for _, m := range models {
		if m == p.model {
			return nil
		}
	}
	return fmt.Errorf("%w: %q not in available models: %v", ErrModelNotFound, p.model, models)
}

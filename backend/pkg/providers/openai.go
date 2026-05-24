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

// OpenAIProvider implements the Provider interface for OpenAI-compatible APIs.
type OpenAIProvider struct {
	apiKey       string
	baseURL      string
	model        string
	isOpenRouter bool // true when baseURL != openAIDefaultBaseURL
	client       *http.Client
}

// NewOpenAIProvider creates a new OpenAI provider instance.
func NewOpenAIProvider(cfg *config.Config) *OpenAIProvider {
	if cfg == nil {
		cfg = &config.Config{}
	}
	baseURL := cfg.OpenAIBaseURL
	if baseURL == "" {
		baseURL = openAIDefaultBaseURL
	}
	return &OpenAIProvider{
		apiKey:       cfg.OpenAIKey,
		baseURL:      baseURL,
		model:        cfg.OpenAIModel,
		isOpenRouter: baseURL != openAIDefaultBaseURL,
		client:       llmHTTPClient(cfg),
	}
}

func (p *OpenAIProvider) httpClient() *http.Client {
	if p.client != nil {
		return p.client
	}
	return llmHTTPClient(&config.Config{})
}

// openAIChatRequest is the JSON body for chat completions.
type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatResponse is the non-streaming response envelope.
type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// openAIStreamChunk is one SSE chunk in a streaming response.
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func (p *OpenAIProvider) newRequest(ctx context.Context, path string, body []byte) (*http.Request, error) {
	url := p.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if p.isOpenRouter {
		req.Header.Set("HTTP-Referer", openRouterReferer)
		req.Header.Set("X-Title", openRouterTitle)
	}
	return req, nil
}

// Chat sends a prompt to the OpenAI chat completions endpoint and returns the full response.
func (p *OpenAIProvider) Chat(ctx context.Context, prompt string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("OpenAI API key is missing")
	}

	payload := openAIChatRequest{
		Model:    p.model,
		Messages: []openAIChatMessage{{Role: "user", Content: prompt}},
		Stream:   false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("openai chat: marshal: %w", err)
	}

	req, err := p.newRequest(ctx, "/chat/completions", body)
	if err != nil {
		return "", fmt.Errorf("openai chat: request: %w", err)
	}

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("openai chat: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai chat: status %d: %s", resp.StatusCode, string(raw))
	}

	var result openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("openai chat: decode: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai chat: empty choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

// ChatStream sends a prompt to OpenAI with streaming enabled and emits tokens on the returned channel.
func (p *OpenAIProvider) ChatStream(ctx context.Context, repoID string, prompt string) (<-chan string, <-chan error, error) {
	if p.apiKey == "" {
		return nil, nil, fmt.Errorf("OpenAI API key is missing")
	}

	payload := openAIChatRequest{
		Model:    p.model,
		Messages: []openAIChatMessage{{Role: "user", Content: prompt}},
		Stream:   true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("openai stream: marshal: %w", err)
	}

	req, err := p.newRequest(ctx, "/chat/completions", body)
	if err != nil {
		return nil, nil, fmt.Errorf("openai stream: request: %w", err)
	}

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("openai stream: http: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, nil, fmt.Errorf("openai stream: status %d: %s", resp.StatusCode, string(raw))
	}

	out := make(chan string)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}
			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue // skip malformed chunks
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			content := chunk.Choices[0].Delta.Content
			if content == "" {
				continue
			}
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case out <- content:
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("openai stream: scan: %w", err)
		}
	}()

	return out, errCh, nil
}

// Name returns the provider name.
func (p *OpenAIProvider) Name() string {
	return "openai"
}

// openAIModelsResponse is the JSON response for GET /models.
type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// ListModels fetches all model IDs available to the configured API key.
func (p *OpenAIProvider) ListModels(ctx context.Context) ([]string, error) {
	url := p.baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("openai list models: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	if p.isOpenRouter {
		req.Header.Set("HTTP-Referer", openRouterReferer)
		req.Header.Set("X-Title", openRouterTitle)
	}

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai list models: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai list models: status %d: %s", resp.StatusCode, string(raw))
	}

	var result openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai list models: decode: %w", err)
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	sort.Strings(models)
	return models, nil
}

// Validate checks that the API key is accepted and the configured model exists.
func (p *OpenAIProvider) Validate(ctx context.Context) error {
	if p.apiKey == "" {
		return fmt.Errorf("openai: API key is missing")
	}
	models, err := p.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("openai: could not reach API: %w", err)
	}
	for _, m := range models {
		if m == p.model {
			return nil
		}
	}
	return fmt.Errorf("%w: %q not in available models: %v", ErrModelNotFound, p.model, models)
}

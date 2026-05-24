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

var geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

// GeminiProvider implements the Provider interface for Google Gemini.
type GeminiProvider struct {
	apiKey string
	model  string
}

// NewGeminiProvider creates a new Gemini provider instance.
func NewGeminiProvider(cfg *config.Config) *GeminiProvider {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &GeminiProvider{
		apiKey: cfg.GeminiKey,
		model:  cfg.GeminiModel,
	}
}

// geminiRequest is the JSON body for the Gemini generateContent API.
type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

// geminiResponse is the non-streaming response envelope.
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (p *GeminiProvider) buildPayload(prompt string) ([]byte, error) {
	payload := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	}
	return json.Marshal(payload)
}

// Chat sends a prompt to the Gemini generateContent endpoint and returns the full response.
func (p *GeminiProvider) Chat(ctx context.Context, prompt string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("Gemini API key is missing")
	}

	body, err := p.buildPayload(prompt)
	if err != nil {
		return "", fmt.Errorf("gemini chat: marshal: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiBaseURL, p.model, p.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("gemini chat: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini chat: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini chat: status %d: %s", resp.StatusCode, string(raw))
	}

	var result geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("gemini chat: decode: %w", err)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini chat: empty candidates in response")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}

// ChatStream sends a prompt to Gemini with SSE streaming enabled and emits tokens on the returned channel.
func (p *GeminiProvider) ChatStream(ctx context.Context, repoID string, prompt string) (<-chan string, <-chan error, error) {
	if p.apiKey == "" {
		return nil, nil, fmt.Errorf("Gemini API key is missing")
	}

	body, err := p.buildPayload(prompt)
	if err != nil {
		return nil, nil, fmt.Errorf("gemini stream: marshal: %w", err)
	}

	url := fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse&key=%s", geminiBaseURL, p.model, p.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("gemini stream: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("gemini stream: http: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, nil, fmt.Errorf("gemini stream: status %d: %s", resp.StatusCode, string(raw))
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
			var chunk geminiResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue // skip malformed chunks
			}
			if len(chunk.Candidates) == 0 || len(chunk.Candidates[0].Content.Parts) == 0 {
				continue
			}
			text := chunk.Candidates[0].Content.Parts[0].Text
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
			errCh <- fmt.Errorf("gemini stream: scan: %w", err)
		}
	}()

	return out, errCh, nil
}

// Name returns the provider name.
func (p *GeminiProvider) Name() string {
	return "gemini"
}

// geminiModelsResponse is the JSON response for GET /v1beta/models.
type geminiModelsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// ListModels fetches all model IDs available to the configured API key.
// The Gemini API returns names like "models/gemini-2.0-flash"; this strips the prefix.
func (p *GeminiProvider) ListModels(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s?key=%s", geminiBaseURL, p.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("gemini list models: request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini list models: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini list models: status %d: %s", resp.StatusCode, string(raw))
	}

	var result geminiModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("gemini list models: decode: %w", err)
	}

	models := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		models = append(models, id)
	}
	sort.Strings(models)
	return models, nil
}

// Validate checks that the API key is accepted and the configured model exists.
func (p *GeminiProvider) Validate(ctx context.Context) error {
	if p.apiKey == "" {
		return fmt.Errorf("gemini: API key is missing")
	}
	models, err := p.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("gemini: could not reach API: %w", err)
	}
	for _, m := range models {
		if m == p.model {
			return nil
		}
	}
	return fmt.Errorf("%w: %q not in available models: %v", ErrModelNotFound, p.model, models)
}

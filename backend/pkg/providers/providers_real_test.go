package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTransport implements http.RoundTripper for testing with mocked servers.
type mockTransport struct {
	mockURL   string
	roundTrip func(*http.Request) (*http.Response, error)
}

// RoundTrip intercepts HTTP requests and redirects them to a mock server.
func (mt *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect the request to the mock server, preserving the path and query
	newReq := req.Clone(req.Context())
	mockURL, _ := url.Parse(mt.mockURL)
	newReq.URL.Scheme = mockURL.Scheme
	newReq.URL.Host = mockURL.Host
	// Don't override the path -- keep the original path from the request
	return http.DefaultTransport.RoundTrip(newReq)
}

// testMockProvider is a simple mock for Router testing (doesn't implement ModelValidator).
type testMockProvider struct {
	name string
}

func (m *testMockProvider) Chat(ctx context.Context, prompt string) (string, error) {
	return "mock response", nil
}

func (m *testMockProvider) ChatStream(ctx context.Context, repoID string, prompt string) (<-chan string, <-chan error, error) {
	ch := make(chan string)
	errCh := make(chan error)
	close(ch)
	close(errCh)
	return ch, errCh, nil
}

func (m *testMockProvider) Name() string {
	return m.name
}

// ============================================================================
// OpenAI Tests
// ============================================================================

// TestOpenAIChat_Success tests successful non-streaming chat completion.
func TestOpenAIChat_Success(t *testing.T) {
	// Create a mock server that returns a valid OpenAI chat response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		// The path may not include "/v1/" since we're injecting the base URL
		require.Contains(t, r.URL.Path, "chat/completions")

		var req openAIChatRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		require.Equal(t, "gpt-4o", req.Model)
		require.False(t, req.Stream)

		resp := openAIChatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{
					Message: struct {
						Content string `json:"content"`
					}{
						Content: "hello world",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create provider with mocked server URL as baseURL
	p := &OpenAIProvider{
		apiKey:  "test-key",
		baseURL: server.URL,
		model:   "gpt-4o",
	}

	result, err := p.Chat(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", result)
}

// TestOpenAIChat_HTTPError tests handling of HTTP errors.
func TestOpenAIChat_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"Invalid API key"}`))
	}))
	defer server.Close()

	p := &OpenAIProvider{
		apiKey:  "bad-key",
		baseURL: server.URL,
		model:   "gpt-4o",
	}

	_, err := p.Chat(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

// TestOpenAIChat_MissingKey tests that Chat returns error when API key is empty.
func TestOpenAIChat_MissingKey(t *testing.T) {
	// Server should not be called
	p := &OpenAIProvider{
		apiKey:  "",
		baseURL: "https://api.openai.com",
		model:   "gpt-4o",
	}

	_, err := p.Chat(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OpenAI API key")
}

// TestOpenAIChatStream_Success tests successful streaming chat completion.
func TestOpenAIChatStream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)

		var req openAIChatRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		require.True(t, req.Stream)

		w.Header().Set("Content-Type", "text/event-stream")

		// Send SSE chunks
		fmt.Fprintf(w, "data: %s\n\n", mustMarshalJSON(openAIStreamChunk{
			Choices: []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			}{
				{Delta: struct {
					Content string `json:"content"`
				}{Content: "hello"}},
			},
		}))

		fmt.Fprintf(w, "data: %s\n\n", mustMarshalJSON(openAIStreamChunk{
			Choices: []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			}{
				{Delta: struct {
					Content string `json:"content"`
				}{Content: " world"}},
			},
		}))

		// Send completion token
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := &OpenAIProvider{
		apiKey:  "test-key",
		baseURL: server.URL,
		model:   "gpt-4o",
	}

	out, errCh, err := p.ChatStream(context.Background(), "repo-123", "test prompt")
	require.NoError(t, err)

	var result strings.Builder
	for token := range out {
		result.WriteString(token)
	}

	// Check for stream errors
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
	default:
	}

	assert.Equal(t, "hello world", result.String())
}

// TestOpenAIListModels_Success tests successful model listing.
func TestOpenAIListModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)
		require.Contains(t, r.URL.Path, "models")

		resp := struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}{
			Data: []struct {
				ID string `json:"id"`
			}{
				{ID: "gpt-4o"},
				{ID: "gpt-4o-mini"},
				{ID: "gpt-4-turbo"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &OpenAIProvider{
		apiKey:  "test-key",
		baseURL: server.URL,
		model:   "gpt-4o",
	}

	models, err := p.ListModels(context.Background())
	require.NoError(t, err)

	// Should be sorted
	assert.Equal(t, []string{"gpt-4-turbo", "gpt-4o", "gpt-4o-mini"}, models)
}

// TestOpenAIListModels_HTTPError tests error handling in ListModels.
func TestOpenAIListModels_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"Forbidden"}`))
	}))
	defer server.Close()

	p := &OpenAIProvider{
		apiKey:  "bad-key",
		baseURL: server.URL,
		model:   "gpt-4o",
	}

	_, err := p.ListModels(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

// TestOpenAIValidate_ModelFound tests successful validation when model exists.
func TestOpenAIValidate_ModelFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "models") {
			resp := struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}{
				Data: []struct {
					ID string `json:"id"`
				}{
					{ID: "gpt-4o"},
					{ID: "gpt-4o-mini"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	p := &OpenAIProvider{
		apiKey:  "test-key",
		baseURL: server.URL,
		model:   "gpt-4o",
	}

	err := p.Validate(context.Background())
	require.NoError(t, err)
}

// TestOpenAIValidate_ModelNotFound tests validation error when model doesn't exist.
func TestOpenAIValidate_ModelNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "models") {
			resp := struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}{
				Data: []struct {
					ID string `json:"id"`
				}{
					{ID: "gpt-4o"},
					{ID: "gpt-4o-mini"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	p := &OpenAIProvider{
		apiKey:  "test-key",
		baseURL: server.URL,
		model:   "nonexistent-model",
	}

	err := p.Validate(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrModelNotFound))
}

// ============================================================================
// Anthropic Tests
// ============================================================================

// TestAnthropicChat_Success tests successful non-streaming chat completion.
func TestAnthropicChat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)

		// Verify authorization header
		authHeader := r.Header.Get("x-api-key")
		require.NotEmpty(t, authHeader)

		var req anthropicRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		require.Equal(t, "claude-3-5-haiku-20241022", req.Model)
		require.False(t, req.Stream)

		resp := anthropicResponse{
			Content: []struct {
				Text string `json:"text"`
			}{
				{Text: "hello from anthropic"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &AnthropicProvider{
		apiKey: "test-key",
		model:  "claude-3-5-haiku-20241022",
		client: &http.Client{Transport: &mockTransport{mockURL: server.URL}},
	}

	result, err := p.Chat(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "hello from anthropic", result)
}

// TestAnthropicChat_MissingKey tests error when API key is empty.
func TestAnthropicChat_MissingKey(t *testing.T) {
	p := &AnthropicProvider{
		apiKey: "",
		model:  "claude-3-5-haiku-20241022",
	}

	_, err := p.Chat(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Anthropic API key")
}

// TestAnthropicListModels_Success tests successful model listing.
func TestAnthropicListModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)

		resp := struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}{
			Data: []struct {
				ID string `json:"id"`
			}{
				{ID: "claude-3-5-sonnet-20241022"},
				{ID: "claude-3-5-haiku-20241022"},
				{ID: "claude-3-opus-20250219"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &AnthropicProvider{
		apiKey: "test-key",
		model:  "claude-3-5-haiku-20241022",
		client: &http.Client{Transport: &mockTransport{mockURL: server.URL}},
	}

	models, err := p.ListModels(context.Background())
	require.NoError(t, err)

	// Should be sorted
	expectedModels := []string{
		"claude-3-5-haiku-20241022",
		"claude-3-5-sonnet-20241022",
		"claude-3-opus-20250219",
	}
	assert.Equal(t, expectedModels, models)
}

// TestAnthropicValidate_ModelNotFound tests validation error when model doesn't exist.
func TestAnthropicValidate_ModelNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}{
			Data: []struct {
				ID string `json:"id"`
			}{
				{ID: "claude-3-5-sonnet-20241022"},
				{ID: "claude-3-5-haiku-20241022"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &AnthropicProvider{
		apiKey: "test-key",
		model:  "nonexistent-claude-model",
		client: &http.Client{Transport: &mockTransport{mockURL: server.URL}},
	}

	err := p.Validate(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrModelNotFound))
}

// ============================================================================
// Gemini Tests
// ============================================================================

// TestGeminiChat_Success tests successful non-streaming chat completion.
func TestGeminiChat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)

		// Verify API key is passed in header, not the URL query string.
		require.Equal(t, "test-key", r.Header.Get("x-goog-api-key"))
		require.Empty(t, r.URL.Query().Get("key"))

		var req geminiRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		require.Len(t, req.Contents, 1)

		resp := geminiResponse{
			Candidates: []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			}{
				{
					Content: struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					}{
						Parts: []struct {
							Text string `json:"text"`
						}{
							{Text: "hello from gemini"},
						},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &GeminiProvider{
		apiKey: "test-key",
		model:  "gemini-2.0-flash",
		client: &http.Client{Transport: &mockTransport{mockURL: server.URL}},
	}

	result, err := p.Chat(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "hello from gemini", result)
}

// TestGeminiChat_MissingKey tests error when API key is empty.
func TestGeminiChat_MissingKey(t *testing.T) {
	p := &GeminiProvider{
		apiKey: "",
		model:  "gemini-2.0-flash",
	}

	_, err := p.Chat(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Gemini API key")
}

// TestGeminiListModels_Success tests successful model listing.
func TestGeminiListModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)

		resp := geminiModelsResponse{
			Models: []struct {
				Name string `json:"name"`
			}{
				{Name: "models/gemini-2.0-flash"},
				{Name: "models/gemini-1.5-pro"},
				{Name: "models/gemini-1.5-flash"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &GeminiProvider{
		apiKey: "test-key",
		model:  "gemini-2.0-flash",
		client: &http.Client{Transport: &mockTransport{mockURL: server.URL}},
	}

	models, err := p.ListModels(context.Background())
	require.NoError(t, err)

	// Should strip "models/" prefix and be sorted
	expectedModels := []string{
		"gemini-1.5-flash",
		"gemini-1.5-pro",
		"gemini-2.0-flash",
	}
	assert.Equal(t, expectedModels, models)
}

// TestGeminiValidate_ModelNotFound tests validation error when model doesn't exist.
func TestGeminiValidate_ModelNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiModelsResponse{
			Models: []struct {
				Name string `json:"name"`
			}{
				{Name: "models/gemini-2.0-flash"},
				{Name: "models/gemini-1.5-pro"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &GeminiProvider{
		apiKey: "test-key",
		model:  "nonexistent-gemini-model",
		client: &http.Client{Transport: &mockTransport{mockURL: server.URL}},
	}

	err := p.Validate(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrModelNotFound))
}

// ============================================================================
// Router Tests
// ============================================================================

// TestRouterValidateProvider_NoOp tests that ValidateProvider returns nil
// for providers without ModelValidator interface.
func TestRouterValidateProvider_NoOp(t *testing.T) {
	// Create a mock provider that doesn't implement ModelValidator
	mock := &testMockProvider{name: "basic"}

	router := &Router{
		providers: map[string]Provider{
			"basic": mock,
		},
		active: "basic",
	}

	err := router.ValidateProvider(context.Background())
	require.NoError(t, err)
}

// TestRouterValidateProvider_ModelNotFound tests ValidateProvider error handling
// when model is not found and lists available models.
func TestRouterValidateProvider_ModelNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}{
			Data: []struct {
				ID string `json:"id"`
			}{
				{ID: "gpt-4o"},
				{ID: "gpt-4o-mini"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &OpenAIProvider{
		apiKey:  "test-key",
		baseURL: server.URL,
		model:   "nonexistent-model",
	}

	router := &Router{
		providers: map[string]Provider{
			"openai": p,
		},
		active: "openai",
	}

	err := router.ValidateProvider(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrModelNotFound))
	// Should include list of available models
	assert.Contains(t, err.Error(), "gpt-4o")
	assert.Contains(t, err.Error(), "available models")
}

// ============================================================================
// Helper Functions
// ============================================================================

// mustMarshalJSON marshals an object to JSON or panics.
func mustMarshalJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

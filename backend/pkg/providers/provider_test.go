package providers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/venkatvghub/argus/pkg/config"
)

// MockProvider is a mock implementation of the Provider interface.
type MockProvider struct {
	name     string
	chatFunc func(ctx context.Context, prompt string) (string, error)
}

func (m *MockProvider) Chat(ctx context.Context, prompt string) (string, error) {
	if m.chatFunc != nil {
		return m.chatFunc(ctx, prompt)
	}
	return "mock response", nil
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) ChatStream(ctx context.Context, repoID string, prompt string) (<-chan string, <-chan error, error) {
	ch := make(chan string)
	errCh := make(chan error)
	close(ch)
	close(errCh)
	return ch, errCh, nil
}

func TestRouterSelection(t *testing.T) {
	cfg := &config.Config{
		LLMProvider: "mock1",
	}

	router := &Router{
		providers: make(map[string]Provider),
		active:    cfg.LLMProvider,
	}

	mock1 := &MockProvider{name: "mock1"}
	mock2 := &MockProvider{name: "mock2"}

	router.Register(mock1)
	router.Register(mock2)

	// Test that it selects mock1
	resp, err := router.Chat(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != "mock response" {
		t.Errorf("expected 'mock response', got %s", resp)
	}

	// Change active provider
	router.active = "mock2"
	resp, err = router.Chat(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != "mock response" {
		t.Errorf("expected 'mock response', got %s", resp)
	}

	// Test invalid provider
	router.active = "invalid"
	_, err = router.Chat(context.Background(), "hello")
	if err == nil {
		t.Error("expected error for invalid provider, got nil")
	}
}

func TestProviderErrorHandling(t *testing.T) {
	mockErr := errors.New("connection failed")
	mock := &MockProvider{
		name: "error-prone",
		chatFunc: func(ctx context.Context, prompt string) (string, error) {
			return "", mockErr
		},
	}

	cfg := &config.Config{LLMProvider: "error-prone"}
	router := &Router{
		providers: make(map[string]Provider),
		active:    cfg.LLMProvider,
	}
	router.Register(mock)

	_, err := router.Chat(context.Background(), "hello")
	if !errors.Is(err, mockErr) {
		t.Errorf("expected error %v, got %v", mockErr, err)
	}
}

func TestOpenAIProviderKeyMissing(t *testing.T) {
	cfg := &config.Config{
		OpenAIKey: "",
	}
	p := NewOpenAIProvider(cfg)
	_, err := p.Chat(context.Background(), "hello")
	if err == nil {
		t.Error("expected error for missing API key, got nil")
	}
	if !strings.Contains(err.Error(), "OpenAI API key") {
		t.Errorf("expected error containing %q, got %q", "OpenAI API key", err.Error())
	}
}

package server

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/argus"
	"github.com/venkatvghub/argus/pkg/config"
	"github.com/venkatvghub/argus/pkg/constants"
	"github.com/venkatvghub/argus/pkg/models"
	"github.com/venkatvghub/argus/pkg/providers"
)

func TestSSEHandler(t *testing.T) {
	ctx := context.Background()
	inst, err := argus.New(ctx, nil)
	assert.NoError(t, err)
	defer inst.Close()

	srv := NewRESTServer(inst)

	t.Run("Subscribe and Receive Updates", func(t *testing.T) {
		job := inst.Jobs.CreateJob("sse_test")

		req := httptest.NewRequest("GET", "/api/events?jobId="+job.ID, nil)
		rr := httptest.NewRecorder()

		// Since sseHandler is blocking, we run it in a goroutine
		ctx, cancel := context.WithTimeout(req.Context(), 1*time.Second)
		defer cancel()
		req = req.WithContext(ctx)

		done := make(chan bool)
		go func() {
			srv.sseHandler(rr, req)
			done <- true
		}()

		// Give it a moment to start subscribing
		time.Sleep(100 * time.Millisecond)

		// Send an update
		inst.Jobs.UpdateStatus(job.ID, models.JobStatusInProgress, "50%", nil)
		inst.Jobs.UpdateStatus(job.ID, models.JobStatusCompleted, "100%", nil)

		select {
		case <-done:
			// Handler returned as expected on completion
		case <-time.After(500 * time.Millisecond):
			// If it hasn't returned, that's also fine if it's still waiting for context
		}

		body := rr.Body.String()
		assert.Contains(t, body, "data: {")
		assert.Contains(t, body, "in_progress")
		assert.Contains(t, body, "completed")
		assert.Contains(t, body, "50%")
		assert.Contains(t, body, "100%")
	})

	t.Run("Global Subscription", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/events", nil) // no jobId
		rr := httptest.NewRecorder()

		ctx, cancel := context.WithTimeout(req.Context(), 500*time.Millisecond)
		defer cancel()
		req = req.WithContext(ctx)

		go func() {
			srv.sseHandler(rr, req)
		}()

		time.Sleep(100 * time.Millisecond)

		job := inst.Jobs.CreateJob("global_test")
		inst.Jobs.UpdateStatus(job.ID, models.JobStatusCompleted, "Done", nil)

		time.Sleep(100 * time.Millisecond)

		body := rr.Body.String()
		assert.Contains(t, body, "global_test")
	})
}

func TestSSEParsing(t *testing.T) {
	// More detailed parsing test
	ctx := context.Background()
	inst, err := argus.New(ctx, nil)
	if err != nil {
		t.Fatalf("failed to create argus instance: %v", err)
	}
	defer inst.Close()
	srv := NewRESTServer(inst)

	job := inst.Jobs.CreateJob("parse_test")

	req := httptest.NewRequest("GET", "/api/events?jobId="+job.ID, nil)
	rr := httptest.NewRecorder()

	// Create a pipe to read the stream
	pr, pw := httptest.NewRecorder().Body, httptest.NewRecorder().Body // This won't work as expected with Recorder
	_ = pr
	_ = pw

	// We'll use a custom response writer that we can read from
	type pipeWriter struct {
		http.ResponseWriter
		ch chan string
	}
	// Actually httptest.NewRecorder is fine if we can read its body periodically.
	// But it only exposes body as a buffer.

	// Let's just use the body string and parse it as SSE
	go func() {
		time.Sleep(50 * time.Millisecond)
		inst.Jobs.UpdateStatus(job.ID, models.JobStatusCompleted, "100%", nil)
	}()

	srv.sseHandler(rr, req)

	scanner := bufio.NewScanner(strings.NewReader(rr.Body.String()))
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var j models.Job
			err := json.Unmarshal([]byte(data), &j)
			assert.NoError(t, err)
			if j.ID == job.ID && j.Status == models.JobStatusCompleted {
				found = true
			}
		}
	}
	assert.True(t, found)
}

func TestChatStreamHandler_MissingRepoID(t *testing.T) {
	ctx := context.Background()
	inst, err := argus.New(ctx, nil)
	require.NoError(t, err)
	defer inst.Close()

	srv := NewRESTServer(inst)

	req := httptest.NewRequest("GET", "/api/chat/stream?q=hello", nil)
	rr := httptest.NewRecorder()

	srv.chatStreamHandler(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "repoID is required")
}

func TestChatStreamHandler_MissingQuery(t *testing.T) {
	ctx := context.Background()
	inst, err := argus.New(ctx, nil)
	require.NoError(t, err)
	defer inst.Close()

	srv := NewRESTServer(inst)

	req := httptest.NewRequest("GET", "/api/chat/stream?repoID=abc", nil)
	rr := httptest.NewRecorder()

	srv.chatStreamHandler(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "q is required")
}

func TestChatStreamHandler_RepoNotFound(t *testing.T) {
	ctx := context.Background()
	inst, err := argus.New(ctx, nil)
	require.NoError(t, err)
	defer inst.Close()

	srv := NewRESTServer(inst)
	router := createRouterWithMockProvider([]string{"token"}, nil)
	srv.SetProvider(router)

	req := httptest.NewRequest("GET", "/api/chat/stream?repoID=unknown-repo&q=test", nil)
	rr := httptest.NewRecorder()

	srv.chatStreamHandler(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "repo not found")
}

func TestChatStreamHandler_NoProvider(t *testing.T) {
	ctx := context.Background()
	inst, err := argus.New(ctx, nil)
	require.NoError(t, err)
	defer inst.Close()

	srv := NewRESTServer(inst)
	// provider is nil by default

	req := httptest.NewRequest("GET", "/api/chat/stream?repoID=abc&q=hello", nil)
	rr := httptest.NewRecorder()

	srv.chatStreamHandler(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), "LLM provider not configured")
}

func TestChatStreamHandler_StreamsTokens(t *testing.T) {
	ctx := context.Background()
	inst, err := argus.New(ctx, nil)
	require.NoError(t, err)
	defer inst.Close()

	repoID := seedChatStreamRepo(t, inst)

	srv := NewRESTServer(inst)

	// Create a router with mock provider that streams 3 tokens
	router := createRouterWithMockProvider([]string{"token1", "token2", "token3"}, nil)
	srv.SetProvider(router)

	req := httptest.NewRequest("GET", "/api/chat/stream?repoID="+repoID+"&q=test", nil)
	rr := httptest.NewRecorder()

	srv.chatStreamHandler(rr, req)

	body := rr.Body.String()
	assert.Contains(t, body, "data: token1\n\n")
	assert.Contains(t, body, "data: token2\n\n")
	assert.Contains(t, body, "data: token3\n\n")
	assert.Contains(t, body, "data: [DONE]\n\n")
}

func TestChatStreamHandler_StreamingError(t *testing.T) {
	ctx := context.Background()
	inst, err := argus.New(ctx, nil)
	require.NoError(t, err)
	defer inst.Close()

	repoID := seedChatStreamRepo(t, inst)

	srv := NewRESTServer(inst)

	// Create a router with mock provider that errors
	router := createRouterWithMockProvider([]string{}, errors.New("streaming error"))
	srv.SetProvider(router)

	req := httptest.NewRequest("GET", "/api/chat/stream?repoID="+repoID+"&q=test", nil)
	rr := httptest.NewRecorder()

	srv.chatStreamHandler(rr, req)

	body := rr.Body.String()
	assert.Contains(t, body, "[ERROR]")
	assert.Contains(t, body, "streaming error")
}

func TestSetProvider(t *testing.T) {
	ctx := context.Background()
	inst, err := argus.New(ctx, nil)
	require.NoError(t, err)
	defer inst.Close()

	repoID := seedChatStreamRepo(t, inst)

	srv := NewRESTServer(inst)

	// Before SetProvider, provider should be nil
	assert.Nil(t, srv.provider)

	// Create and set a router with mock provider
	router := createRouterWithMockProvider([]string{"test"}, nil)
	srv.SetProvider(router)

	// After SetProvider, it should be set
	assert.NotNil(t, srv.provider)

	// Verify it works by making a request
	req := httptest.NewRequest("GET", "/api/chat/stream?repoID="+repoID+"&q=hello", nil)
	rr := httptest.NewRecorder()

	srv.chatStreamHandler(rr, req)

	// Should succeed (200-ish) not fail with 503
	assert.NotEqual(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), "data: ")
}

// TestMockProvider implements the providers.Provider interface for testing
type TestMockProvider struct {
	tokens []string
	err    error
	name   string
}

func (m *TestMockProvider) Chat(ctx context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return strings.Join(m.tokens, " "), nil
}

func (m *TestMockProvider) ChatStream(ctx context.Context, repoID string, prompt string) (<-chan string, <-chan error, error) {
	tokenCh := make(chan string, len(m.tokens))
	errCh := make(chan error)

	go func() {
		defer close(tokenCh)
		defer close(errCh)

		if m.err != nil {
			errCh <- m.err
			return
		}

		for _, token := range m.tokens {
			select {
			case <-ctx.Done():
				return
			case tokenCh <- token:
			}
		}
	}()

	return tokenCh, errCh, nil
}

func (m *TestMockProvider) Name() string {
	return m.name
}

// createRouterWithMockProvider creates a real Router with a registered mock provider
func createRouterWithMockProvider(tokens []string, err error) *providers.Router {
	cfg := &config.Config{
		LLMProvider: "test-mock",
	}
	router := providers.NewRouter(cfg)

	// Override the active provider with our mock
	mock := &TestMockProvider{
		tokens: tokens,
		err:    err,
		name:   "test-mock",
	}
	router.Register(mock)

	return router
}

func seedChatStreamRepo(t *testing.T, inst *argus.Instance) string {
	t.Helper()

	tmpRepo, repoCleanup := setupTestRepo(t)
	t.Cleanup(repoCleanup)

	_, err := inst.Analyze(context.Background(), tmpRepo)
	require.NoError(t, err)

	absPath, err := filepath.Abs(tmpRepo)
	require.NoError(t, err)
	repoID := fmt.Sprintf("%x", sha256.Sum256([]byte(absPath)))[:constants.RepoIDLength]

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := inst.GetRepoSymbols(context.Background(), repoID); err == nil {
			return repoID
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for repo to be indexed")
	return ""
}

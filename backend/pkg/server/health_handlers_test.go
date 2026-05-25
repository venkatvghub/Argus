package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/argus"
	"github.com/venkatvghub/argus/pkg/config"
)

func setupHealthTestServer(t *testing.T) (*RESTServer, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "argus-health-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		DBPath:             dbPath,
		LogLevel:           "error",
		AppName:            "ArgusTest",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}

	ctx := context.Background()
	instance, err := argus.New(ctx, cfg)
	require.NoError(t, err)

	restServer := NewRESTServer(instance)

	return restServer, func() {
		instance.Close()
		os.RemoveAll(tmpDir)
	}
}

// TestGetHealth_Endpoint tests the /health endpoint.
func TestGetHealth_Endpoint(t *testing.T) {
	restServer, cleanup := setupHealthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	restServer.Routes().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Response should be plain text or JSON
	assert.NotEmpty(t, w.Body.String())
}

// TestGetRepoHealth_Overview_NoEngine tests health overview when engine not loaded.
// Engine-not-in-memory returns a 200 with an empty summary rather than 404,
// so the dashboard degrades gracefully after a server restart.
func TestGetRepoHealth_Overview_NotFound(t *testing.T) {
	restServer, cleanup := setupHealthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/repos/nonexistent-repo/health/overview", nil)
	w := httptest.NewRecorder()

	restServer.Routes().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "summary")
}

// TestGetRepoHealth_Files_NoEngine tests health files when engine not loaded.
func TestGetRepoHealth_Files_NotFound(t *testing.T) {
	restServer, cleanup := setupHealthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/repos/nonexistent-repo/health/files", nil)
	w := httptest.NewRecorder()

	restServer.Routes().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "files")
}

// TestGetRepoHealth_Findings_NoEngine tests health findings when engine not loaded.
func TestGetRepoHealth_Findings_NotFound(t *testing.T) {
	restServer, cleanup := setupHealthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/repos/nonexistent-repo/health/findings", nil)
	w := httptest.NewRecorder()

	restServer.Routes().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestHealthEndpointHeaders tests response headers for health endpoints.
func TestHealthEndpointHeaders(t *testing.T) {
	restServer, cleanup := setupHealthTestServer(t)
	defer cleanup()

	tests := []struct {
		path string
		name string
	}{
		{"/health", "health"},
		{"/api/repos/test-repo/health/overview", "health overview"},
		{"/api/repos/test-repo/health/files", "health files"},
		{"/api/repos/test-repo/health/findings", "health findings"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", test.path, nil)
			req.Header.Set("Origin", "http://localhost:3000")
			w := httptest.NewRecorder()

			restServer.Routes().ServeHTTP(w, req)

			assert.True(t, w.Code >= 200 && w.Code < 500, "expected 2xx or 4xx, got %d", w.Code)
			assert.NotEmpty(t, w.Body.String())
			assert.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
			assert.Equal(t, "Origin", w.Header().Get("Vary"))
			assert.Equal(t, "GET, POST, DELETE, PATCH, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
			assert.Equal(t, "Accept, Content-Type, Cache-Control, Last-Event-ID", w.Header().Get("Access-Control-Allow-Headers"))
		})
	}
}

// TestHealthEndpointContentType tests Content-Type headers.
func TestHealthEndpointContentType(t *testing.T) {
	restServer, cleanup := setupHealthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	restServer.Routes().ServeHTTP(w, req)

	// Should have a content type (JSON or text); allow charset suffixes.
	contentType := w.Header().Get("Content-Type")
	assert.True(t,
		strings.HasPrefix(contentType, "application/json") || strings.HasPrefix(contentType, "text/plain"),
		"unexpected Content-Type: %q", contentType)
}

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
		DBPath:   dbPath,
		LogLevel: "error",
		AppName:  "ArgusTest",
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

// TestGetRepoHealth_Overview_NotFound tests health overview for non-existent repo.
func TestGetRepoHealth_Overview_NotFound(t *testing.T) {
	restServer, cleanup := setupHealthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/repos/nonexistent-repo/health/overview", nil)
	w := httptest.NewRecorder()

	restServer.Routes().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGetRepoHealth_Files_NotFound tests health files for non-existent repo.
func TestGetRepoHealth_Files_NotFound(t *testing.T) {
	restServer, cleanup := setupHealthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/repos/nonexistent-repo/health/files", nil)
	w := httptest.NewRecorder()

	restServer.Routes().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGetRepoHealth_Findings_NotFound tests health findings for non-existent repo.
func TestGetRepoHealth_Findings_NotFound(t *testing.T) {
	restServer, cleanup := setupHealthTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/repos/nonexistent-repo/health/findings", nil)
	w := httptest.NewRecorder()

	restServer.Routes().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
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

			// Health endpoints should return success or not found, not error
			assert.True(t, w.Code >= 200 && w.Code < 500, "expected 2xx or 4xx, got %d", w.Code)
			// With Origin header set and allowed, CORS headers may be present (depends on implementation)
			// Just verify response is valid
			assert.NotEmpty(t, w.Body.String())
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

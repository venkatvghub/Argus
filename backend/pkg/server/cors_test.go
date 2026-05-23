package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetCORSHeaders_AllowlistedOrigin(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.Header.Set("Origin", "https://app.example.com")

	setCORSHeaders(rr, req, []string{"https://app.example.com", "http://localhost:3000"})

	assert.Equal(t, "https://app.example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "Origin", rr.Header().Get("Vary"))
	assert.Equal(t, "GET, OPTIONS", rr.Header().Get("Access-Control-Allow-Methods"))
}

func TestSetCORSHeaders_UnknownOrigin(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.Header.Set("Origin", "https://evil.example.com")

	setCORSHeaders(rr, req, []string{"https://app.example.com"})

	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestSetCORSHeaders_NoOriginHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)

	setCORSHeaders(rr, req, []string{"https://app.example.com"})

	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

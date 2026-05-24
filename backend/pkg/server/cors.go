package server

import (
	"net/http"
	"strings"
)

// corsMiddleware returns a middleware that handles CORS for all routes,
// using the server's configured allowed origins.
func (s *RESTServer) corsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			allowed := false
			if origin != "" {
				for _, o := range s.corsAllowedOrigins() {
					if origin == strings.TrimSpace(o) {
						allowed = true
						break
					}
				}
			}
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, PATCH, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Cache-Control, Last-Event-ID")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func setSSEHeaders(w http.ResponseWriter, r *http.Request, allowedOrigins []string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// CORS is now handled by corsMiddleware; only set here as fallback when middleware is absent.
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		setCORSHeaders(w, r, allowedOrigins)
	}
}

func setCORSHeaders(w http.ResponseWriter, r *http.Request, allowedOrigins []string) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return
	}
	for _, allowed := range allowedOrigins {
		if origin == strings.TrimSpace(allowed) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Cache-Control, Last-Event-ID")
			return
		}
	}
}

func (s *RESTServer) corsAllowedOrigins() []string {
	if s.argus == nil {
		return nil
	}
	cfg := s.argus.Config()
	if cfg == nil {
		return nil
	}
	return cfg.CORSAllowedOrigins
}

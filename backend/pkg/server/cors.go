package server

import (
	"net/http"
	"strings"
)

func setSSEHeaders(w http.ResponseWriter, r *http.Request, allowedOrigins []string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	setCORSHeaders(w, r, allowedOrigins)
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

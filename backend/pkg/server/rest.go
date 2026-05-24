package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/venkatvghub/argus/pkg/argus"
	"github.com/venkatvghub/argus/pkg/constants"
	"github.com/venkatvghub/argus/pkg/providers"
)

// RESTServer handles HTTP requests for Argus.
type RESTServer struct {
	argus    *argus.Instance
	provider *providers.Router
}

// NewRESTServer creates a new RESTServer instance.
func NewRESTServer(argus *argus.Instance) *RESTServer {
	return &RESTServer{argus: argus}
}

// SetProvider wires the active LLM provider router for chat stream token-streaming endpoints.
func (s *RESTServer) SetProvider(p *providers.Router) {
	s.provider = p
}

// Routes initializes the chi router and returns it.
func (s *RESTServer) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		r.Get("/repos", s.listRepos)
		r.Post("/repos/index", s.indexRepo)
		r.Get("/repos/{repoID}/symbols", s.getSymbols)
		r.Get("/repos/{repoID}/markers", s.getMarkers)
		r.Get("/score/file", s.getFileScore)
		r.Get("/score/repo", s.getRepoScore)
		r.Get("/export/cognee", s.exportCognee)
		r.Get("/events", s.sseHandler)
		r.Get("/chat/stream", s.chatStreamHandler)
	})

	return r
}

func (s *RESTServer) listRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := s.argus.ListRepositories(r.Context())
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusOK, repos)
}

func (s *RESTServer) indexRepo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Path) == "" {
		s.error(w, http.StatusBadRequest, "path is required")
		return
	}
	if _, err := s.argus.Analyze(r.Context(), body.Path); err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusAccepted, map[string]string{"message": "analysis started"})
}

func (s *RESTServer) getSymbols(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repoID is required")
		return
	}
	symbols, err := s.argus.GetRepoSymbols(r.Context(), repoID)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusOK, symbols)
}

func (s *RESTServer) getMarkers(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	markers, err := s.argus.GetRepoMarkers(r.Context(), repoID)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusOK, markers)
}

func (s *RESTServer) getFileScore(w http.ResponseWriter, r *http.Request) {
	repoID := r.URL.Query().Get("repo_id")
	filePath := r.URL.Query().Get("path")
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repo_id is required")
		return
	}
	if filePath == "" {
		s.error(w, http.StatusBadRequest, "path is required")
		return
	}
	score, err := s.argus.GetFileScore(r.Context(), repoID, filePath)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, score)
}

func (s *RESTServer) getRepoScore(w http.ResponseWriter, r *http.Request) {
	repoID := r.URL.Query().Get("repo_id")
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repo_id is required")
		return
	}
	score, err := s.argus.GetRepoScore(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, map[string]float64{"score": score})
}

func (s *RESTServer) exportCognee(w http.ResponseWriter, r *http.Request) {
	// Cognee Export placeholder
	s.json(w, http.StatusOK, map[string]any{
		"entities":  []any{},
		"relations": []any{},
		"version":   constants.APIVersion,
	})
}

func (s *RESTServer) json(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

func (s *RESTServer) error(w http.ResponseWriter, code int, message string) {
	s.json(w, code, map[string]string{"error": message})
}

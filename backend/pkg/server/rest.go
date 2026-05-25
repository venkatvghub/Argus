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
	r.Use(s.corsMiddleware())

	r.Get("/health", s.healthCheck)

	r.Route("/api", func(r chi.Router) {
		// Repos — literal paths before parameterized ones to avoid capture.
		r.Get("/repos", s.listRepos)
		r.Post("/repos", s.createRepo)
		r.Post("/repos/index", s.indexRepo)
		r.Get("/repos/{repoID}", s.getRepo)
		r.Delete("/repos/{repoID}", s.deleteRepo)
		r.Get("/repos/{repoID}/stats", s.getRepoStats)
		r.Post("/repos/{repoID}/sync", s.syncRepo)
		r.Get("/repos/{repoID}/symbols", s.getSymbols)
		r.Get("/repos/{repoID}/markers", s.getMarkers)

		// Git intelligence
		r.Get("/repos/{repoID}/git-summary", s.getGitSummary)
		r.Get("/repos/{repoID}/ownership", s.getOwnership)
		r.Get("/repos/{repoID}/hotspots", s.getHotspots)

		// Dead code
		r.Get("/repos/{repoID}/dead-code/summary", s.getDeadCodeSummary)
		r.Get("/repos/{repoID}/dead-code", s.getDeadCode)

		// Decisions / ADR
		r.Get("/repos/{repoID}/decisions", s.getDecisions)
		r.Get("/repos/{repoID}/decisions/health", s.getDecisionsHealth)

		// Knowledge map
		r.Get("/repos/{repoID}/knowledge-map", s.getKnowledgeMap)

		// Security findings (stub — PII markers served from health/findings)
		r.Get("/repos/{repoID}/security", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, []any{})
		})

		// Health
		r.Get("/repos/{repoID}/health/overview", s.getHealthOverview)
		r.Get("/repos/{repoID}/health/files", s.getHealthFiles)
		r.Get("/repos/{repoID}/health/findings", s.getHealthFindings)
		r.Get("/repos/{repoID}/health/files/breakdown", s.getHealthFiles) // same as getHealthFiles
		r.Get("/repos/{repoID}/health/trend", s.getHealthTrend)
		r.Get("/repos/{repoID}/health/coverage", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{
				"summary": map[string]any{
					"file_count":           0,
					"covered_lines":        0,
					"total_lines":          0,
					"line_coverage_pct":    nil,
					"branch_coverage_pct":  nil,
					"source_format":        nil,
					"ingested_at":          nil,
					"ingested_commit_sha":  nil,
				},
				"files":   []any{},
				"modules": []any{},
			})
		})
		r.Get("/repos/{repoID}/health/refactoring-targets", s.getRefactoringTargets)
		r.Get("/repos/{repoID}/health/coordinator", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{
				"status":      "ok",
				"sql_pages":   nil,
				"vector_count": nil,
				"graph_nodes":  nil,
				"drift_pct":    nil,
			})
		})

		// Providers
		r.Get("/providers", s.getProviders)

		// Graph
		r.Get("/graph/{repoID}", s.getGraphExport)
		r.Get("/graph/{repoID}/architecture", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{"nodes": []any{}, "edges": []any{}})
		})
		r.Get("/graph/{repoID}/communities", s.getCommunities)
		r.Get("/graph/{repoID}/path", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{"path": []any{}})
		})
		r.Get("/graph/{repoID}/ego", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{"nodes": []any{}, "edges": []any{}})
		})
		r.Get("/graph/{repoID}/nodes/search", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{"results": []any{}})
		})
		r.Get("/graph/{repoID}/dead-nodes", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{"findings": []any{}})
		})
		r.Get("/graph/{repoID}/hot-files", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{"files": []any{}})
		})
		r.Get("/graph/{repoID}/modules", s.getGraphModules)
		r.Get("/graph/{repoID}/execution-flows", s.getExecutionFlows)

		// C4 diagram stubs
		r.Get("/graph/{repoID}/c4/l1", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{
				"system":           map[string]any{"id": "system", "name": "System", "description": ""},
				"people":           []any{},
				"external_systems": []any{},
				"relations":        []any{},
			})
		})
		r.Get("/graph/{repoID}/c4/l2", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{
				"containers":       []any{},
				"external_systems": []any{},
				"relations":        []any{},
			})
		})
		r.Get("/graph/{repoID}/c4/l3", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{
				"container":        nil,
				"components":       []any{},
				"external_systems": []any{},
				"relations":        []any{},
			})
		})
		r.Get("/graph/{repoID}/c4/mermaid", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("C4Context\n  title System Context"))
		})

		// Jobs
		r.Get("/jobs", s.listJobs)
		r.Get("/jobs/{jobID}", s.getJob)
		r.Post("/jobs/{jobID}/cancel", s.cancelJob)
		r.Get("/jobs/{jobID}/stream", s.jobStreamHandler)

		// Chat / Conversations
		r.Get("/repos/{repoID}/chat/conversations", s.listConversations)
		r.Get("/repos/{repoID}/chat/conversations/{convID}", s.getConversation)
		r.Delete("/repos/{repoID}/chat/conversations/{convID}", s.deleteConversation)
		r.Post("/repos/{repoID}/chat/messages", s.postChatMessage)

		// Symbols — flat query-param style used by the frontend (/api/symbols?repo_id=...)
		r.Get("/symbols", s.listSymbols)
		r.Get("/symbols/by-name/{name}", s.listSymbols)
		r.Get("/symbols/{symbolID}", s.listSymbols)

		// Wiki pages — flat query-param style (/api/pages?repo_id=...)
		r.Get("/pages", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, []any{})
		})
		r.Get("/pages/lookup", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{})
		})
		r.Get("/pages/lookup/versions", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, []any{})
		})
		r.Post("/pages/lookup/regenerate", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{"job_id": ""})
		})

		// Module health stubs
		r.Get("/repos/{repoID}/modules/health", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{
				"items": []any{}, "total": 0, "has_more": false, "next_offset": nil,
			})
		})
		r.Get("/repos/{repoID}/modules/health/{modulePath}", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{})
		})

		// LLM cost tracking
		r.Get("/repos/{repoID}/costs", s.getRepoCosts)
		r.Get("/repos/{repoID}/costs/summary", s.getRepoCostSummary)

		// Owners stubs
		r.Get("/repos/{repoID}/owners", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{
				"items": []any{}, "total": 0, "has_more": false, "next_offset": nil,
			})
		})
		r.Get("/repos/{repoID}/owners/{ownerKey}", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{})
		})

		// Score & export (existing)
		r.Get("/score/file", s.getFileScore)
		r.Get("/score/repo", s.getRepoScore)
		r.Get("/export/cognee", s.exportCognee)

		// SSE & chat stream (existing — kept for compat)
		r.Get("/events", s.sseHandler)
		r.Get("/chat/stream", s.chatStreamHandler)

		// Workspace
		r.Get("/workspace", s.getWorkspace)
	})

	return r
}

func (s *RESTServer) healthCheck(w http.ResponseWriter, r *http.Request) {
	s.json(w, http.StatusOK, map[string]string{"status": "ok", "version": constants.APIVersion})
}

func (s *RESTServer) stubEmptyData(w http.ResponseWriter, r *http.Request) {
	s.json(w, http.StatusOK, map[string]any{"data": []any{}})
}

func (s *RESTServer) listRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := s.argus.ListRepositories(r.Context())
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]RepoResponse, 0, len(repos))
	for _, r := range repos {
		out = append(out, repoToResponse(r))
	}
	s.json(w, http.StatusOK, out)
}

func (s *RESTServer) createRepo(w http.ResponseWriter, r *http.Request) {
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
	jobID, err := s.argus.Analyze(r.Context(), body.Path)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusAccepted, map[string]string{"job_id": jobID, "message": "analysis started"})
}

func (s *RESTServer) getRepo(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	repo, err := s.argus.GetRepository(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, repoToResponse(repo))
}

func (s *RESTServer) deleteRepo(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	if err := s.argus.DeleteRepository(r.Context(), repoID); err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *RESTServer) getRepoStats(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	stats, err := s.argus.GetRepoStats(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			// Engine not yet loaded; return empty stats so the dashboard doesn't 404.
			s.json(w, http.StatusOK, RepoStatsResponse{})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	// Map the generic map to the typed API shape.
	fileCount, _ := stats["file_count"].(int)
	symCount, _ := stats["total_symbols"].(int)
	s.json(w, http.StatusOK, RepoStatsResponse{
		FileCount:   fileCount,
		SymbolCount: symCount,
	})
}

func (s *RESTServer) syncRepo(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	repo, err := s.argus.GetRepository(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	jobID, err := s.argus.Analyze(r.Context(), repo.Path)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusAccepted, map[string]string{"job_id": jobID, "message": "sync started"})
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
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.json(w, http.StatusOK, []any{})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, symbols)
}

// listSymbols handles GET /api/symbols?repo_id=... with the paginated envelope
// shape the frontend expects: { items, total, has_more, next_offset }.
func (s *RESTServer) listSymbols(w http.ResponseWriter, r *http.Request) {
	repoID := r.URL.Query().Get("repo_id")
	if repoID == "" {
		s.json(w, http.StatusOK, map[string]any{
			"items": []any{}, "total": 0, "has_more": false, "next_offset": nil,
		})
		return
	}
	symbols, err := s.argus.GetRepoSymbols(r.Context(), repoID)
	if err != nil {
		s.json(w, http.StatusOK, map[string]any{
			"items": []any{}, "total": 0, "has_more": false, "next_offset": nil,
		})
		return
	}
	s.json(w, http.StatusOK, map[string]any{
		"items": symbols, "total": len(symbols), "has_more": false, "next_offset": nil,
	})
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

func (s *RESTServer) getHealthOverview(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	overview, err := s.argus.GetHealthOverview(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.json(w, http.StatusOK, HealthOverviewResponse{
				Summary:     HealthOverviewSummary{SeverityBreakdown: map[string]int{}},
				Files:       []HealthFileMetric{},
				TopFindings: []HealthFinding{},
			})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Get top findings for the overview (capped at 10).
	rawFindings, _ := s.argus.GetHealthFindings(r.Context(), repoID)
	topFindings := make([]HealthFinding, 0, len(rawFindings))
	for _, f := range rawFindings {
		topFindings = append(topFindings, argusHealthFindingToAPI(f))
		if len(topFindings) >= 10 {
			break
		}
	}

	s.json(w, http.StatusOK, HealthOverviewResponse{
		Summary: HealthOverviewSummary{
			FileCount:     overview.FileCount,
			AverageHealth: overview.OverallScore,
			OpenFindings:  overview.FindingCount,
			SeverityBreakdown: map[string]int{
				"critical": overview.CriticalCount,
				"warning":  overview.WarningCount,
				"info":     overview.InfoCount,
			},
		},
		Files:       []HealthFileMetric{},
		TopFindings: topFindings,
	})
}

func (s *RESTServer) getHealthFiles(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	files, err := s.argus.GetHealthFiles(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			// Engine not loaded; return empty paginated response.
			s.json(w, http.StatusOK, HealthFilesResponse{Files: []HealthFileMetric{}})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	metrics := make([]HealthFileMetric, 0, len(files))
	for _, f := range files {
		metrics = append(metrics, HealthFileMetric{
			FilePath:    f.Path,
			Score:       f.Score,
			HasTestFile: f.HasTestFile,
		})
	}
	s.json(w, http.StatusOK, HealthFilesResponse{
		Total:  len(metrics),
		Offset: 0,
		Limit:  len(metrics),
		Files:  metrics,
	})
}

func (s *RESTServer) getHealthFindings(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	findings, err := s.argus.GetHealthFindings(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.json(w, http.StatusOK, []HealthFinding{})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	out := make([]HealthFinding, 0, len(findings))
	for _, f := range findings {
		out = append(out, argusHealthFindingToAPI(f))
	}
	s.json(w, http.StatusOK, out)
}

func (s *RESTServer) getGraphExport(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	export, err := s.argus.GetGraphExport(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.json(w, http.StatusOK, map[string]any{"repo_id": repoID, "nodes": []any{}, "edges": []any{}})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, export)
}

func (s *RESTServer) getCommunities(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	communities, err := s.argus.GetCommunities(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.json(w, http.StatusOK, []any{})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, communities)
}

func (s *RESTServer) listJobs(w http.ResponseWriter, r *http.Request) {
	repoID := r.URL.Query().Get("repoId")
	jobs, err := s.argus.ListJobs(r.Context(), repoID)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]JobResponse, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, jobToResponse(j))
	}
	s.json(w, http.StatusOK, out)
}

func (s *RESTServer) getJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	job, err := s.argus.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, argus.ErrJobNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, jobToResponse(job))
}

func (s *RESTServer) cancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	if err := s.argus.CancelJob(r.Context(), jobID); err != nil {
		s.error(w, http.StatusBadRequest, err.Error())
		return
	}
	s.json(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *RESTServer) jobStreamHandler(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	// Clone the request before modifying the URL so middleware/tracing sees the original.
	r2 := r.Clone(r.Context())
	q := r2.URL.Query()
	q.Set("jobId", jobID)
	r2.URL.RawQuery = q.Encode()
	s.sseHandler(w, r2)
}

func (s *RESTServer) listConversations(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	convs, err := s.argus.ListConversations(r.Context(), repoID)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusOK, convs)
}

func (s *RESTServer) getConversation(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	convID := chi.URLParam(r, "convID")
	conv, err := s.argus.GetConversation(r.Context(), convID)
	if err != nil {
		if errors.Is(err, argus.ErrConversationNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if conv.RepositoryID != repoID {
		s.error(w, http.StatusNotFound, "conversation not found")
		return
	}
	msgs, err := s.argus.ListChatMessages(r.Context(), convID)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusOK, map[string]any{
		"conversation": conv,
		"messages":     msgs,
	})
}

func (s *RESTServer) deleteConversation(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "convID")
	if err := s.argus.DeleteConversation(r.Context(), convID); err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *RESTServer) getWorkspace(w http.ResponseWriter, r *http.Request) {
	ws, err := s.argus.GetWorkspace(r.Context())
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusOK, ws)
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
	s.json(w, http.StatusOK, map[string]any{
		"entities":  []any{},
		"relations": []any{},
		"version":   constants.APIVersion,
	})
}

// --- Git intelligence ---

func (s *RESTServer) getGitSummary(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repoID is required")
		return
	}
	files, err := s.argus.GetRepoFiles(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	const hotspotChurnThreshold = 10
	hotspotCount, stableCount := 0, 0
	ownerMap := make(map[string]int)
	var totalChurn int
	for _, f := range files {
		if f.Churn >= hotspotChurnThreshold {
			hotspotCount++
		} else {
			stableCount++
		}
		totalChurn += f.Churn
	}

	// Aggregate ownership by extracting top contributor info from markers.
	markers, _ := s.argus.GetRepoMarkers(r.Context(), repoID)
	for _, m := range markers {
		if m.Type == "knowledge_loss" || m.Type == "developer_congestion" {
			ownerMap[m.File]++
		}
	}

	avgChurn := 0.0
	if len(files) > 0 {
		avgChurn = float64(totalChurn) / float64(len(files))
	}

	s.json(w, http.StatusOK, map[string]any{
		"total_files":              len(files),
		"hotspot_count":            hotspotCount,
		"stable_count":             stableCount,
		"average_churn_percentile": avgChurn,
		"top_owners":               []any{},
	})
}

func (s *RESTServer) getOwnership(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repoID is required")
		return
	}
	files, err := s.argus.GetRepoFiles(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	type ownershipItem struct {
		Path        string  `json:"path"`
		Language    string  `json:"language"`
		Ownership   float64 `json:"ownership_pct"`
		AuthorCount int     `json:"author_count"`
		Churn       int     `json:"churn"`
	}

	items := make([]ownershipItem, 0, len(files))
	for _, f := range files {
		items = append(items, ownershipItem{
			Path:        f.Path,
			Language:    f.Language,
			Ownership:   f.Ownership,
			AuthorCount: f.AuthorCount,
			Churn:       f.Churn,
		})
	}

	s.json(w, http.StatusOK, map[string]any{
		"items":       items,
		"total":       len(items),
		"has_more":    false,
		"next_offset": nil,
	})
}

func (s *RESTServer) getHotspots(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repoID is required")
		return
	}
	files, err := s.argus.GetRepoFiles(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	type hotspotItem struct {
		Path        string  `json:"path"`
		Language    string  `json:"language"`
		Churn       int     `json:"churn"`
		Ownership   float64 `json:"ownership_pct"`
		AuthorCount int     `json:"author_count"`
	}

	const hotspotThreshold = 5
	items := make([]hotspotItem, 0)
	for _, f := range files {
		if f.Churn >= hotspotThreshold {
			items = append(items, hotspotItem{
				Path:        f.Path,
				Language:    f.Language,
				Churn:       f.Churn,
				Ownership:   f.Ownership,
				AuthorCount: f.AuthorCount,
			})
		}
	}

	s.json(w, http.StatusOK, map[string]any{
		"items":       items,
		"total":       len(items),
		"has_more":    false,
		"next_offset": nil,
	})
}

// --- Dead code stubs ---

func (s *RESTServer) getDeadCodeSummary(w http.ResponseWriter, r *http.Request) {
	repoID := strings.TrimSpace(chi.URLParam(r, "repoID"))
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repo_id is required")
		return
	}
	if _, err := s.argus.GetRepository(r.Context(), repoID); err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, map[string]any{
		"total_findings":     0,
		"confidence_summary": map[string]int{},
		"deletable_lines":    0,
		"total_lines":        0,
		"by_kind":            map[string]int{},
	})
}

func (s *RESTServer) getDeadCode(w http.ResponseWriter, r *http.Request) {
	repoID := strings.TrimSpace(chi.URLParam(r, "repoID"))
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repo_id is required")
		return
	}
	if _, err := s.argus.GetRepository(r.Context(), repoID); err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, []any{})
}

// --- Decision / ADR stubs ---

func (s *RESTServer) getDecisions(w http.ResponseWriter, r *http.Request) {
	repoID := strings.TrimSpace(chi.URLParam(r, "repoID"))
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repo_id is required")
		return
	}
	if _, err := s.argus.GetRepository(r.Context(), repoID); err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, []any{})
}

func (s *RESTServer) getDecisionsHealth(w http.ResponseWriter, r *http.Request) {
	repoID := strings.TrimSpace(chi.URLParam(r, "repoID"))
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repo_id is required")
		return
	}
	if _, err := s.argus.GetRepository(r.Context(), repoID); err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, map[string]any{
		"summary": map[string]int{
			"active":     0,
			"proposed":   0,
			"deprecated": 0,
			"superseded": 0,
			"stale":      0,
		},
		"stale_decisions":          []any{},
		"proposed_awaiting_review": []any{},
		"ungoverned_hotspots":      []any{},
	})
}

// --- Knowledge map stub ---

func (s *RESTServer) getKnowledgeMap(w http.ResponseWriter, r *http.Request) {
	repoID := strings.TrimSpace(chi.URLParam(r, "repoID"))
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repo_id is required")
		return
	}
	if _, err := s.argus.GetRepository(r.Context(), repoID); err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, map[string]any{
		"top_owners":         []any{},
		"knowledge_silos":    []any{},
		"onboarding_targets": []any{},
	})
}

// --- Providers stub ---

func (s *RESTServer) getProviders(w http.ResponseWriter, r *http.Request) {
	active := map[string]string{"provider": "", "model": ""}
	if s.argus != nil {
		if cfg := s.argus.Config(); cfg != nil {
			active["provider"] = cfg.LLMProvider
		}
	}
	s.json(w, http.StatusOK, map[string]any{
		"active":    active,
		"providers": []any{},
	})
}

// --- Graph extension stubs ---

func (s *RESTServer) getGraphModules(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	if _, err := s.argus.GetCommunities(r.Context(), repoID); err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.json(w, http.StatusOK, map[string]any{"nodes": []any{}, "edges": []any{}})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, map[string]any{
		"nodes": []any{},
		"edges": []any{},
	})
}

func (s *RESTServer) getExecutionFlows(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	if _, err := s.argus.GetCommunities(r.Context(), repoID); err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.json(w, http.StatusOK, map[string]any{
				"total_entry_points": 0,
				"flows":              []any{},
			})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, map[string]any{
		"total_entry_points": 0,
		"flows":              []any{},
	})
}

func (s *RESTServer) getHealthTrend(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	score, err := s.argus.GetRepoScore(r.Context(), repoID)
	if err != nil {
		score = 0
	}
	s.json(w, http.StatusOK, HealthTrendResponse{
		History:    []any{},
		Alerts:     []any{},
		FileDeltas: []any{},
		Summary: HealthTrendSummary{
			CurrentAverageHealth: score,
		},
		SnapshotCount: 0,
	})
}

func (s *RESTServer) getRepoCosts(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	by := r.URL.Query().Get("by")
	if by == "" {
		by = "day"
	}
	groups, err := s.argus.GetRepoCosts(r.Context(), repoID, by)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, map[string]any{
		"items":   groups,
		"total":   len(groups),
		"has_more": false,
	})
}

func (s *RESTServer) getRepoCostSummary(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	summary, err := s.argus.GetRepoCostSummary(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, summary)
}

func (s *RESTServer) getRefactoringTargets(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	markers, err := s.argus.GetRepoMarkers(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.json(w, http.StatusOK, map[string]any{"targets": []any{}, "total": 0})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	type refactorTarget struct {
		File       string  `json:"file"`
		Type       string  `json:"type"`
		Severity   string  `json:"severity"`
		Message    string  `json:"message"`
		Line       int     `json:"line"`
		Deduction  float64 `json:"deduction"`
		Suggestion string  `json:"suggestion"`
	}

	refactorCategories := map[string]struct{}{
		"structural_complexity": {},
		"size_api_complexity":   {},
		"duplication":           {},
	}

	targets := make([]refactorTarget, 0)
	for _, m := range markers {
		if _, ok := refactorCategories[string(m.Category)]; ok && m.Deduction > 0 {
			targets = append(targets, refactorTarget{
				File:       m.File,
				Type:       m.Type,
				Severity:   m.Severity,
				Message:    m.Message,
				Line:       m.Line,
				Deduction:  m.Deduction,
				Suggestion: m.Suggestion,
			})
		}
	}

	// Sort by deduction descending (highest impact first).
	for i := 1; i < len(targets); i++ {
		for j := 0; j < len(targets)-i; j++ {
			if targets[j].Deduction < targets[j+1].Deduction {
				targets[j], targets[j+1] = targets[j+1], targets[j]
			}
		}
	}

	s.json(w, http.StatusOK, map[string]any{
		"targets": targets,
		"total":   len(targets),
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

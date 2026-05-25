package server

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/venkatvghub/argus/pkg/argus"
	"github.com/venkatvghub/argus/pkg/constants"
	"github.com/venkatvghub/argus/pkg/models"
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
		r.Get("/repos/{repoID}/git-metadata", s.getGitMetadata)
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

		// Security findings — PII and AppSec markers from the marker store.
		r.Get("/repos/{repoID}/security", s.getSecurityFindings)

		// Health
		r.Get("/repos/{repoID}/health/overview", s.getHealthOverview)
		r.Get("/repos/{repoID}/health/files", s.getHealthFiles)
		r.Get("/repos/{repoID}/health/findings", s.getHealthFindings)
		r.Get("/repos/{repoID}/health/files/breakdown", s.getHealthFileBreakdown)
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
		r.Get("/graph/{repoID}/entry-points", s.getGraphExport) // alias used by architecture view
		r.Get("/graph/{repoID}/execution-flows", s.getExecutionFlows)

		// C4 diagram — derived from graph/module data
		r.Get("/graph/{repoID}/c4/l1", s.getC4L1)
		r.Get("/graph/{repoID}/c4/l2", s.getC4L2)
		r.Get("/graph/{repoID}/c4/l3", s.getC4L3)
		r.Get("/graph/{repoID}/c4/mermaid", s.getC4Mermaid)

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
		r.Get("/pages", s.listPages)
		r.Get("/pages/lookup", s.getPageByID)
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

		// Owners
		r.Get("/repos/{repoID}/owners", s.listOwners)
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

// listSymbols handles GET /api/symbols?repo_id=... with filtering, sorting, and pagination.
func (s *RESTServer) listSymbols(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	repoID := q.Get("repo_id")
	if repoID == "" {
		s.json(w, http.StatusOK, map[string]any{
			"items": []any{}, "total": 0, "has_more": false, "next_offset": nil,
		})
		return
	}

	filterQ := strings.ToLower(q.Get("q"))
	filterKind := q.Get("kind")
	filterLang := strings.ToLower(q.Get("language"))
	filterVis := q.Get("visibility")
	inHotFiles := q.Get("in_hot_files") == "true"
	sortBy := q.Get("sort")
	limit := 50
	offset := 0
	if v := q.Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit) //nolint:errcheck
	}
	if v := q.Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset) //nolint:errcheck
	}

	symbols, err := s.argus.GetRepoSymbols(r.Context(), repoID)
	if err != nil {
		s.json(w, http.StatusOK, map[string]any{
			"items": []any{}, "total": 0, "has_more": false, "next_offset": nil,
		})
		return
	}

	// Build file-level lookup: path → {language, churn}.
	files, _ := s.argus.GetRepoFiles(r.Context(), repoID)
	type fileMeta struct {
		language string
		churn    int
	}
	fileLookup := make(map[string]fileMeta, len(files))
	for _, f := range files {
		fileLookup[f.Path] = fileMeta{language: strings.ToLower(f.Language), churn: f.Churn}
	}

	const hotspotChurn = 10

	type symOut struct {
		ID                  string  `json:"id"`
		RepositoryID        string  `json:"repository_id"`
		FilePath            string  `json:"file_path"`
		SymbolID            string  `json:"symbol_id"`
		Name                string  `json:"name"`
		QualifiedName       string  `json:"qualified_name"`
		Kind                string  `json:"kind"`
		Signature           string  `json:"signature"`
		StartLine           int     `json:"start_line"`
		EndLine             int     `json:"end_line"`
		Docstring           *string `json:"docstring"`
		Visibility          string  `json:"visibility"`
		IsAsync             bool    `json:"is_async"`
		ComplexityEstimate  int     `json:"complexity_estimate"`
		Language            string  `json:"language"`
		ParentName          *string `json:"parent_name"`
		ImportanceScore     *float64 `json:"importance_score"`
		FilePagerank        *float64 `json:"file_pagerank"`
		IsEntryPoint        *bool   `json:"is_entry_point"`
		FileChurnPercentile *float64 `json:"file_churn_percentile"`
		FileIsHotspot       *bool   `json:"file_is_hotspot"`
	}

	// Compute churn percentile across all files.
	var churns []int
	for _, f := range files {
		churns = append(churns, f.Churn)
	}
	churnPercentile := func(c int) float64 {
		if len(churns) == 0 {
			return 0
		}
		rank := 0
		for _, v := range churns {
			if v <= c {
				rank++
			}
		}
		return float64(rank) / float64(len(churns)) * 100
	}

	seen := make(map[string]struct{})
	var out []symOut
	for _, sym := range symbols {
		fm := fileLookup[sym.FilePath]
		kind := string(sym.Type)
		lang := fm.language

		// Apply filters.
		if filterQ != "" && !strings.Contains(strings.ToLower(sym.Name), filterQ) {
			continue
		}
		if filterKind != "" && filterKind != "all" && kind != filterKind {
			continue
		}
		if filterLang != "" && filterLang != "all" && lang != filterLang {
			continue
		}
		if filterVis != "" && filterVis != "all" {
			if filterVis != "public" {
				continue
			}
		}
		isHotspot := fm.churn >= hotspotChurn
		if inHotFiles && !isHotspot {
			continue
		}

		id := fmt.Sprintf("%s:%s:%s:%d:%d", repoID, sym.FilePath, sym.Name, sym.Line, sym.EndLine)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}

		pct := churnPercentile(fm.churn)
		hotBool := isHotspot
		h := symOut{
			ID:                  id,
			RepositoryID:        repoID,
			FilePath:            sym.FilePath,
			SymbolID:            id,
			Name:                sym.Name,
			QualifiedName:       sym.Name,
			Kind:                kind,
			Signature:           sym.Name,
			StartLine:           sym.Line,
			EndLine:             sym.EndLine,
			Visibility:          "public",
			Language:            lang,
			FileChurnPercentile: &pct,
			FileIsHotspot:       &hotBool,
		}
		out = append(out, h)
	}

	// Sort.
	switch sortBy {
	case "name":
		for i := 1; i < len(out); i++ {
			for j := i; j > 0 && out[j].Name < out[j-1].Name; j-- {
				out[j], out[j-1] = out[j-1], out[j]
			}
		}
	default: // "importance" — sort by hotspot descending, then name
		for i := 1; i < len(out); i++ {
			a, b := out[i], out[i-1]
			aHot := a.FileIsHotspot != nil && *a.FileIsHotspot
			bHot := b.FileIsHotspot != nil && *b.FileIsHotspot
			if aHot && !bHot {
				out[i], out[i-1] = out[i-1], out[i]
			}
		}
	}

	total := len(out)
	if out == nil {
		out = []symOut{}
	}
	if offset >= total {
		s.json(w, http.StatusOK, map[string]any{
			"items": []symOut{}, "total": total, "has_more": false, "next_offset": nil,
		})
		return
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := out[offset:end]
	hasMore := end < total
	var nextOffset *int
	if hasMore {
		n := end
		nextOffset = &n
	}
	s.json(w, http.StatusOK, map[string]any{
		"items": page, "total": total, "has_more": hasMore, "next_offset": nextOffset,
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

// securityMarkerTypes is the set of marker type prefixes that surface as security findings.
var securityMarkerTypes = map[string]bool{
	"dpdp_pii_exposure":    true,
	"dpdp_mobile_exposure": true,
	"pii_email_exposure":   true,
	"pii_mobile_exposure":  true,
	"rbi_logging":          true,
	"data_sovereignty":     true,
	"untracked_consent":    true,
	"sql_injection":        true,
	"broken_crypto":        true,
	"hardcoded_secret":     true,
}

type securityFinding struct {
	ID         int    `json:"id"`
	FilePath   string `json:"file_path"`
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Snippet    string `json:"snippet"`
	DetectedAt string `json:"detected_at"`
}

func (s *RESTServer) getSecurityFindings(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	markers, err := s.argus.GetRepoMarkers(r.Context(), repoID)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]securityFinding, 0)
	id := 1
	for _, m := range markers {
		if !securityMarkerTypes[m.Type] {
			continue
		}
		sev := m.Severity
		if sev == "medium" {
			sev = "med"
		}
		out = append(out, securityFinding{
			ID:         id,
			FilePath:   m.File,
			Kind:       m.Type,
			Severity:   sev,
			Snippet:    m.Message,
			DetectedAt: now,
		})
		id++
	}
	s.json(w, http.StatusOK, out)
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

func (s *RESTServer) getHealthFileBreakdown(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	filePath := strings.TrimSpace(r.URL.Query().Get("file_path"))
	if filePath == "" {
		s.error(w, http.StatusBadRequest, "file_path is required")
		return
	}

	score, scoreErr := s.argus.GetFileScore(r.Context(), repoID, filePath)
	if scoreErr != nil {
		if errors.Is(scoreErr, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, scoreErr.Error())
		} else {
			s.error(w, http.StatusInternalServerError, scoreErr.Error())
		}
		return
	}

	allMarkers, _ := s.argus.GetRepoMarkers(r.Context(), repoID)
	var fileMarkers []models.Marker
	for _, m := range allMarkers {
		if filepath.ToSlash(m.File) == filepath.ToSlash(filePath) {
			fileMarkers = append(fileMarkers, m)
		}
	}

	// Group raw deductions by category.
	type catFinding struct {
		id        string
		kind      string
		severity  string
		rawImpact float64
		reason    string
		line      int
	}
	type catAcc struct {
		rawDeduction float64
		findings     []catFinding
	}
	cats := make(map[string]*catAcc)
	for _, m := range fileMarkers {
		cat := string(m.Category)
		if cat == "" {
			cat = "other"
		}
		acc := cats[cat]
		if acc == nil {
			acc = &catAcc{}
			cats[cat] = acc
		}
		acc.rawDeduction += m.Deduction
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(m.File+m.Type+fmt.Sprint(m.Line))))[:12]
		acc.findings = append(acc.findings, catFinding{
			id: id, kind: m.Type, severity: m.Severity,
			rawImpact: m.Deduction, reason: m.Message, line: m.Line,
		})
	}

	type apiCatFinding struct {
		ID            string  `json:"id"`
		BiomarkerType string  `json:"biomarker_type"`
		Severity      string  `json:"severity"`
		RawImpact     float64 `json:"raw_impact"`
		AppliedImpact float64 `json:"applied_impact"`
		FunctionName  *string `json:"function_name"`
		Reason        string  `json:"reason"`
	}
	type apiCategory struct {
		Category        string          `json:"category"`
		Cap             float64         `json:"cap"`
		RawDeduction    float64         `json:"raw_deduction"`
		AppliedDeduction float64        `json:"applied_deduction"`
		Capped          bool            `json:"capped"`
		FindingCount    int             `json:"finding_count"`
		Findings        []apiCatFinding `json:"findings"`
	}

	categories := make([]apiCategory, 0, len(cats))
	totalDeduction := 0.0
	for catName, acc := range cats {
		cap := models.CategoryCaps[models.ScoreCategory(catName)]
		applied := score.Deductions[models.ScoreCategory(catName)]
		totalDeduction += applied
		capped := cap > 0 && acc.rawDeduction > cap
		apiFindings := make([]apiCatFinding, 0, len(acc.findings))
		for _, f := range acc.findings {
			apiFindings = append(apiFindings, apiCatFinding{
				ID:            f.id,
				BiomarkerType: f.kind,
				Severity:      f.severity,
				RawImpact:     f.rawImpact,
				AppliedImpact: f.rawImpact, // individual impact (cap applies to category total)
				FunctionName:  nil,
				Reason:        f.reason,
			})
		}
		categories = append(categories, apiCategory{
			Category:         catName,
			Cap:              cap,
			RawDeduction:     acc.rawDeduction,
			AppliedDeduction: applied,
			Capped:           capped,
			FindingCount:     len(acc.findings),
			Findings:         apiFindings,
		})
	}

	// Build metric sub-object.
	module := filepath.Dir(filePath)
	metric := &HealthFileMetric{
		FilePath:    filePath,
		Score:       score.Final,
		HasTestFile: false,
	}

	// Build findings list for the sidebar.
	findings := make([]HealthFinding, 0, len(fileMarkers))
	for _, m := range fileMarkers {
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(m.File+m.Type+fmt.Sprint(m.Line))))[:12]
		var lineStart *int
		if m.Line > 0 {
			l := m.Line
			lineStart = &l
		}
		findings = append(findings, HealthFinding{
			ID:            id,
			FilePath:      m.File,
			BiomarkerType: m.Type,
			Severity:      m.Severity,
			FunctionName:  nil,
			LineStart:     lineStart,
			LineEnd:       nil,
			HealthImpact:  m.Deduction,
			Reason:        m.Message,
			Details:       map[string]any{},
			Status:        "open",
		})
	}
	_ = module

	s.json(w, http.StatusOK, map[string]any{
		"file_path": filePath,
		"metric":    metric,
		"breakdown": map[string]any{
			"score":           score.Final,
			"total_deduction": totalDeduction,
			"categories":      categories,
		},
		"findings":    findings,
		"suggestions": map[string]any{},
	})
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

func (s *RESTServer) getGitMetadata(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repoID is required")
		return
	}
	filePath := strings.TrimSpace(r.URL.Query().Get("file_path"))
	if filePath == "" {
		s.error(w, http.StatusBadRequest, "file_path is required")
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

	var target *struct {
		churn       int
		ownership   float64
		authorCount int
	}
	const hotspotChurnThreshold = 10
	totalChurn := 0
	for _, f := range files {
		totalChurn += f.Churn
		if filepath.ToSlash(f.Path) == filepath.ToSlash(filePath) {
			c := f.Churn
			o := f.Ownership
			a := f.AuthorCount
			target = &struct {
				churn       int
				ownership   float64
				authorCount int
			}{c, o, a}
		}
	}
	if target == nil {
		s.error(w, http.StatusNotFound, "file not found in index")
		return
	}

	// Compute churn percentile by ranking this file against all indexed files.
	churnPct := 0.0
	if len(files) > 1 {
		below := 0
		for _, f := range files {
			if f.Churn < target.churn {
				below++
			}
		}
		churnPct = float64(below) / float64(len(files)-1) * 100
	}

	s.json(w, http.StatusOK, map[string]any{
		"file_path":                filePath,
		"commit_count_total":       target.churn,
		"commit_count_90d":         target.churn,
		"commit_count_30d":         0,
		"first_commit_at":          nil,
		"last_commit_at":           nil,
		"primary_owner_name":       nil,
		"primary_owner_email":      nil,
		"primary_owner_commit_pct": target.ownership * 100,
		"recent_owner_name":        nil,
		"recent_owner_commit_pct":  target.ownership * 100,
		"top_authors":              []any{},
		"significant_commits":      []any{},
		"co_change_partners":       []any{},
		"is_hotspot":               target.churn >= hotspotChurnThreshold,
		"is_stable":                target.churn < hotspotChurnThreshold,
		"churn_percentile":         churnPct,
		"age_days":                 0,
		"bus_factor":               target.authorCount,
		"contributor_count":        target.authorCount,
		"lines_added_90d":          0,
		"lines_deleted_90d":        0,
		"avg_commit_size":          0,
		"commit_categories":        map[string]int{},
		"merge_commit_count_90d":   0,
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
	resp, err := s.argus.GetModuleGraph(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.json(w, http.StatusOK, map[string]any{"nodes": []any{}, "edges": []any{}})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.json(w, http.StatusOK, resp)
}

func (s *RESTServer) getC4L1(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	resp, err := s.argus.GetC4L1(r.Context(), repoID)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusOK, resp)
}

func (s *RESTServer) getC4L2(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	resp, err := s.argus.GetC4L2(r.Context(), repoID)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusOK, resp)
}

func (s *RESTServer) getC4L3(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	containerID := strings.TrimSpace(r.URL.Query().Get("container_id"))
	if containerID == "" {
		s.json(w, http.StatusOK, map[string]any{
			"container":        nil,
			"components":       []any{},
			"external_systems": []any{},
			"relations":        []any{},
		})
		return
	}
	resp, err := s.argus.GetC4L3(r.Context(), repoID, containerID)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusOK, resp)
}

func (s *RESTServer) getC4Mermaid(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	level := strings.TrimSpace(r.URL.Query().Get("level"))

	var diagram string
	switch level {
	case "2":
		resp, err := s.argus.GetC4L2(r.Context(), repoID)
		if err != nil || len(resp.Containers) == 0 {
			diagram = "C4Container\n  title Container View\n"
			break
		}
		var sb strings.Builder
		sb.WriteString("C4Container\n")
		sb.WriteString("  title Container View\n")
		for _, c := range resp.Containers {
			sb.WriteString(fmt.Sprintf("  Container(%s, \"%s\", \"%s\", \"%d files\")\n",
				strings.ReplaceAll(c.ID, "/", "_"), c.Name, c.Language, c.FileCount))
		}
		for _, rel := range resp.Relations {
			sb.WriteString(fmt.Sprintf("  Rel(%s, %s, \"%s\")\n",
				strings.ReplaceAll(rel.SourceID, "/", "_"),
				strings.ReplaceAll(rel.TargetID, "/", "_"),
				rel.Label))
		}
		diagram = sb.String()
	case "3":
		containerID := strings.TrimSpace(r.URL.Query().Get("container_id"))
		resp, err := s.argus.GetC4L3(r.Context(), repoID, containerID)
		if err != nil || resp.Container == nil {
			diagram = "C4Component\n  title Component View\n"
			break
		}
		var sb strings.Builder
		sb.WriteString("C4Component\n")
		sb.WriteString(fmt.Sprintf("  title Component View — %s\n", resp.Container.Name))
		for _, c := range resp.Components {
			sb.WriteString(fmt.Sprintf("  Component(%s, \"%s\", \"%d symbols\")\n",
				strings.ReplaceAll(c.ID, "/", "_"), c.Name, c.SymbolCount))
		}
		diagram = sb.String()
	default: // level 1
		resp, _ := s.argus.GetC4L1(r.Context(), repoID)
		diagram = fmt.Sprintf("C4Context\n  title System Context\n  System(%s, \"%s\", \"Codebase under analysis\")\n",
			repoID, resp.System.Name)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(diagram))
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
	s.json(w, http.StatusOK, groups)
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
	q := r.URL.Query()
	filterBiomarker := q.Get("biomarker")
	filterMinSev := q.Get("min_severity")
	filterMaxEffort := q.Get("max_effort")
	sortBy := q.Get("sort")

	markers, err := s.argus.GetRepoMarkers(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.json(w, http.StatusOK, map[string]any{"targets": []any{}, "total": 0})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Build file-size index for NLOC approximation.
	files, _ := s.argus.GetRepoFiles(r.Context(), repoID)
	fileSize := make(map[string]int64, len(files))
	for _, f := range files {
		fileSize[f.Path] = f.Size
	}

	refactorCategories := map[string]bool{
		"structural_complexity": true,
		"size_api_complexity":   true,
		"duplication":           true,
		"test_coverage":         true,
	}
	severityRank := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}
	effortWeight := map[string]float64{"S": 1, "M": 2, "L": 4, "XL": 8}

	type finding struct {
		id           string
		biomarkerType string
		severity     string
		function     string
		healthImpact float64
		reason       string
		lineStart    int
		suggestion   string
	}

	type perFile struct {
		findings   []finding
		biomarkers map[string]struct{}
		total      float64
	}

	byFile := make(map[string]*perFile)
	for i, m := range markers {
		if !refactorCategories[string(m.Category)] || m.Deduction <= 0 {
			continue
		}
		pf, ok := byFile[m.File]
		if !ok {
			pf = &perFile{biomarkers: make(map[string]struct{})}
			byFile[m.File] = pf
		}
		pf.biomarkers[m.Type] = struct{}{}
		pf.total += m.Deduction
		pf.findings = append(pf.findings, finding{
			id:           strings.ReplaceAll(m.File, "/", "_") + "_" + strings.ReplaceAll(m.Type, "_", "") + "_" + fmt.Sprintf("%d", i),
			biomarkerType: m.Type,
			severity:     m.Severity,
			healthImpact: m.Deduction,
			reason:       m.Message,
			lineStart:    m.Line,
			suggestion:   m.Suggestion,
		})
	}

	effortBucket := func(nloc int) string {
		switch {
		case nloc <= 40:
			return "S"
		case nloc <= 150:
			return "M"
		case nloc <= 400:
			return "L"
		default:
			return "XL"
		}
	}

	type refactoringTarget struct {
		FilePath        string   `json:"file_path"`
		Score           float64  `json:"score"`
		Nloc            int      `json:"nloc"`
		Module          string   `json:"module"`
		PrimaryBiomarker string  `json:"primary_biomarker"`
		PrimarySeverity  string  `json:"primary_severity"`
		PrimaryReason    string  `json:"primary_reason"`
		PrimaryFunction  *string `json:"primary_function"`
		PrimaryLineStart *int    `json:"primary_line_start"`
		PrimaryLineEnd   *int    `json:"primary_line_end"`
		PrimarySuggestion string `json:"primary_suggestion"`
		PrimaryFindingID  string `json:"primary_finding_id"`
		TotalImpact      float64  `json:"total_impact"`
		FindingCount     int      `json:"finding_count"`
		Biomarkers       []string `json:"biomarkers"`
		EffortBucket     string   `json:"effort_bucket"`
		ImpactPerEffort  float64  `json:"impact_per_effort"`
		AllFindings      []map[string]any `json:"all_findings"`
	}

	targets := make([]refactoringTarget, 0, len(byFile))
	for filePath, pf := range byFile {
		// Approximate NLOC from file size (rough: ~50 chars per line).
		size := fileSize[filePath]
		nloc := int(size / 50)
		if nloc < 1 {
			nloc = 1
		}
		bucket := effortBucket(nloc)

		// Primary = finding with highest deduction.
		primary := pf.findings[0]
		for _, f := range pf.findings[1:] {
			if f.healthImpact > primary.healthImpact {
				primary = f
			}
		}

		// Unique biomarker list.
		bms := make([]string, 0, len(pf.biomarkers))
		for bm := range pf.biomarkers {
			bms = append(bms, bm)
		}

		module := strings.TrimSuffix(filePath, "/"+filepath.Base(filePath))
		if module == filePath {
			module = "."
		}

		score := 10.0 - pf.total
		if score < 1 {
			score = 1
		}

		ew := effortWeight[bucket]
		ipe := 0.0
		if ew > 0 {
			ipe = pf.total / ew
		}

		var lineStart *int
		if primary.lineStart > 0 {
			lineStart = &primary.lineStart
		}

		allFindings := make([]map[string]any, 0, len(pf.findings))
		for _, f := range pf.findings {
			var ls *int
			if f.lineStart > 0 {
				ls = &f.lineStart
			}
			allFindings = append(allFindings, map[string]any{
				"id":             f.id,
				"biomarker_type": f.biomarkerType,
				"severity":       f.severity,
				"function_name":  nil,
				"health_impact":  f.healthImpact,
				"reason":         f.reason,
				"line_start":     ls,
				"status":         "open",
			})
		}

		t := refactoringTarget{
			FilePath:         filePath,
			Score:            score,
			Nloc:             nloc,
			Module:           module,
			PrimaryBiomarker: primary.biomarkerType,
			PrimarySeverity:  primary.severity,
			PrimaryReason:    primary.reason,
			PrimaryFunction:  nil,
			PrimaryLineStart: lineStart,
			PrimaryLineEnd:   nil,
			PrimarySuggestion: primary.suggestion,
			PrimaryFindingID: primary.id,
			TotalImpact:      pf.total,
			FindingCount:     len(pf.findings),
			Biomarkers:       bms,
			EffortBucket:     bucket,
			ImpactPerEffort:  ipe,
			AllFindings:      allFindings,
		}

		// Apply filters.
		if filterBiomarker != "" && filterBiomarker != "all" {
			found := false
			for _, bm := range bms {
				if bm == filterBiomarker {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if filterMinSev != "" && filterMinSev != "all" {
			if severityRank[t.PrimarySeverity] < severityRank[filterMinSev] {
				continue
			}
		}
		if filterMaxEffort != "" && filterMaxEffort != "all" {
			bucketRank := map[string]int{"S": 1, "M": 2, "L": 3, "XL": 4}
			if bucketRank[t.EffortBucket] > bucketRank[filterMaxEffort] {
				continue
			}
		}

		targets = append(targets, t)
	}

	// Sort.
	sortTargets := func(i, j int) bool {
		switch sortBy {
		case "total_impact":
			return targets[i].TotalImpact > targets[j].TotalImpact
		case "score":
			return targets[i].Score < targets[j].Score
		case "finding_count":
			return targets[i].FindingCount > targets[j].FindingCount
		default: // impact_per_effort
			return targets[i].ImpactPerEffort > targets[j].ImpactPerEffort
		}
	}
	for i := 1; i < len(targets); i++ {
		for j := 0; j < len(targets)-i; j++ {
			if sortTargets(j+1, j) {
				targets[j], targets[j+1] = targets[j+1], targets[j]
			}
		}
	}

	s.json(w, http.StatusOK, map[string]any{
		"targets": targets,
		"total":   len(targets),
	})
}

// wikiPageToResponse maps the internal WikiPage model to the JSON shape the frontend expects.
func wikiPageToResponse(p models.WikiPage) map[string]any {
	return map[string]any{
		"id":               p.ID,
		"repository_id":    p.RepoID,
		"page_type":        p.Type,
		"title":            p.Subject,
		"content":          p.Content,
		"target_path":      p.Subject,
		"source_hash":      "",
		"model_name":       "",
		"provider_name":    "",
		"input_tokens":     0,
		"output_tokens":    0,
		"cached_tokens":    0,
		"generation_level": p.Level,
		"version":          1,
		"confidence":       0.0,
		"freshness_status": "fresh",
		"metadata":         map[string]any{},
		"human_notes":      nil,
		"created_at":       p.CreatedAt.Format(time.RFC3339),
		"updated_at":       p.UpdatedAt.Format(time.RFC3339),
	}
}

func (s *RESTServer) listOwners(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	q := r.URL.Query()
	search := q.Get("q")
	sortKey := q.Get("sort")
	limit := 50
	offset := 0
	if v := q.Get("limit"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &limit); n != 1 || err != nil {
			limit = 50
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &offset); n != 1 || err != nil {
			offset = 0
		}
	}

	entries, total, err := s.argus.GetOwners(r.Context(), repoID, search, sortKey, limit, offset)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.json(w, http.StatusOK, map[string]any{"items": []any{}, "total": 0, "has_more": false, "next_offset": nil})
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	hasMore := offset+len(entries) < total
	var nextOffset *int
	if hasMore {
		n := offset + len(entries)
		nextOffset = &n
	}
	if entries == nil {
		entries = []argus.OwnerListEntry{}
	}
	s.json(w, http.StatusOK, map[string]any{
		"items":       entries,
		"total":       total,
		"has_more":    hasMore,
		"next_offset": nextOffset,
	})
}

func (s *RESTServer) listPages(w http.ResponseWriter, r *http.Request) {
	repoID := r.URL.Query().Get("repo_id")
	if repoID == "" {
		s.json(w, http.StatusOK, []any{})
		return
	}
	pages, err := s.argus.ListWikiPages(r.Context(), repoID)
	if err != nil {
		s.json(w, http.StatusOK, []any{})
		return
	}
	// Apply optional page_type filter.
	pageType := r.URL.Query().Get("page_type")
	out := make([]map[string]any, 0, len(pages))
	for _, p := range pages {
		if pageType != "" && p.Type != pageType {
			continue
		}
		out = append(out, wikiPageToResponse(p))
	}
	s.json(w, http.StatusOK, out)
}

func (s *RESTServer) getPageByID(w http.ResponseWriter, r *http.Request) {
	pageID := r.URL.Query().Get("page_id")
	if pageID == "" {
		s.error(w, http.StatusBadRequest, "page_id required")
		return
	}
	page, err := s.argus.GetWikiPage(r.Context(), pageID)
	if err != nil {
		s.error(w, http.StatusNotFound, "page not found")
		return
	}
	s.json(w, http.StatusOK, wikiPageToResponse(page))
}

func (s *RESTServer) json(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

func (s *RESTServer) error(w http.ResponseWriter, code int, message string) {
	s.json(w, code, map[string]string{"error": message})
}

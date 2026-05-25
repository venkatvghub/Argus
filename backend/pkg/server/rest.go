package server

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
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
		r.Post("/repos/{repoID}/full-resync", s.forceResync)
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
		r.Post("/repos/{repoID}/dead-code/analyze", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{"job_id": ""})
		})

		// Blast radius
		r.Post("/repos/{repoID}/blast-radius", s.postBlastRadius)

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
		r.Get("/graph/{repoID}/dead-nodes", s.getDeadNodesGraph)
		r.Get("/graph/{repoID}/hot-files", s.getHotFilesGraph)
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

		// Wiki pages — flat query-param style (/api/pages?repo_id=...)
		r.Get("/pages", s.listPages)
		r.Get("/pages/lookup", s.getPageByID)
		r.Get("/pages/lookup/versions", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, []any{})
		})
		r.Post("/pages/lookup/regenerate", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{"job_id": ""})
		})

		// Module health
		r.Get("/repos/{repoID}/modules/health", s.getModuleHealth)
		r.Get("/repos/{repoID}/modules/health/{modulePath}", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, http.StatusOK, map[string]any{})
		})

		// LLM cost tracking
		r.Get("/repos/{repoID}/costs", s.getRepoCosts)
		r.Get("/repos/{repoID}/costs/summary", s.getRepoCostSummary)

		// Owners
		r.Get("/repos/{repoID}/owners", s.listOwners)
		r.Get("/repos/{repoID}/owners/{ownerKey}", func(w http.ResponseWriter, r *http.Request) {
			s.error(w, http.StatusNotImplemented, "owner detail endpoint not yet implemented")
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

func (s *RESTServer) forceResync(w http.ResponseWriter, r *http.Request) {
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
	// Reset the HEAD checkpoint so Analyze won't skip due to unchanged HEAD.
	if err := s.argus.ResetAnalysisCheckpoint(r.Context(), repoID); err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	jobID, err := s.argus.Analyze(r.Context(), repo.Path)
	if err != nil {
		s.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.json(w, http.StatusAccepted, map[string]string{"job_id": jobID, "message": "full resync started"})
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

	// When kind=biomarker, return markers shaped as symOut items instead of symbols.
	if filterKind == "biomarker" {
		markers, _ := s.argus.GetRepoMarkers(r.Context(), repoID)
		seen := make(map[string]struct{})
		var out []symOut
		for _, m := range markers {
			if filterQ != "" && !strings.Contains(strings.ToLower(m.Message), filterQ) && !strings.Contains(strings.ToLower(m.File), filterQ) {
				continue
			}
			fm := fileLookup[m.File]
			if filterLang != "" && filterLang != "all" && fm.language != filterLang {
				continue
			}
			id := fmt.Sprintf("%s:%s:%d", repoID, m.File, m.Line)
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			pct := churnPercentile(fm.churn)
			hotBool := fm.churn >= hotspotChurn
			h := symOut{
				ID:                  id,
				RepositoryID:        repoID,
				FilePath:            m.File,
				SymbolID:            id,
				Name:                m.Message,
				QualifiedName:       m.Message,
				Kind:                "biomarker",
				Signature:           m.Type,
				StartLine:           m.Line,
				EndLine:             m.Line,
				Visibility:          "public",
				Language:            fm.language,
				FileChurnPercentile: &pct,
				FileIsHotspot:       &hotBool,
			}
			out = append(out, h)
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
		return
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
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Name < out[j].Name
		})
	default: // "importance" — sort by hotspot descending, then name
		sort.SliceStable(out, func(i, j int) bool {
			aHot := out[i].FileIsHotspot != nil && *out[i].FileIsHotspot
			bHot := out[j].FileIsHotspot != nil && *out[j].FileIsHotspot
			if aHot != bHot {
				return aHot
			}
			return out[i].Name < out[j].Name
		})
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
	ID         string `json:"id"`
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
	for _, m := range markers {
		if !securityMarkerTypes[m.Type] {
			continue
		}
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(m.File+m.Type+fmt.Sprint(m.Line))))[:12]
		out = append(out, securityFinding{
			ID:         id,
			FilePath:   m.File,
			Kind:       m.Type,
			Severity:   m.Severity,
			Snippet:    m.Message,
			DetectedAt: now,
		})
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
	moduleDir := filepath.ToSlash(filepath.Dir(filePath))
	var modulePtr *string
	if moduleDir != "" {
		modulePtr = &moduleDir
	}
	metric := &HealthFileMetric{
		FilePath:    filePath,
		Score:       score.Final,
		HasTestFile: false,
		Module:      modulePtr,
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
	type communityOut struct {
		CommunityID int      `json:"community_id"`
		ID          int      `json:"id"`
		Size        int      `json:"size"`
		Files       []string `json:"files"`
	}
	out := make([]communityOut, len(communities))
	for i, c := range communities {
		out[i] = communityOut{CommunityID: c.ID, ID: c.ID, Size: c.Size, Files: c.Files}
	}
	s.json(w, http.StatusOK, out)
}

func (s *RESTServer) getDeadNodesGraph(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	markers, err := s.argus.GetRepoMarkers(r.Context(), repoID)
	if err != nil {
		s.json(w, http.StatusOK, map[string]any{"nodes": []any{}, "links": []any{}})
		return
	}
	files, _ := s.argus.GetRepoFiles(r.Context(), repoID)
	type fileInfo struct {
		language  string
		churn     int
		ownership float64
	}
	fileLookup := make(map[string]fileInfo, len(files))
	for _, f := range files {
		fileLookup[f.Path] = fileInfo{language: f.Language, churn: f.Churn, ownership: f.Ownership}
	}

	type deadNode struct {
		NodeID          string  `json:"node_id"`
		NodeType        string  `json:"node_type"`
		Language        string  `json:"language"`
		SymbolCount     int     `json:"symbol_count"`
		Pagerank        float64 `json:"pagerank"`
		Betweenness     float64 `json:"betweenness"`
		CommunityID     int     `json:"community_id"`
		IsTest          bool    `json:"is_test"`
		IsEntryPoint    bool    `json:"is_entry_point"`
		HasDoc          bool    `json:"has_doc"`
		ConfidenceGroup string  `json:"confidence_group"`
	}

	seen := make(map[string]bool)
	nodes := make([]deadNode, 0)
	for _, m := range markers {
		if m.Type != "dead_code" || seen[m.File] {
			continue
		}
		seen[m.File] = true
		fi := fileLookup[m.File]
		base := strings.ToLower(filepath.Base(m.File))
		isTest := strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
		isEntry := base == "main.go" || strings.Contains(m.File, "/cmd/")
		confGroup := "high"
		if isTest || strings.HasSuffix(base, ".pb.go") || strings.Contains(base, "_gen.") {
			confGroup = "low"
		} else if fi.churn > 20 || isEntry {
			confGroup = "medium"
		}
		nodes = append(nodes, deadNode{
			NodeID:          m.File,
			NodeType:        "file",
			Language:        fi.language,
			SymbolCount:     0,
			Pagerank:        0,
			Betweenness:     0,
			CommunityID:     0,
			IsTest:          isTest,
			IsEntryPoint:    isEntry,
			HasDoc:          false,
			ConfidenceGroup: confGroup,
		})
	}
	s.json(w, http.StatusOK, map[string]any{"nodes": nodes, "links": []any{}})
}

func (s *RESTServer) getHotFilesGraph(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	files, err := s.argus.GetRepoFiles(r.Context(), repoID)
	if err != nil {
		s.json(w, http.StatusOK, map[string]any{"nodes": []any{}, "links": []any{}})
		return
	}

	type hotNode struct {
		NodeID       string  `json:"node_id"`
		NodeType     string  `json:"node_type"`
		Language     string  `json:"language"`
		SymbolCount  int     `json:"symbol_count"`
		Pagerank     float64 `json:"pagerank"`
		Betweenness  float64 `json:"betweenness"`
		CommunityID  int     `json:"community_id"`
		IsTest       bool    `json:"is_test"`
		IsEntryPoint bool    `json:"is_entry_point"`
		HasDoc       bool    `json:"has_doc"`
		CommitCount  int     `json:"commit_count"`
	}

	// compute churn p90 as hotspot threshold
	churns := make([]int, 0, len(files))
	for _, f := range files {
		if f.IsFile {
			churns = append(churns, f.Churn)
		}
	}
	sort.Ints(churns)
	threshold := 10
	if n := len(churns); n > 0 {
		p90idx := int(float64(n)*0.9)
		if p90idx >= n {
			p90idx = n - 1
		}
		threshold = churns[p90idx]
		if threshold < 5 {
			threshold = 5
		}
	}

	nodes := make([]hotNode, 0)
	for _, f := range files {
		if !f.IsFile || f.Churn < threshold {
			continue
		}
		base := strings.ToLower(filepath.Base(f.Path))
		isTest := strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
		isEntry := base == "main.go" || strings.Contains(f.Path, "/cmd/")
		nodes = append(nodes, hotNode{
			NodeID:       f.Path,
			NodeType:     "file",
			Language:     f.Language,
			SymbolCount:  0,
			Pagerank:     0,
			Betweenness:  0,
			CommunityID:  0,
			IsTest:       isTest,
			IsEntryPoint: isEntry,
			HasDoc:       false,
			CommitCount:  f.Churn,
		})
	}
	s.json(w, http.StatusOK, map[string]any{"nodes": nodes, "links": []any{}})
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
	var totalChurn int

	type ownerAgg struct {
		name      string
		fileCount int
	}
	ownersByEmail := make(map[string]*ownerAgg)

	for _, f := range files {
		if f.Churn >= hotspotChurnThreshold {
			hotspotCount++
		} else {
			stableCount++
		}
		totalChurn += f.Churn

		// Aggregate file counts per primary owner.
		if f.PrimaryAuthor != "" || f.Ownership > 0 {
			// Use path as email key fallback when no PrimaryAuthor set yet.
			key := f.PrimaryAuthor
			if key == "" {
				continue
			}
			agg, ok := ownersByEmail[key]
			if !ok {
				agg = &ownerAgg{name: f.PrimaryAuthor}
				ownersByEmail[key] = agg
			}
			agg.fileCount++
		}
	}

	avgChurn := 0.0
	if len(files) > 0 {
		avgChurn = float64(totalChurn) / float64(len(files))
	}

	// Build top_owners sorted by file_count desc, capped at 10.
	type topOwner struct {
		Name      string  `json:"name"`
		FileCount int     `json:"file_count"`
		Pct       float64 `json:"pct"`
	}
	topOwners := make([]topOwner, 0, len(ownersByEmail))
	for _, agg := range ownersByEmail {
		pct := 0.0
		if len(files) > 0 {
			pct = float64(agg.fileCount) / float64(len(files))
		}
		topOwners = append(topOwners, topOwner{
			Name:      agg.name,
			FileCount: agg.fileCount,
			Pct:       pct,
		})
	}
	sort.Slice(topOwners, func(i, j int) bool {
		return topOwners[i].FileCount > topOwners[j].FileCount
	})
	if len(topOwners) > 10 {
		topOwners = topOwners[:10]
	}

	s.json(w, http.StatusOK, map[string]any{
		"total_files":              len(files),
		"hotspot_count":            hotspotCount,
		"stable_count":             stableCount,
		"average_churn_percentile": avgChurn,
		"top_owners":               topOwners,
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
		churn         int
		ownership     float64
		authorCount   int
		primaryAuthor string
	}
	const hotspotChurnThreshold = 10
	totalChurn := 0
	for _, f := range files {
		totalChurn += f.Churn
		if filepath.ToSlash(f.Path) == filepath.ToSlash(filePath) {
			c := f.Churn
			o := f.Ownership
			a := f.AuthorCount
			pa := f.PrimaryAuthor
			target = &struct {
				churn         int
				ownership     float64
				authorCount   int
				primaryAuthor string
			}{c, o, a, pa}
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
		"primary_owner_name":       target.primaryAuthor,
		"primary_owner_email":      target.primaryAuthor,
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

	granularity := strings.TrimSpace(r.URL.Query().Get("granularity"))
	if granularity == "" {
		granularity = "module"
	}

	type ownershipItem struct {
		ModulePath   string  `json:"module_path"`
		PrimaryOwner *string `json:"primary_owner"`
		OwnerPct     float64 `json:"owner_pct"`
		FileCount    int     `json:"file_count"`
		IsSilo       bool    `json:"is_silo"`
	}

	if granularity == "file" {
		items := make([]ownershipItem, 0, len(files))
		for _, f := range files {
			var owner *string
			if f.PrimaryAuthor != "" {
				s := f.PrimaryAuthor
				owner = &s
			}
			items = append(items, ownershipItem{
				ModulePath:   f.Path,
				PrimaryOwner: owner,
				OwnerPct:     f.Ownership,
				FileCount:    1,
				IsSilo:       f.Ownership > 0.8,
			})
		}
		s.json(w, http.StatusOK, map[string]any{
			"items":       items,
			"total":       len(items),
			"has_more":    false,
			"next_offset": nil,
		})
		return
	}

	// module granularity: group by parent directory
	type moduleAgg struct {
		totalChurnWeightedOwnership float64
		totalChurn                  int
		fileCount                   int
		ownerCommits                map[string]int
		ownerNames                  map[string]string
	}
	moduleMap := make(map[string]*moduleAgg)

	for _, f := range files {
		dir := filepath.ToSlash(filepath.Dir(f.Path))
		if dir == "." {
			dir = "(root)"
		}
		agg, ok := moduleMap[dir]
		if !ok {
			agg = &moduleAgg{
				ownerCommits: make(map[string]int),
				ownerNames:   make(map[string]string),
			}
			moduleMap[dir] = agg
		}
		agg.fileCount++
		if f.PrimaryAuthor != "" {
			// Weight by churn: files with more commits influence module owner more.
			weight := f.Churn
			if weight < 1 {
				weight = 1
			}
			agg.ownerCommits[f.PrimaryAuthor] += weight
			agg.ownerNames[f.PrimaryAuthor] = f.PrimaryAuthor
			agg.totalChurn += weight
		}
		agg.totalChurnWeightedOwnership += f.Ownership * float64(max(f.Churn, 1))
	}

	items := make([]ownershipItem, 0, len(moduleMap))
	for dir, agg := range moduleMap {
		var primaryOwner *string
		ownerPct := 0.0

		if len(agg.ownerCommits) > 0 {
			maxW := 0
			var topOwner string
			for owner, w := range agg.ownerCommits {
				if w > maxW || (w == maxW && (topOwner == "" || owner < topOwner)) {
					maxW = w
					topOwner = owner
				}
			}
			if topOwner != "" {
				o := topOwner
				primaryOwner = &o
			}
			if agg.totalChurn > 0 {
				ownerPct = float64(maxW) / float64(agg.totalChurn)
			}
		}

		items = append(items, ownershipItem{
			ModulePath:   dir,
			PrimaryOwner: primaryOwner,
			OwnerPct:     ownerPct,
			FileCount:    agg.fileCount,
			IsSilo:       ownerPct > 0.8,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].FileCount > items[j].FileCount
	})

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
		FilePath        string  `json:"file_path"`
		Language        string  `json:"language"`
		CommitCount90d  int     `json:"commit_count_90d"`
		CommitCount30d  int     `json:"commit_count_30d"`
		ChurnPercentile float64 `json:"churn_percentile"`
		PrimaryOwner    *string `json:"primary_owner"`
		IsHotspot       bool    `json:"is_hotspot"`
		IsStable        bool    `json:"is_stable"`
		BusFactor       int     `json:"bus_factor"`
		ContributorCount int    `json:"contributor_count"`
		LinesAdded90d   int     `json:"lines_added_90d"`
		LinesDeleted90d int     `json:"lines_deleted_90d"`
		AvgCommitSize   int     `json:"avg_commit_size"`
		CommitCategories map[string]int `json:"commit_categories"`
	}

	const hotspotChurnThreshold = 10

	// Pre-compute churn percentile for each file (O(n log n), not O(n²) per file).
	n := len(files)
	churnPctMap := make(map[string]float64, n)
	if n > 1 {
		type pathChurn struct {
			path  string
			churn int
		}
		pairs := make([]pathChurn, 0, n)
		for _, f := range files {
			pairs = append(pairs, pathChurn{path: f.Path, churn: f.Churn})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].churn < pairs[j].churn })
		below := 0
		for i := 0; i < len(pairs); {
			churn := pairs[i].churn
			j := i
			for j < len(pairs) && pairs[j].churn == churn {
				j++
			}
			pct := float64(below) / float64(n-1) * 100
			for k := i; k < j; k++ {
				churnPctMap[pairs[k].path] = pct
			}
			below += j - i
			i = j
		}
	}

	buildItem := func(f models.FileNode) hotspotItem {
		churnPct := churnPctMap[f.Path]
		var owner *string
		if f.PrimaryAuthor != "" {
			o := f.PrimaryAuthor
			owner = &o
		}
		return hotspotItem{
			FilePath:         f.Path,
			Language:         f.Language,
			CommitCount90d:   f.Churn,
			CommitCount30d:   0,
			ChurnPercentile:  churnPct,
			PrimaryOwner:     owner,
			IsHotspot:        f.Churn >= hotspotChurnThreshold,
			IsStable:         f.Churn < hotspotChurnThreshold,
			BusFactor:        f.AuthorCount,
			ContributorCount: f.AuthorCount,
			LinesAdded90d:    0,
			LinesDeleted90d:  0,
			AvgCommitSize:    0,
			CommitCategories: map[string]int{},
		}
	}

	items := make([]hotspotItem, 0, len(files))
	for _, f := range files {
		items = append(items, buildItem(f))
	}

	// Sort by churn descending.
	sort.Slice(items, func(i, j int) bool {
		return items[i].CommitCount90d > items[j].CommitCount90d
	})

	// Apply limit / offset pagination.
	q := r.URL.Query()
	limit := 50
	offset := 0
	if v := q.Get("limit"); v != "" {
		if parsed, err := fmt.Sscanf(v, "%d", &limit); parsed != 1 || err != nil || limit <= 0 {
			limit = 50
		}
	}
	if v := q.Get("offset"); v != "" {
		if parsed, err := fmt.Sscanf(v, "%d", &offset); parsed != 1 || err != nil || offset < 0 {
			offset = 0
		}
	}

	total := len(items)
	if offset >= total {
		items = []hotspotItem{}
	} else {
		items = items[offset:]
		if len(items) > limit {
			items = items[:limit]
		}
	}

	s.json(w, http.StatusOK, map[string]any{
		"items":       items,
		"total":       total,
		"has_more":    offset+len(items) < total,
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
	markers, err := s.argus.GetRepoMarkers(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	files, _ := s.argus.GetRepoFiles(r.Context(), repoID)
	type fileInfo struct {
		churn     int
		ownership float64
	}
	fileLookup := make(map[string]fileInfo, len(files))
	for _, f := range files {
		fileLookup[f.Path] = fileInfo{churn: f.Churn, ownership: f.Ownership}
	}
	confHigh, confMed, confLow := 0, 0, 0
	count := 0
	for _, m := range markers {
		if m.Type != "dead_code" {
			continue
		}
		count++
		base := strings.ToLower(filepath.Base(m.File))
		var conf float64
		if strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, ".test.ts") ||
			strings.HasSuffix(base, ".test.tsx") || strings.HasSuffix(base, ".spec.ts") ||
			strings.HasSuffix(base, ".spec.tsx") {
			conf = 0.5
		} else if strings.HasSuffix(base, ".pb.go") || strings.Contains(base, "_gen.") ||
			strings.Contains(base, ".gen.") || strings.Contains(base, "_mock.") {
			conf = 0.55
		} else if base == "main.go" || base == "index.ts" || base == "index.tsx" ||
			strings.Contains(m.File, "/cmd/") {
			conf = 0.6
		} else if fi, ok := fileLookup[m.File]; ok && fi.churn > 20 {
			conf = 0.65
		} else if fi, ok := fileLookup[m.File]; ok && fi.churn <= 5 && fi.ownership > 0.8 {
			conf = 0.9
		} else {
			conf = 0.8
		}
		switch {
		case conf >= 0.8:
			confHigh++
		case conf >= 0.6:
			confMed++
		default:
			confLow++
		}
	}
	s.json(w, http.StatusOK, map[string]any{
		"total_findings":     count,
		"confidence_summary": map[string]int{"high": confHigh, "medium": confMed, "low": confLow},
		"deletable_lines":    0,
		"total_lines":        0,
		"by_kind":            map[string]int{"zombie_export": count},
	})
}

func (s *RESTServer) getDeadCode(w http.ResponseWriter, r *http.Request) {
	repoID := strings.TrimSpace(chi.URLParam(r, "repoID"))
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repo_id is required")
		return
	}
	markers, err := s.argus.GetRepoMarkers(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, err.Error())
		} else {
			s.error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	files, _ := s.argus.GetRepoFiles(r.Context(), repoID)
	type fileInfo struct {
		churn     int
		ownership float64
		owner     string
	}
	fileLookup := make(map[string]fileInfo, len(files))
	for _, f := range files {
		fileLookup[f.Path] = fileInfo{churn: f.Churn, ownership: f.Ownership, owner: f.PrimaryAuthor}
	}

	deadCodeConfidence := func(path string) float64 {
		base := strings.ToLower(filepath.Base(path))
		// Test files: high false-positive rate
		if strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, ".test.ts") ||
			strings.HasSuffix(base, ".test.tsx") || strings.HasSuffix(base, ".spec.ts") ||
			strings.HasSuffix(base, ".spec.tsx") {
			return 0.5
		}
		// Generated files: protobuf, mocks, codegen
		if strings.HasSuffix(base, ".pb.go") || strings.Contains(base, "_gen.") ||
			strings.Contains(base, ".gen.") || strings.Contains(base, "_mock.") {
			return 0.55
		}
		// Entry points: may be called by runtime/OS, not graph-traceable
		if base == "main.go" || base == "index.ts" || base == "index.tsx" ||
			strings.Contains(path, "/cmd/") {
			return 0.6
		}
		fi, ok := fileLookup[path]
		if !ok {
			return 0.75
		}
		// High-churn file: recently active, exports may be in-flight
		if fi.churn > 20 {
			return 0.65
		}
		// Stable, single-owner: very likely genuinely dead
		if fi.churn <= 5 && fi.ownership > 0.8 {
			return 0.9
		}
		return 0.8
	}

	type deadCodeItem struct {
		ID           string  `json:"id"`
		RepoID       string  `json:"repo_id"`
		FilePath     string  `json:"file_path"`
		SymbolName   string  `json:"symbol_name"`
		Kind         string  `json:"kind"`
		Confidence   float64 `json:"confidence"`
		SafeToDelete bool    `json:"safe_to_delete"`
		LineStart    int     `json:"line_start"`
		LineEnd      int     `json:"line_end"`
		PrimaryOwner string  `json:"primary_owner"`
		Status       string  `json:"status"`
		Message      string  `json:"message"`
	}
	out := make([]deadCodeItem, 0)
	for _, m := range markers {
		if m.Type != "dead_code" {
			continue
		}
		conf := deadCodeConfidence(m.File)
		owner := fileLookup[m.File].owner
		out = append(out, deadCodeItem{
			ID:           fmt.Sprintf("%s:%d:%s", m.File, m.Line, m.Message),
			RepoID:       repoID,
			FilePath:     m.File,
			SymbolName:   m.Message,
			Kind:         "zombie_export",
			Confidence:   conf,
			SafeToDelete: conf >= 0.7,
			LineStart:    m.Line,
			LineEnd:      m.Line,
			PrimaryOwner: owner,
			Status:       "open",
			Message:      m.Message,
		})
	}
	s.json(w, http.StatusOK, out)
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
	sort.Slice(targets, sortTargets)

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

// getModuleHealth handles GET /repos/{repoID}/modules/health
func (s *RESTServer) getModuleHealth(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	q := r.URL.Query()
	sortBy := q.Get("sort")
	limit := 30
	offset := 0
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
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
	markers, _ := s.argus.GetRepoMarkers(r.Context(), repoID)
	symbols, _ := s.argus.GetRepoSymbols(r.Context(), repoID)

	// Compute global churn percentile helpers.
	n := len(files)
	churnPercentileFor := func(churn int) float64 {
		if n <= 1 {
			return 0
		}
		below := 0
		for _, f := range files {
			if f.Churn < churn {
				below++
			}
		}
		return float64(below) / float64(n-1) * 100
	}

	// Group files by module (directory).
	type moduleData struct {
		files     []models.FileNode
		churns    []float64
		busFactor []int
		owners    map[string]int
	}
	modules := make(map[string]*moduleData)
	for _, f := range files {
		dir := filepath.ToSlash(filepath.Dir(f.Path))
		if dir == "." {
			dir = "(root)"
		}
		md, ok := modules[dir]
		if !ok {
			md = &moduleData{owners: make(map[string]int)}
			modules[dir] = md
		}
		md.files = append(md.files, f)
		md.churns = append(md.churns, churnPercentileFor(f.Churn))
		md.busFactor = append(md.busFactor, f.AuthorCount)
		if f.PrimaryAuthor != "" {
			md.owners[f.PrimaryAuthor]++
		}
	}

	// Count dead_code markers per module.
	deadCodeByModule := make(map[string]int)
	for _, m := range markers {
		if m.Type != "dead_code" {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(m.File))
		if dir == "." {
			dir = "(root)"
		}
		deadCodeByModule[dir]++
	}

	// Count symbols per module.
	symbolsByModule := make(map[string]int)
	for _, sym := range symbols {
		dir := filepath.ToSlash(filepath.Dir(sym.FilePath))
		if dir == "." {
			dir = "(root)"
		}
		symbolsByModule[dir]++
	}

	type moduleHealthSummary struct {
		ModulePath        string  `json:"module_path"`
		FileCount         int     `json:"file_count"`
		SymbolCount       int     `json:"symbol_count"`
		HotspotCount      int     `json:"hotspot_count"`
		DeadCodeCount     int     `json:"dead_code_count"`
		DeadCodeLines     int     `json:"dead_code_lines"`
		AvgChurnPercentile float64 `json:"avg_churn_percentile"`
		MedianBusFactor   float64 `json:"median_bus_factor"`
		MinBusFactor      int     `json:"min_bus_factor"`
		PrimaryOwner      *string `json:"primary_owner"`
		PrimaryOwnerPct   float64 `json:"primary_owner_pct"`
		IsSilo            bool    `json:"is_silo"`
		DecisionCount     int     `json:"decision_count"`
		DocCoveragePct    float64 `json:"doc_coverage_pct"`
		HealthScore       float64 `json:"health_score"`
	}

	const hotspotChurnThreshold = 10

	clamp := func(v, lo, hi float64) float64 {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}

	items := make([]moduleHealthSummary, 0, len(modules))
	for modPath, md := range modules {
		fc := len(md.files)

		// Avg churn percentile.
		avgChurn := 0.0
		if fc > 0 {
			sum := 0.0
			for _, c := range md.churns {
				sum += c
			}
			avgChurn = sum / float64(fc)
		}

		// Bus factor stats.
		sortedBF := make([]int, len(md.busFactor))
		copy(sortedBF, md.busFactor)
		sort.Ints(sortedBF)
		minBF := 1
		if len(sortedBF) > 0 {
			minBF = sortedBF[0]
			if minBF < 1 {
				minBF = 1
			}
		}
		medianBF := 0.0
		if len(sortedBF) > 0 {
			mid := len(sortedBF) / 2
			if len(sortedBF)%2 == 0 {
				medianBF = float64(sortedBF[mid-1]+sortedBF[mid]) / 2.0
			} else {
				medianBF = float64(sortedBF[mid])
			}
		}

		// Primary owner.
		var primaryOwner *string
		primaryOwnerPct := 0.0
		if len(md.owners) > 0 {
			bestOwner := ""
			bestCount := 0
			for owner, cnt := range md.owners {
				if cnt > bestCount {
					bestCount = cnt
					bestOwner = owner
				}
			}
			if bestOwner != "" {
				primaryOwner = &bestOwner
				primaryOwnerPct = float64(bestCount) / float64(fc)
			}
		}

		// Hotspot count.
		hotspotCount := 0
		for _, f := range md.files {
			if f.Churn >= hotspotChurnThreshold {
				hotspotCount++
			}
		}

		deadCodeCount := deadCodeByModule[modPath]
		symCount := symbolsByModule[modPath]

		// Health score.
		score := 100.0
		score -= clamp(avgChurn*0.4, 0, 40)
		score -= clamp((1-primaryOwnerPct)*20, 0, 20)
		score -= clamp((1/float64(minBF))*20, 0, 20)
		score -= clamp(float64(deadCodeCount)*2, 0, 20)
		score = clamp(score, 0, 100)

		items = append(items, moduleHealthSummary{
			ModulePath:         modPath,
			FileCount:          fc,
			SymbolCount:        symCount,
			HotspotCount:       hotspotCount,
			DeadCodeCount:      deadCodeCount,
			DeadCodeLines:      0,
			AvgChurnPercentile: avgChurn,
			MedianBusFactor:    medianBF,
			MinBusFactor:       minBF,
			PrimaryOwner:       primaryOwner,
			PrimaryOwnerPct:    primaryOwnerPct,
			IsSilo:             primaryOwnerPct > 0.8,
			DecisionCount:      0,
			DocCoveragePct:     0,
			HealthScore:        score,
		})
	}

	// Sort.
	switch sortBy {
	case "health_score":
		sort.Slice(items, func(i, j int) bool {
			return items[i].HealthScore < items[j].HealthScore
		})
	case "hotspot_count":
		sort.Slice(items, func(i, j int) bool {
			return items[i].HotspotCount > items[j].HotspotCount
		})
	case "dead_code_lines":
		sort.Slice(items, func(i, j int) bool {
			return items[i].DeadCodeCount > items[j].DeadCodeCount
		})
	case "file_count":
		sort.Slice(items, func(i, j int) bool {
			return items[i].FileCount > items[j].FileCount
		})
	default:
		sort.Slice(items, func(i, j int) bool {
			return items[i].HealthScore < items[j].HealthScore
		})
	}

	total := len(items)
	if offset >= total {
		s.json(w, http.StatusOK, map[string]any{
			"items": []moduleHealthSummary{}, "total": total, "has_more": false, "next_offset": nil,
		})
		return
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := items[offset:end]
	hasMore := end < total
	s.json(w, http.StatusOK, map[string]any{
		"items": page, "total": total, "has_more": hasMore, "next_offset": nil,
	})
}

// postBlastRadius handles POST /repos/{repoID}/blast-radius
func (s *RESTServer) postBlastRadius(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	var body struct {
		ChangedFiles []string `json:"changed_files"`
		MaxDepth     int      `json:"max_depth"`
	}
	body.MaxDepth = 3
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.MaxDepth <= 0 {
		body.MaxDepth = 3
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

	graph, _ := s.argus.GetGraphExport(r.Context(), repoID)

	// Build file map and churn array for percentile.
	fileMap := make(map[string]models.FileNode, len(files))
	churns := make([]int, 0, len(files))
	for _, f := range files {
		fileMap[filepath.ToSlash(f.Path)] = f
		churns = append(churns, f.Churn)
	}
	churnPctFor := func(churn int) float64 {
		if len(churns) <= 1 {
			return 0
		}
		below := 0
		for _, c := range churns {
			if c < churn {
				below++
			}
		}
		return float64(below) / float64(len(churns)-1) * 100
	}

	// Compute max pagerank for normalization.
	maxPR := 0.0
	prByID := make(map[string]float64, len(graph.Nodes))
	for _, node := range graph.Nodes {
		prByID[node.NodeID] = node.PageRank
		if node.PageRank > maxPR {
			maxPR = node.PageRank
		}
	}
	if maxPR == 0 {
		maxPR = 1
	}

	// Build adjacency map: nodeID → []nodeID from graph links.
	adjacency := make(map[string][]string)
	for _, link := range graph.Links {
		adjacency[link.Source] = append(adjacency[link.Source], link.Target)
	}

	// Build set of changed files (slash-normalized).
	changedSet := make(map[string]bool, len(body.ChangedFiles))
	for _, cf := range body.ChangedFiles {
		changedSet[filepath.ToSlash(cf)] = true
	}

	// Direct risks.
	type directRisk struct {
		Path            string  `json:"path"`
		RiskScore       float64 `json:"risk_score"`
		TemporalHotspot float64 `json:"temporal_hotspot"`
		Centrality      float64 `json:"centrality"`
	}
	directRisks := make([]directRisk, 0)
	for _, cf := range body.ChangedFiles {
		cfSlash := filepath.ToSlash(cf)
		f, ok := fileMap[cfSlash]
		if !ok {
			continue
		}
		churnPct := churnPctFor(f.Churn)
		pr := prByID[cfSlash]
		centrality := pr / maxPR

		riskScore := (churnPct/100*0.5 + f.Ownership*0.3 + centrality*0.2)
		if riskScore > 1 {
			riskScore = 1
		}
		directRisks = append(directRisks, directRisk{
			Path:            cfSlash,
			RiskScore:       riskScore,
			TemporalHotspot: churnPct / 100,
			Centrality:      centrality,
		})
	}

	// Transitive affected: BFS from changed files via adjacency edges.
	type transitiveItem struct {
		Path  string `json:"path"`
		Depth int    `json:"depth"`
	}
	transitiveAffected := make([]transitiveItem, 0)
	visited := make(map[string]bool)
	for cf := range changedSet {
		visited[cf] = true
	}
	type bfsNode struct {
		id    string
		depth int
	}
	queue := make([]bfsNode, 0)
	for cf := range changedSet {
		queue = append(queue, bfsNode{id: cf, depth: 0})
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= body.MaxDepth {
			continue
		}
		for _, neighbor := range adjacency[cur.id] {
			if visited[neighbor] {
				continue
			}
			visited[neighbor] = true
			transitiveAffected = append(transitiveAffected, transitiveItem{Path: neighbor, Depth: cur.depth + 1})
			queue = append(queue, bfsNode{id: neighbor, depth: cur.depth + 1})
		}
	}

	// Recommended reviewers: aggregate PrimaryAuthors from direct + transitive files.
	type reviewerAgg struct {
		files      int
		ownershipSum float64
	}
	reviewers := make(map[string]*reviewerAgg)
	collectReviewer := func(path string) {
		f, ok := fileMap[path]
		if !ok {
			return
		}
		if f.PrimaryAuthor == "" {
			return
		}
		agg, exists := reviewers[f.PrimaryAuthor]
		if !exists {
			agg = &reviewerAgg{}
			reviewers[f.PrimaryAuthor] = agg
		}
		agg.files++
		agg.ownershipSum += f.Ownership
	}
	for _, cf := range body.ChangedFiles {
		collectReviewer(filepath.ToSlash(cf))
	}
	for _, t := range transitiveAffected {
		collectReviewer(t.Path)
	}
	type reviewerItem struct {
		Email        string  `json:"email"`
		Files        int     `json:"files"`
		OwnershipPct float64 `json:"ownership_pct"`
	}
	reviewerList := make([]reviewerItem, 0, len(reviewers))
	for email, agg := range reviewers {
		avgOwnership := 0.0
		if agg.files > 0 {
			avgOwnership = agg.ownershipSum / float64(agg.files)
		}
		reviewerList = append(reviewerList, reviewerItem{
			Email:        email,
			Files:        agg.files,
			OwnershipPct: avgOwnership,
		})
	}
	sort.Slice(reviewerList, func(i, j int) bool {
		return reviewerList[i].Files > reviewerList[j].Files
	})
	if len(reviewerList) > 5 {
		reviewerList = reviewerList[:5]
	}

	// Test gaps: changed non-test files with no test file neighbor.
	testGaps := make([]string, 0)
	for _, cf := range body.ChangedFiles {
		cfSlash := filepath.ToSlash(cf)
		// Skip files that are themselves test files.
		if strings.HasSuffix(cfSlash, "_test.go") ||
			strings.Contains(cfSlash, "/test/") ||
			strings.Contains(cfSlash, "test_") {
			continue
		}
		hasTest := false
		for _, neighbor := range adjacency[cfSlash] {
			if strings.HasSuffix(neighbor, "_test.go") {
				hasTest = true
				break
			}
		}
		if !hasTest {
			testGaps = append(testGaps, cfSlash)
		}
	}

	// Overall risk score: average of direct risks * 10.
	overallRisk := 0.0
	if len(directRisks) > 0 {
		sum := 0.0
		for _, dr := range directRisks {
			sum += dr.RiskScore
		}
		overallRisk = (sum / float64(len(directRisks))) * 10
	}

	s.json(w, http.StatusOK, map[string]any{
		"direct_risks":          directRisks,
		"transitive_affected":   transitiveAffected,
		"cochange_warnings":     []any{},
		"recommended_reviewers": reviewerList,
		"test_gaps":             testGaps,
		"overall_risk_score":    overallRisk,
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

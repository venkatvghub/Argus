package server

import (
	"time"

	"github.com/venkatvghub/argus/pkg/argus"
	"github.com/venkatvghub/argus/pkg/models"
)

// RepoResponse is the frontend-compatible shape for a repository.
type RepoResponse struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	LocalPath     string    `json:"local_path"`
	URL           string    `json:"url,omitempty"`
	DefaultBranch string    `json:"default_branch"`
	HeadCommit    string    `json:"head_commit,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func repoToResponse(r models.Repository) RepoResponse {
	defaultBranch := r.DefaultBranch
	updatedAt := r.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = r.CreatedAt
	}
	return RepoResponse{
		ID:            r.ID,
		Name:          r.Name,
		LocalPath:     r.Path,
		URL:           r.URL,
		DefaultBranch: defaultBranch,
		HeadCommit:    r.LastCommit,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     updatedAt,
	}
}

// JobResponse is the frontend-compatible shape for a job.
// Status "in_progress" is mapped to "running" for frontend compatibility.
type JobResponse struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id,omitempty"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	Progress     string    `json:"progress"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func jobToResponse(j models.Job) JobResponse {
	status := string(j.Status)
	if status == string(models.JobStatusInProgress) {
		status = "running"
	}
	return JobResponse{
		ID:           j.ID,
		RepositoryID: j.RepoID,
		Type:         j.Type,
		Status:       status,
		Progress:     j.Progress,
		ErrorMessage: j.Error,
		CreatedAt:    j.CreatedAt,
		UpdatedAt:    j.UpdatedAt,
	}
}

// RepoStatsResponse is the frontend-compatible shape for repo stats.
type RepoStatsResponse struct {
	FileCount       int     `json:"file_count"`
	SymbolCount     int     `json:"symbol_count"`
	EntryPointCount int     `json:"entry_point_count"`
	DocCoveragePct  float64 `json:"doc_coverage_pct"`
	FreshnessScore  float64 `json:"freshness_score"`
	DeadExportCount int     `json:"dead_export_count"`
}

// HealthFileMetric is the per-file health shape the frontend expects.
type HealthFileMetric struct {
	FilePath        string   `json:"file_path"`
	Score           float64  `json:"score"`
	MaxCCN          int      `json:"max_ccn"`
	MaxNesting      int      `json:"max_nesting"`
	NLOC            int      `json:"nloc"`
	HasTestFile     bool     `json:"has_test_file"`
	LineCoveragePct *float64 `json:"line_coverage_pct"`
	Module          *string  `json:"module"`
	DuplicationPct  *float64 `json:"duplication_pct"`
}

// HealthFinding is the per-finding shape the frontend expects.
type HealthFinding struct {
	ID            string         `json:"id"`
	FilePath      string         `json:"file_path"`
	BiomarkerType string         `json:"biomarker_type"`
	Severity      string         `json:"severity"`
	FunctionName  *string        `json:"function_name"`
	LineStart     *int           `json:"line_start"`
	LineEnd       *int           `json:"line_end"`
	HealthImpact  float64        `json:"health_impact"`
	Reason        string         `json:"reason"`
	Details       map[string]any `json:"details"`
	Status        string         `json:"status"`
}

// HealthOverviewSummary is the summary sub-object the frontend reads.
type HealthOverviewSummary struct {
	FileCount           int     `json:"file_count"`
	AverageHealth       float64 `json:"average_health"`
	HotspotHealth       *float64 `json:"hotspot_health"`
	WorstPerformerPath  *string `json:"worst_performer_path"`
	WorstPerformerScore *float64 `json:"worst_performer_score"`
	OpenFindings        int     `json:"open_findings"`
	SeverityBreakdown   map[string]int `json:"severity_breakdown"`
}

// HealthOverviewMeta holds indexing metadata.
type HealthOverviewMeta struct {
	LastIndexedAt *string `json:"last_indexed_at"`
	HeadCommit    *string `json:"head_commit"`
	SnapshotCount int     `json:"snapshot_count"`
}

// HealthOverviewResponse is the complete health/overview response.
type HealthOverviewResponse struct {
	Summary     HealthOverviewSummary `json:"summary"`
	Files       []HealthFileMetric    `json:"files"`
	TopFindings []HealthFinding       `json:"top_findings"`
	Meta        *HealthOverviewMeta   `json:"meta,omitempty"`
}

// HealthFilesResponse wraps the paginated files list.
type HealthFilesResponse struct {
	Total  int                `json:"total"`
	Offset int                `json:"offset"`
	Limit  int                `json:"limit"`
	Files  []HealthFileMetric `json:"files"`
}

// HealthTrendSummary is the trend summary sub-object.
type HealthTrendSummary struct {
	CurrentHotspotHealth  float64  `json:"current_hotspot_health"`
	CurrentAverageHealth  float64  `json:"current_average_health"`
	PreviousHotspotHealth *float64 `json:"previous_hotspot_health"`
	PreviousAverageHealth *float64 `json:"previous_average_health"`
	HotspotDelta          *float64 `json:"hotspot_delta"`
	AverageDelta          *float64 `json:"average_delta"`
}

// HealthTrendResponse is the health/trend response.
type HealthTrendResponse struct {
	History       []any              `json:"history"`
	Summary       HealthTrendSummary `json:"summary"`
	Alerts        []any              `json:"alerts"`
	FileDeltas    []any              `json:"file_deltas"`
	SnapshotCount int                `json:"snapshot_count"`
}

// argusHealthFindingToAPI converts an internal argus.HealthFinding to the API shape.
func argusHealthFindingToAPI(f argus.HealthFinding) HealthFinding {
	var lineStart *int
	if f.Line > 0 {
		line := f.Line
		lineStart = &line
	}
	return HealthFinding{
		ID:            f.ID,
		FilePath:      f.File,
		BiomarkerType: f.Type,
		Severity:      f.Severity,
		FunctionName:  nil,
		LineStart:     lineStart,
		LineEnd:       nil,
		HealthImpact:  f.Deduction,
		Reason:        f.Message,
		Details:       map[string]any{},
		Status:        f.Status,
	}
}

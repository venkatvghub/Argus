package argus

import (
	"context"
	"crypto/sha256"
	"fmt"
)

// HealthFinding is the API shape for a single marker result.
type HealthFinding struct {
	ID         string  `json:"id"`          // sha256(file+type+line)[:12]
	RepoID     string  `json:"repo_id"`
	File       string  `json:"file_path"`
	Type       string  `json:"type"`
	Severity   string  `json:"severity"`
	Message    string  `json:"message"`
	Line       int     `json:"line"`
	Deduction  float64 `json:"health_impact"`
	Status     string  `json:"status"`     // stub "open"
	Suggestion string  `json:"suggestion"`
}

// HealthOverview summarises repo health for the dashboard.
type HealthOverview struct {
	RepoID        string  `json:"repo_id"`
	OverallScore  float64 `json:"overall_score"`
	FindingCount  int     `json:"finding_count"`
	CriticalCount int     `json:"critical_count"`
	WarningCount  int     `json:"warning_count"`
	InfoCount     int     `json:"info_count"`
	MaxCCN        int     `json:"max_ccn"`    // stub 0
	MaxNesting    int     `json:"max_nesting"` // stub 0
	TotalNLOC     int     `json:"total_nloc"`  // stub 0
}

// HealthFile summarises health for one file.
type HealthFile struct {
	Path         string  `json:"path"`
	Score        float64 `json:"score"`
	FindingCount int     `json:"finding_count"`
	HasTestFile  bool    `json:"has_test_file"` // stub false
	Language     string  `json:"language"`
}

// GetHealthOverview returns aggregate health info for a repository.
func (i *Instance) GetHealthOverview(ctx context.Context, repoID string) (HealthOverview, error) {
	markers, err := i.GetRepoMarkers(ctx, repoID)
	if err != nil {
		return HealthOverview{}, err
	}

	score, _ := i.GetRepoScore(ctx, repoID)

	overview := HealthOverview{
		RepoID:       repoID,
		OverallScore: score,
		FindingCount: len(markers),
	}
	for _, m := range markers {
		switch m.Severity {
		case "critical":
			overview.CriticalCount++
		case "warning":
			overview.WarningCount++
		default:
			overview.InfoCount++
		}
	}
	return overview, nil
}

// GetHealthFiles returns per-file health summaries for a repository.
func (i *Instance) GetHealthFiles(ctx context.Context, repoID string) ([]HealthFile, error) {
	files, err := i.GetRepoFiles(ctx, repoID)
	if err != nil {
		return nil, err
	}

	markers, err := i.GetRepoMarkers(ctx, repoID)
	if err != nil {
		return nil, err
	}

	// Count markers per file.
	countByFile := make(map[string]int)
	for _, m := range markers {
		countByFile[m.File]++
	}

	// Build per-file health scores.
	result := make([]HealthFile, 0, len(files))
	for _, f := range files {
		score, _ := i.GetFileScore(ctx, repoID, f.Path)
		result = append(result, HealthFile{
			Path:         f.Path,
			Score:        score.Final,
			FindingCount: countByFile[f.Path],
			Language:     f.Language,
		})
	}
	return result, nil
}

// GetHealthFindings returns all health findings for a repository.
func (i *Instance) GetHealthFindings(ctx context.Context, repoID string) ([]HealthFinding, error) {
	markers, err := i.GetRepoMarkers(ctx, repoID)
	if err != nil {
		return nil, err
	}

	findings := make([]HealthFinding, 0, len(markers))
	for _, m := range markers {
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(m.File+m.Type+fmt.Sprint(m.Line))))[:12]
		findings = append(findings, HealthFinding{
			ID:         id,
			RepoID:     repoID,
			File:       m.File,
			Type:       m.Type,
			Severity:   m.Severity,
			Message:    m.Message,
			Line:       m.Line,
			Deduction:  m.Deduction,
			Status:     "open",
			Suggestion: m.Suggestion,
		})
	}
	return findings, nil
}


// Package models defines the core domain entities for Argus.
// These models are used for data exchange between ingestion, analysis,
// and persistence layers.
package models

import "time"

// Repository represents a source code repository under analysis.
type Repository struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	URL       string    `json:"url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// FileNode represents a single file or directory within a repository.
type FileNode struct {
	Path      string    `json:"path"`
	IsFile    bool      `json:"is_file"`
	Size      int64     `json:"size"`
	LastMod   time.Time `json:"last_mod"`
	Language  string    `json:"language,omitempty"`
	Churn     int       `json:"churn"`     // Number of commits touching this file
	Ownership float64   `json:"ownership"` // Percentage of commits by the top author
}

// CommitInfo captures metadata about a specific Git commit.
type CommitInfo struct {
	Hash      string    `json:"hash"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// SymbolType represents the category of a code symbol.
type SymbolType string

const (
	SymbolFunction SymbolType = "function"
	SymbolClass    SymbolType = "class"
	SymbolVariable SymbolType = "variable"
	SymbolImport   SymbolType = "import"
)

// Symbol represents a semantic unit in the source code.
type Symbol struct {
	Name     string     `json:"name"`
	Type     SymbolType `json:"type"`
	Line     int        `json:"line"`
	EndLine  int        `json:"end_line"`
	FilePath string     `json:"file_path"`
}

// Relation represents a connection between two symbols (e.g., call, inheritance).
type Relation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// ScoreCategory identifies which deduction cap a marker counts toward.
type ScoreCategory string

const (
	// ScoreCatStructural covers brain_method, nested_complexity, bumpy_road (cap −3.5).
	ScoreCatStructural ScoreCategory = "structural_complexity"
	// ScoreCatSize covers complex_method, large_method, primitive_obsession (cap −2.0).
	ScoreCatSize ScoreCategory = "size_api_complexity"
	// ScoreCatDuplication covers dry_violation (cap −1.5).
	ScoreCatDuplication ScoreCategory = "duplication"
	// ScoreCatTestCoverage covers untested_hotspot, coverage_gap (cap −2.0).
	ScoreCatTestCoverage ScoreCategory = "test_coverage"
	// ScoreCatOrg covers developer_congestion, knowledge_loss (cap −1.0).
	ScoreCatOrg ScoreCategory = "organizational_risk"
	// ScoreCatDeadCode covers dead_code, unreferenced_symbols, zombie_exports (cap −1.0).
	ScoreCatDeadCode ScoreCategory = "dead_code"
	// ScoreCatCompliance covers DPDP, RBI, concurrency, AppSec markers (uncapped).
	ScoreCatCompliance ScoreCategory = "compliance_appsec"
	// ScoreCatEfficiency covers AI-agent efficiency markers (uncapped).
	ScoreCatEfficiency ScoreCategory = "ai_efficiency"
)

// CategoryCaps maps each ScoreCategory to its maximum total deduction.
// Compliance and Efficiency categories have no structural cap.
var CategoryCaps = map[ScoreCategory]float64{
	ScoreCatStructural:   3.5,
	ScoreCatSize:         2.0,
	ScoreCatDuplication:  1.5,
	ScoreCatTestCoverage: 2.0,
	ScoreCatOrg:          1.0,
	ScoreCatDeadCode:     1.0,
}

// Marker represents a biomarker finding on a file or symbol.
// Deduction is score points subtracted (0 = informational only).
// Category determines which cap applies when aggregating deductions per file.
type Marker struct {
	Type      string        `json:"type"`
	Severity  string        `json:"severity"`
	Message   string        `json:"message"`
	File      string        `json:"file"`
	Line      int           `json:"line"`
	Deduction float64       `json:"deduction,omitempty"`
	Category  ScoreCategory `json:"category,omitempty"`
}

// FileScore holds the computed health score for a single file.
// Base is always 10.0; Final is clamped to [1.0, 10.0].
type FileScore struct {
	File        string                    `json:"file"`
	Base        float64                   `json:"base"`       // always 10.0
	Final       float64                   `json:"final"`      // clamped [1.0, 10.0]
	Deductions  map[ScoreCategory]float64 `json:"deductions"` // per-category totals after cap
	MarkerCount int                       `json:"marker_count"`
}

type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusInProgress JobStatus = "in_progress"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

type Job struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // e.g., "analysis"
	Status    JobStatus `json:"status"`
	Progress  string    `json:"progress"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

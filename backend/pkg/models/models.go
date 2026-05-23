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

// Marker represents a regulatory or efficiency finding.
type Marker struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	File     string `json:"file"`
	Line     int    `json:"line"`
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

// Package persistence provides the data access layer for Argus.
package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/venkatvghub/argus/pkg/models"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB represents a persistent store instance.
type DB struct {
	*sql.DB
}

// New initializes a new SQLite connection at the specified path and
// runs pending migrations.
func New(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), defaultDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// WAL mode + busy timeout: allow concurrent readers, serialise writers, avoid SQLITE_BUSY.
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	// WAL mode allows concurrent readers; cap idle connections to limit overhead.
	db.SetMaxOpenConns(runtime.NumCPU())
	db.SetMaxIdleConns(1)

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &DB{db}, nil
}

// Close gracefully shuts down the database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}

// UpsertRepository inserts or updates a repository record.
func (db *DB) UpsertRepository(ctx context.Context, repo models.Repository) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO repositories (id, name, local_path, last_commit, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			local_path=excluded.local_path,
			last_commit=excluded.last_commit,
			updated_at=CURRENT_TIMESTAMP`,
		repo.ID, repo.Name, repo.Path, repo.LastCommit)
	return err
}

// GetRepository returns a single repository by ID. Returns sql.ErrNoRows if not found.
func (db *DB) GetRepository(ctx context.Context, repoID string) (models.Repository, error) {
	var r models.Repository
	err := db.QueryRowContext(ctx,
		"SELECT id, name, local_path, last_commit, created_at FROM repositories WHERE id = ?",
		repoID,
	).Scan(&r.ID, &r.Name, &r.Path, &r.LastCommit, &r.CreatedAt)
	return r, err
}

// ListRepositories returns all indexed repositories.
func (db *DB) ListRepositories(ctx context.Context) ([]models.Repository, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, name, local_path, last_commit, created_at FROM repositories")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []models.Repository = []models.Repository{}
	for rows.Next() {
		var r models.Repository
		if err := rows.Scan(&r.ID, &r.Name, &r.Path, &r.LastCommit, &r.CreatedAt); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, nil
}

// UpsertMarkers replaces all markers for the given repo with the provided slice.
func (db *DB) UpsertMarkers(ctx context.Context, repoID string, markers []models.Marker) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, "DELETE FROM markers WHERE repo_id = ?", repoID); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO markers (repo_id, file, type, severity, message, line, deduction, category, suggestion)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range markers {
		if _, err := stmt.ExecContext(ctx, repoID, m.File, m.Type, m.Severity, m.Message, m.Line, m.Deduction, string(m.Category), m.Suggestion); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LoadAllMarkers reads all persisted markers grouped by repo ID.
func (db *DB) LoadAllMarkers(ctx context.Context) (map[string][]models.Marker, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT repo_id, file, type, severity, message, line, deduction, category, suggestion FROM markers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]models.Marker)
	for rows.Next() {
		var repoID string
		var m models.Marker
		var cat string
		if err := rows.Scan(&repoID, &m.File, &m.Type, &m.Severity, &m.Message, &m.Line, &m.Deduction, &cat, &m.Suggestion); err != nil {
			return nil, err
		}
		m.Category = models.ScoreCategory(cat)
		result[repoID] = append(result[repoID], m)
	}
	return result, rows.Err()
}

// CreateWikiJob inserts a new wiki generation job and returns its ID.
// The ID is a sha256-based short hash of repoID + current time.
func (db *DB) CreateWikiJob(ctx context.Context, repoID string, totalPages int) (string, error) {
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(repoID+time.Now().UTC().String())))[:16]
	_, err := db.ExecContext(ctx, `
		INSERT INTO wiki_jobs (id, repo_id, status, total_pages, created_at, updated_at)
		VALUES (?, ?, 'pending', ?, datetime('now'), datetime('now'))`,
		id, repoID, totalPages)
	return id, err
}

// UpdateWikiJobStatus updates the status of a wiki generation job.
func (db *DB) UpdateWikiJobStatus(ctx context.Context, jobID string, status models.WikiJobStatus) error {
	_, err := db.ExecContext(ctx, `
		UPDATE wiki_jobs SET status = ?, updated_at = datetime('now') WHERE id = ?`,
		string(status), jobID)
	return err
}

// MarkWikiPageComplete records a page as completed in a wiki generation job.
func (db *DB) MarkWikiPageComplete(ctx context.Context, jobID, pageID string) error {
	_, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO wiki_job_pages (job_id, page_id) VALUES (?, ?)`,
		jobID, pageID)
	return err
}

// GetCompletedWikiPages returns the set of completed page IDs for a job.
func (db *DB) GetCompletedWikiPages(ctx context.Context, jobID string) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, `SELECT page_id FROM wiki_job_pages WHERE job_id = ?`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]struct{})
	for rows.Next() {
		var pageID string
		if err := rows.Scan(&pageID); err != nil {
			return nil, err
		}
		result[pageID] = struct{}{}
	}
	return result, rows.Err()
}

// GetWikiJob returns a wiki job by ID.
func (db *DB) GetWikiJob(ctx context.Context, jobID string) (models.WikiJob, error) {
	var j models.WikiJob
	var status, createdAt, updatedAt string
	err := db.QueryRowContext(ctx, `
		SELECT id, repo_id, status, total_pages, created_at, updated_at
		FROM wiki_jobs WHERE id = ?`, jobID).
		Scan(&j.ID, &j.RepoID, &status, &j.TotalPages, &createdAt, &updatedAt)
	if err != nil {
		return j, err
	}
	j.Status = models.WikiJobStatus(status)
	var parseErr error
	if j.CreatedAt, parseErr = parseSQLiteTimestamp("created_at", createdAt); parseErr != nil {
		return j, fmt.Errorf("wiki job %q: %w", jobID, parseErr)
	}
	if j.UpdatedAt, parseErr = parseSQLiteTimestamp("updated_at", updatedAt); parseErr != nil {
		return j, fmt.Errorf("wiki job %q: %w", jobID, parseErr)
	}
	return j, nil
}

// ListWikiJobs returns all wiki jobs for a repository, ordered by created_at desc.
func (db *DB) ListWikiJobs(ctx context.Context, repoID string) ([]models.WikiJob, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, repo_id, status, total_pages, created_at, updated_at
		FROM wiki_jobs WHERE repo_id = ? ORDER BY created_at DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []models.WikiJob
	for rows.Next() {
		var j models.WikiJob
		var status, createdAt, updatedAt string
		if err := rows.Scan(&j.ID, &j.RepoID, &status, &j.TotalPages, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		j.Status = models.WikiJobStatus(status)
		if j.CreatedAt, err = parseSQLiteTimestamp("created_at", createdAt); err != nil {
			return nil, fmt.Errorf("wiki job %q: %w", j.ID, err)
		}
		if j.UpdatedAt, err = parseSQLiteTimestamp("updated_at", updatedAt); err != nil {
			return nil, fmt.Errorf("wiki job %q: %w", j.ID, err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpsertWikiPage inserts or replaces a generated wiki page.
func (db *DB) UpsertWikiPage(ctx context.Context, page models.WikiPage) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO wiki_pages (id, repo_id, job_id, type, subject, content, level, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			content = excluded.content,
			updated_at = datetime('now')`,
		page.ID, page.RepoID, page.JobID, page.Type, page.Subject, page.Content, page.Level)
	return err
}

// ListWikiPages returns all wiki pages for a repository.
func (db *DB) ListWikiPages(ctx context.Context, repoID string) ([]models.WikiPage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, repo_id, job_id, type, subject, content, level, created_at, updated_at
		FROM wiki_pages WHERE repo_id = ? ORDER BY level, type, subject`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []models.WikiPage
	for rows.Next() {
		var p models.WikiPage
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.RepoID, &p.JobID, &p.Type, &p.Subject, &p.Content, &p.Level, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if p.CreatedAt, err = parseSQLiteTimestamp("created_at", createdAt); err != nil {
			return nil, fmt.Errorf("wiki page %q: %w", p.ID, err)
		}
		if p.UpdatedAt, err = parseSQLiteTimestamp("updated_at", updatedAt); err != nil {
			return nil, fmt.Errorf("wiki page %q: %w", p.ID, err)
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

// GetWikiPage returns a single wiki page by ID.
func (db *DB) GetWikiPage(ctx context.Context, pageID string) (models.WikiPage, error) {
	var p models.WikiPage
	var createdAt, updatedAt string
	err := db.QueryRowContext(ctx, `
		SELECT id, repo_id, job_id, type, subject, content, level, created_at, updated_at
		FROM wiki_pages WHERE id = ?`, pageID).
		Scan(&p.ID, &p.RepoID, &p.JobID, &p.Type, &p.Subject, &p.Content, &p.Level, &createdAt, &updatedAt)
	if err != nil {
		return p, err
	}
	if p.CreatedAt, err = parseSQLiteTimestamp("created_at", createdAt); err != nil {
		return p, fmt.Errorf("wiki page %q: %w", pageID, err)
	}
	if p.UpdatedAt, err = parseSQLiteTimestamp("updated_at", updatedAt); err != nil {
		return p, fmt.Errorf("wiki page %q: %w", pageID, err)
	}
	return p, nil
}

const sqliteTimestampLayout = "2006-01-02 15:04:05"

var sqliteTimestampLayouts = []string{
	sqliteTimestampLayout,
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999Z",
	"2006-01-02T15:04:05Z",
}

func parseSQLiteTimestamp(field, raw string) (time.Time, error) {
	for _, layout := range sqliteTimestampLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse %s timestamp %q: unrecognised format", field, raw)
}

// DeleteRepository removes a repository record by ID.
func (db *DB) DeleteRepository(ctx context.Context, repoID string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM repositories WHERE id = ?", repoID)
	return err
}

// UpsertRepoFiles replaces all file records for the given repo.
func (db *DB) UpsertRepoFiles(ctx context.Context, repoID string, files []models.FileNode) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, "DELETE FROM repo_files WHERE repo_id = ?", repoID); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO repo_files (repo_id, path, language, churn, ownership, author_count, line_coverage, size, primary_author_last_commit)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, f := range files {
		var pac *time.Time
		if !f.PrimaryAuthorLastCommit.IsZero() {
			pac = &f.PrimaryAuthorLastCommit
		}
		if _, err := stmt.ExecContext(ctx, repoID, f.Path, f.Language, f.Churn, f.Ownership, f.AuthorCount, f.LineCoverage, f.Size, pac); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetRepoFiles returns all file records for a repository.
func (db *DB) GetRepoFiles(ctx context.Context, repoID string) ([]models.FileNode, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT path, language, churn, ownership, author_count, line_coverage, size, primary_author_last_commit
		FROM repo_files WHERE repo_id = ? ORDER BY churn DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var files []models.FileNode
	for rows.Next() {
		var f models.FileNode
		var pac *string
		f.IsFile = true
		if err := rows.Scan(&f.Path, &f.Language, &f.Churn, &f.Ownership, &f.AuthorCount, &f.LineCoverage, &f.Size, &pac); err != nil {
			return nil, err
		}
		if pac != nil {
			if t, parseErr := parseSQLiteTimestamp("primary_author_last_commit", *pac); parseErr == nil {
				f.PrimaryAuthorLastCommit = t
			}
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// UpsertRepoSymbols replaces all symbol records for the given repo.
func (db *DB) UpsertRepoSymbols(ctx context.Context, repoID string, symbols []models.Symbol) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, "DELETE FROM repo_symbols WHERE repo_id = ?", repoID); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO repo_symbols (repo_id, name, type, file_path, line, end_line)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range symbols {
		if _, err := stmt.ExecContext(ctx, repoID, s.Name, string(s.Type), s.FilePath, s.Line, s.EndLine); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetRepoSymbols returns all symbol records for a repository.
func (db *DB) GetRepoSymbols(ctx context.Context, repoID string) ([]models.Symbol, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, type, file_path, line, end_line
		FROM repo_symbols WHERE repo_id = ?`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var symbols []models.Symbol
	for rows.Next() {
		var s models.Symbol
		var typ string
		if err := rows.Scan(&s.Name, &typ, &s.FilePath, &s.Line, &s.EndLine); err != nil {
			return nil, err
		}
		s.Type = models.SymbolType(typ)
		symbols = append(symbols, s)
	}
	return symbols, rows.Err()
}

// LLMCostRecord is a single LLM call cost entry to persist.
type LLMCostRecord struct {
	RepoID       string
	Model        string
	Operation    string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// CostGroup is an aggregated cost row returned by ListLLMCosts.
type CostGroup struct {
	Group        string  `json:"group"`
	Calls        int     `json:"calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// CostSummary is the aggregate cost totals for a repo.
type CostSummary struct {
	TotalCostUSD      float64  `json:"total_cost_usd"`
	TotalCalls        int      `json:"total_calls"`
	TotalInputTokens  int      `json:"total_input_tokens"`
	TotalOutputTokens int      `json:"total_output_tokens"`
	Since             *string  `json:"since"`
}

// RecordLLMCost inserts a cost record for an LLM call.
func (db *DB) RecordLLMCost(ctx context.Context, rec LLMCostRecord) error {
	op := rec.Operation
	if op == "" {
		op = "chat"
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO llm_costs (repo_id, model, operation, input_tokens, output_tokens, cost_usd)
		VALUES (?, ?, ?, ?, ?, ?)`,
		rec.RepoID, rec.Model, op, rec.InputTokens, rec.OutputTokens, rec.CostUSD)
	return err
}

// ListLLMCosts returns aggregated cost data for a repo, grouped by "day", "model", or "operation".
func (db *DB) ListLLMCosts(ctx context.Context, repoID, by string) ([]CostGroup, error) {
	var groupExpr string
	switch by {
	case "model":
		groupExpr = "model"
	case "operation":
		groupExpr = "operation"
	default:
		groupExpr = "strftime('%Y-%m-%d', called_at)"
	}
	q := fmt.Sprintf(`
		SELECT %s as grp, COUNT(*) as calls, SUM(input_tokens), SUM(output_tokens), SUM(cost_usd)
		FROM llm_costs WHERE repo_id = ?
		GROUP BY grp ORDER BY grp DESC`, groupExpr)
	rows, err := db.QueryContext(ctx, q, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []CostGroup
	for rows.Next() {
		var g CostGroup
		if err := rows.Scan(&g.Group, &g.Calls, &g.InputTokens, &g.OutputTokens, &g.CostUSD); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// GetLLMCostSummary returns aggregate totals for all LLM calls for a repo.
func (db *DB) GetLLMCostSummary(ctx context.Context, repoID string) (CostSummary, error) {
	var summary CostSummary
	var since *string
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cost_usd),0.0), MIN(called_at)
		FROM llm_costs WHERE repo_id = ?`, repoID).
		Scan(&summary.TotalCalls, &summary.TotalInputTokens, &summary.TotalOutputTokens, &summary.TotalCostUSD, &since)
	if err != nil {
		return summary, err
	}
	summary.Since = since
	return summary, nil
}

// CreateJob inserts a new job record.
func (db *DB) CreateJob(ctx context.Context, job models.Job) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO jobs (id, repo_id, type, status, progress, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`,
		job.ID, job.RepoID, job.Type, string(job.Status), job.Progress, job.Error)
	return err
}

// GetJob returns a job by ID.
func (db *DB) GetJob(ctx context.Context, jobID string) (models.Job, error) {
	var j models.Job
	var status, createdAt, updatedAt string
	err := db.QueryRowContext(ctx, `
		SELECT id, repo_id, type, status, progress, error, created_at, updated_at
		FROM jobs WHERE id = ?`, jobID).
		Scan(&j.ID, &j.RepoID, &j.Type, &status, &j.Progress, &j.Error, &createdAt, &updatedAt)
	if err != nil {
		return j, err
	}
	j.Status = models.JobStatus(status)
	var parseErr error
	if j.CreatedAt, parseErr = parseSQLiteTimestamp("created_at", createdAt); parseErr != nil {
		return j, fmt.Errorf("job %q: %w", jobID, parseErr)
	}
	if j.UpdatedAt, parseErr = parseSQLiteTimestamp("updated_at", updatedAt); parseErr != nil {
		return j, fmt.Errorf("job %q: %w", jobID, parseErr)
	}
	return j, nil
}

// ListJobs returns jobs; when repoID is empty, all jobs are returned.
func (db *DB) ListJobs(ctx context.Context, repoID string) ([]models.Job, error) {
	var rows *sql.Rows
	var err error
	if repoID == "" {
		rows, err = db.QueryContext(ctx, `
			SELECT id, repo_id, type, status, progress, error, created_at, updated_at
			FROM jobs ORDER BY created_at DESC`)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT id, repo_id, type, status, progress, error, created_at, updated_at
			FROM jobs WHERE repo_id = ? ORDER BY created_at DESC`, repoID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []models.Job
	for rows.Next() {
		var j models.Job
		var status, createdAt, updatedAt string
		if err := rows.Scan(&j.ID, &j.RepoID, &j.Type, &status, &j.Progress, &j.Error, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		j.Status = models.JobStatus(status)
		if j.CreatedAt, err = parseSQLiteTimestamp("created_at", createdAt); err != nil {
			return nil, fmt.Errorf("job %q: %w", j.ID, err)
		}
		if j.UpdatedAt, err = parseSQLiteTimestamp("updated_at", updatedAt); err != nil {
			return nil, fmt.Errorf("job %q: %w", j.ID, err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateJobStatus updates status, progress, and error fields of a job.
func (db *DB) UpdateJobStatus(ctx context.Context, jobID, status, progress, errMsg string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE jobs SET status = ?, progress = ?, error = ?,
		updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
		status, progress, errMsg, jobID)
	return err
}

// CreateConversation inserts a new conversation record.
func (db *DB) CreateConversation(ctx context.Context, conv models.Conversation) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO conversations (id, repo_id, title, message_count, created_at, updated_at)
		VALUES (?, ?, ?, 0, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`,
		conv.ID, conv.RepositoryID, conv.Title)
	return err
}

// GetConversation returns a conversation by ID.
func (db *DB) GetConversation(ctx context.Context, convID string) (models.Conversation, error) {
	var c models.Conversation
	var createdAt, updatedAt string
	err := db.QueryRowContext(ctx, `
		SELECT id, repo_id, title, message_count, created_at, updated_at
		FROM conversations WHERE id = ?`, convID).
		Scan(&c.ID, &c.RepositoryID, &c.Title, &c.MessageCount, &createdAt, &updatedAt)
	if err != nil {
		return c, err
	}
	var parseErr error
	if c.CreatedAt, parseErr = parseSQLiteTimestamp("created_at", createdAt); parseErr != nil {
		return c, fmt.Errorf("conversation %q: %w", convID, parseErr)
	}
	if c.UpdatedAt, parseErr = parseSQLiteTimestamp("updated_at", updatedAt); parseErr != nil {
		return c, fmt.Errorf("conversation %q: %w", convID, parseErr)
	}
	return c, nil
}

// ListConversations returns all conversations for a repository.
func (db *DB) ListConversations(ctx context.Context, repoID string) ([]models.Conversation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, repo_id, title, message_count, created_at, updated_at
		FROM conversations WHERE repo_id = ? ORDER BY updated_at DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var convs []models.Conversation
	for rows.Next() {
		var c models.Conversation
		var createdAt, updatedAt string
		if err := rows.Scan(&c.ID, &c.RepositoryID, &c.Title, &c.MessageCount, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if c.CreatedAt, err = parseSQLiteTimestamp("created_at", createdAt); err != nil {
			return nil, fmt.Errorf("conversation %q: %w", c.ID, err)
		}
		if c.UpdatedAt, err = parseSQLiteTimestamp("updated_at", updatedAt); err != nil {
			return nil, fmt.Errorf("conversation %q: %w", c.ID, err)
		}
		convs = append(convs, c)
	}
	return convs, rows.Err()
}

// DeleteConversation removes a conversation and its messages (via FK cascade).
func (db *DB) DeleteConversation(ctx context.Context, convID string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM conversations WHERE id = ?", convID)
	return err
}

// IncrementMessageCount increments the message_count for a conversation by 1.
func (db *DB) IncrementMessageCount(ctx context.Context, convID string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE conversations SET message_count = message_count + 1,
		updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, convID)
	return err
}

// CreateChatMessage inserts a new chat message.
func (db *DB) CreateChatMessage(ctx context.Context, msg models.ChatMessage) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO chat_messages (id, conversation_id, role, content, created_at)
		VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`,
		msg.ID, msg.ConversationID, msg.Role, msg.Content)
	return err
}

// ListChatMessages returns all messages for a conversation in chronological order.
func (db *DB) ListChatMessages(ctx context.Context, convID string) ([]models.ChatMessage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, conversation_id, role, content, created_at
		FROM chat_messages WHERE conversation_id = ? ORDER BY created_at ASC`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []models.ChatMessage
	for rows.Next() {
		var m models.ChatMessage
		var createdAt string
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &createdAt); err != nil {
			return nil, err
		}
		if m.CreatedAt, err = parseSQLiteTimestamp("created_at", createdAt); err != nil {
			return nil, fmt.Errorf("chat message %q: %w", m.ID, err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func runMigrations(db *sql.DB) error {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create iofs driver: %w", err)
	}

	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", d, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("failed to create migration instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

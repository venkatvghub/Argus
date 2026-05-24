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

	// Single writer connection prevents intra-process lock contention from concurrent goroutines.
	db.SetMaxOpenConns(1)

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
		INSERT INTO repositories (id, name, local_path, updated_at) 
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET 
			name=excluded.name, 
			local_path=excluded.local_path,
			updated_at=CURRENT_TIMESTAMP`,
		repo.ID, repo.Name, repo.Path)
	return err
}

// ListRepositories returns all indexed repositories.
func (db *DB) ListRepositories(ctx context.Context) ([]models.Repository, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, name, local_path, created_at FROM repositories")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []models.Repository = []models.Repository{}
	for rows.Next() {
		var r models.Repository
		if err := rows.Scan(&r.ID, &r.Name, &r.Path, &r.CreatedAt); err != nil {
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
		`INSERT INTO markers (repo_id, file, type, severity, message, line, deduction, category)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range markers {
		if _, err := stmt.ExecContext(ctx, repoID, m.File, m.Type, m.Severity, m.Message, m.Line, m.Deduction, string(m.Category)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LoadAllMarkers reads all persisted markers grouped by repo ID.
func (db *DB) LoadAllMarkers(ctx context.Context) (map[string][]models.Marker, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT repo_id, file, type, severity, message, line, deduction, category FROM markers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]models.Marker)
	for rows.Next() {
		var repoID string
		var m models.Marker
		var cat string
		if err := rows.Scan(&repoID, &m.File, &m.Type, &m.Severity, &m.Message, &m.Line, &m.Deduction, &cat); err != nil {
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
	j.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	j.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
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
		j.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		j.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpsertWikiPage inserts or replaces a generated wiki page.
func (db *DB) UpsertWikiPage(ctx context.Context, page models.WikiPage) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO wiki_pages (id, repo_id, job_id, type, subject, content, level, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			content = excluded.content,
			created_at = datetime('now')`,
		page.ID, page.RepoID, page.JobID, page.Type, page.Subject, page.Content, page.Level)
	return err
}

// ListWikiPages returns all wiki pages for a repository.
func (db *DB) ListWikiPages(ctx context.Context, repoID string) ([]models.WikiPage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, repo_id, job_id, type, subject, content, level, created_at
		FROM wiki_pages WHERE repo_id = ? ORDER BY level, type, subject`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []models.WikiPage
	for rows.Next() {
		var p models.WikiPage
		var createdAt string
		if err := rows.Scan(&p.ID, &p.RepoID, &p.JobID, &p.Type, &p.Subject, &p.Content, &p.Level, &createdAt); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

// GetWikiPage returns a single wiki page by ID.
func (db *DB) GetWikiPage(ctx context.Context, pageID string) (models.WikiPage, error) {
	var p models.WikiPage
	var createdAt string
	err := db.QueryRowContext(ctx, `
		SELECT id, repo_id, job_id, type, subject, content, level, created_at
		FROM wiki_pages WHERE id = ?`, pageID).
		Scan(&p.ID, &p.RepoID, &p.JobID, &p.Type, &p.Subject, &p.Content, &p.Level, &createdAt)
	if err != nil {
		return p, err
	}
	p.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return p, nil
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

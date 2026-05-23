// Package persistence provides the data access layer for Argus.
package persistence

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

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

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

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

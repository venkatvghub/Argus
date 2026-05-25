// Package argus provides the core orchestration layer for repository ingestion,
// analysis, and API serving. It manages in-memory graph engines, markers, and jobs
// for indexed repositories.
package argus

import (
	"context"
	"database/sql"
	"errors"

	"github.com/venkatvghub/argus/pkg/analysis"
	"github.com/venkatvghub/argus/pkg/models"
)

// GetRepository returns a single repository by ID.
func (i *Instance) GetRepository(ctx context.Context, repoID string) (models.Repository, error) {
	r, err := i.db.GetRepository(ctx, repoID)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrRepoNotFound
	}
	return r, err
}

// DeleteRepository removes a repository from memory and the database.
// The DB delete is performed inside the lock to prevent concurrent Analyze()
// calls from re-populating the in-memory maps between the unlock and the delete.
func (i *Instance) DeleteRepository(ctx context.Context, repoID string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.engines, repoID)
	delete(i.markers, repoID)
	delete(i.changedFiles, repoID)
	delete(i.upToDate, repoID)
	return i.db.DeleteRepository(ctx, repoID)
}

// GetRepoStats returns aggregate statistics for a repository.
func (i *Instance) GetRepoStats(ctx context.Context, repoID string) (map[string]any, error) {
	files, err := i.GetRepoFiles(ctx, repoID)
	if err != nil {
		return nil, err
	}

	langBreakdown := make(map[string]int)
	for _, f := range files {
		if f.Language != "" {
			langBreakdown[f.Language]++
		}
	}

	symbols, err := i.GetRepoSymbols(ctx, repoID)
	if err != nil {
		return nil, err
	}

	score, err := i.GetRepoScore(ctx, repoID)
	if err != nil {
		// Not fatal — repo may have no markers yet.
		score = 0
	}

	repo, err := i.GetRepository(ctx, repoID)
	if err != nil {
		return nil, err
	}
	communityCount, err := i.GetCommunityCount(ctx, repoID)
	if err != nil {
		return nil, err
	}

	i.mu.RLock()
	engine, hasEngine := i.engines[repoID]
	i.mu.RUnlock()

	// Use engine node count for file_count so it matches in-memory state.
	fileCount := len(files)
	if hasEngine {
		cnt := 0
		for _, n := range engine.GetNodes() {
			if n.InternalType() == analysis.NodeTypeFile {
				cnt++
			}
		}
		if cnt > 0 {
			fileCount = cnt
		}
	}

	return map[string]any{
		"file_count":       fileCount,
		"languages":        langBreakdown,
		"total_symbols":    len(symbols),
		"score":            score,
		"last_commit":      repo.LastCommit,
		"community_count":  communityCount,
	}, nil
}

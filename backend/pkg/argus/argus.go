package argus

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/venkatvghub/argus/pkg/analysis"
	"github.com/venkatvghub/argus/pkg/config"
	"github.com/venkatvghub/argus/pkg/constants"
	"github.com/venkatvghub/argus/pkg/ingestion"
	"github.com/venkatvghub/argus/pkg/logger"
	"github.com/venkatvghub/argus/pkg/models"
	"github.com/venkatvghub/argus/pkg/persistence"
)

// Instance manages the lifecycle and resources for a single Argus session.
type Instance struct {
	cfg    *config.Config
	db     *persistence.DB
	log    *logger.Logger
	parser *ingestion.TreeSitterParser
	Jobs   *JobManager

	mu      sync.RWMutex
	engines map[string]*analysis.GraphEngine
	markers map[string][]models.Marker
}

// New creates and initializes a new Argus instance.
func New(ctx context.Context, cfg *config.Config) (*Instance, error) {
	if cfg == nil {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return nil, fmt.Errorf("config load: %w", err)
		}
	}

	if err := logger.Init(logger.Config{
		AppName: cfg.AppName,
		Level:   cfg.LogLevel,
	}); err != nil {
		return nil, fmt.Errorf("logger init: %w", err)
	}
	log := logger.FromContext(ctx)

	db, err := persistence.New(cfg.ResolveDBPath())
	if err != nil {
		return nil, fmt.Errorf("persistence init: %w", err)
	}

	parser, err := ingestion.NewTreeSitterParser()
	if err != nil {
		log.Warn("failed to initialize tree-sitter parser", "error", err)
	}

	return &Instance{
		cfg:     cfg,
		db:      db,
		log:     log,
		parser:  parser,
		Jobs:    NewJobManager(cfg),
		engines: make(map[string]*analysis.GraphEngine),
		markers: make(map[string][]models.Marker),
	}, nil
}

// Config returns the instance's configuration.

func (i *Instance) Config() *config.Config {
	return i.cfg
}

// Close releases all resources.
func (i *Instance) Close() error {
	if i.Jobs != nil {
		i.Jobs.Close()
	}
	if i.db != nil {
		if err := i.db.Close(); err != nil {
			return fmt.Errorf("db close: %w", err)
		}
	}
	i.log.Sync()
	return nil
}

// Analyze traverses the repository at the given path and performs deep analysis.
func (i *Instance) Analyze(ctx context.Context, repoPath string) (string, error) {
	job := i.Jobs.CreateJob(jobTypeAnalysis)

	jobCtx, cancel := context.WithCancel(ctx)
	i.Jobs.RegisterCancel(job.ID, cancel)

	i.Jobs.Submit(job.ID, func() {
		defer cancel()

		i.Jobs.UpdateStatus(job.ID, models.JobStatusInProgress, "Indexing... (0 files)", nil)

		absPath, err := filepath.Abs(repoPath)
		if err != nil {
			i.Jobs.UpdateStatus(job.ID, models.JobStatusFailed, "Failed", err)
			return
		}
		repoName := filepath.Base(absPath)
		repoID := fmt.Sprintf("%x", sha256.Sum256([]byte(absPath)))[:constants.RepoIDLength]

		i.log.Info("starting analysis", "repo_path", absPath, "repo_id", repoID)

		walker := ingestion.NewGitWalker(absPath, i.parser)
		walker.OnProgress = func(n int) {
			i.Jobs.UpdateStatus(job.ID, models.JobStatusInProgress, fmt.Sprintf("Indexing... (%d files)", n), nil)
		}
		nodes, symbols, err := walker.Walk(jobCtx)
		if err != nil {
			i.Jobs.UpdateStatus(job.ID, models.JobStatusFailed, "Failed", err)
			return
		}

		i.Jobs.UpdateStatus(job.ID, models.JobStatusInProgress, "Assembling Graph...", nil)

		i.log.Info("analysis complete", "files", len(nodes), "biomarkers_found", len(symbols))

		engine := analysis.NewGraphEngine()
		if err := engine.BuildGraph(nodes, symbols, nil); err != nil {
			i.Jobs.UpdateStatus(job.ID, models.JobStatusFailed, "Failed", err)
			return
		}
		_ = engine.DetectCommunities()

		markerEngine := analysis.NewMarkerEngine(absPath, i.cfg)
		markers := markerEngine.Run(nodes, symbols, engine)

		// Persist
		repo := models.Repository{
			ID:   repoID,
			Name: repoName,
			Path: absPath,
		}
		if err := i.db.UpsertRepository(jobCtx, repo); err != nil {
			i.log.Warn("failed to persist repo", "error", err)
		}

		i.mu.Lock()
		i.engines[repoID] = engine
		i.markers[repoID] = markers
		i.mu.Unlock()

		i.Jobs.UpdateStatus(job.ID, models.JobStatusCompleted, "Complete", nil)
	})

	return job.ID, nil
}

// ListRepositories returns all indexed repositories.
func (i *Instance) ListRepositories(ctx context.Context) ([]models.Repository, error) {
	return i.db.ListRepositories(ctx)
}

// SearchSymbols searches for symbols across all indexed repositories.
func (i *Instance) SearchSymbols(ctx context.Context, query string, symbolType string) ([]models.Symbol, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	var results []models.Symbol = []models.Symbol{}
	for _, engine := range i.engines {
		for _, node := range engine.GetNodes() {
			if node.InternalType() == analysis.NodeTypeSymbol {
				s := node.Symbol()
				if s != nil && (query == "" || s.Name == query) {
					if symbolType == "" || string(s.Type) == symbolType {
						results = append(results, *s)
					}
				}
			}
		}
	}
	return results, nil
}

// GetFileMarkers returns markers for a specific file.
func (i *Instance) GetFileMarkers(ctx context.Context, repoID string, filePath string) ([]models.Marker, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	markers, ok := i.markers[repoID]
	if !ok {
		return nil, ErrRepoNotFound
	}
	var results []models.Marker = []models.Marker{}
	for _, m := range markers {
		if m.File == filePath {
			results = append(results, m)
		}
	}
	return results, nil
}

// GetCommunityGraph returns nodes for a community.
func (i *Instance) GetCommunityGraph(ctx context.Context, repoID string, communityID int) ([]*analysis.Node, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	engine, ok := i.engines[repoID]
	if !ok {
		return nil, ErrRepoNotFound
	}
	var results []*analysis.Node
	for _, n := range engine.GetNodes() {
		if n.CommunityID == communityID {
			results = append(results, n)
		}
	}
	return results, nil
}

// GetRepoSymbols returns all symbols for a specific repository.
func (i *Instance) GetRepoSymbols(ctx context.Context, repoID string) ([]models.Symbol, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	engine, ok := i.engines[repoID]
	if !ok {
		return nil, ErrRepoNotFound
	}

	var results []models.Symbol = []models.Symbol{}
	for _, node := range engine.GetNodes() {
		if node.InternalType() == analysis.NodeTypeSymbol {
			s := node.Symbol()
			if s != nil {
				results = append(results, *s)
			}
		}
	}
	return results, nil
}

// GetRepoMarkers returns all markers for a specific repository.
func (i *Instance) GetRepoMarkers(ctx context.Context, repoID string) ([]models.Marker, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	markers, ok := i.markers[repoID]
	if !ok {
		return nil, ErrRepoNotFound
	}
	return markers, nil
}

// GetFileScore returns the computed health score for a single file.
func (i *Instance) GetFileScore(ctx context.Context, repoID, filePath string) (models.FileScore, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	markers, ok := i.markers[repoID]
	if !ok {
		return models.FileScore{}, ErrRepoNotFound
	}

	var fileMarkers []models.Marker
	for _, m := range markers {
		if m.File == filePath {
			fileMarkers = append(fileMarkers, m)
		}
	}

	return analysis.ComputeFileScore(filePath, fileMarkers), nil
}

// GetRepoScore returns the aggregate health score for a repository.
func (i *Instance) GetRepoScore(ctx context.Context, repoID string) (float64, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	markers, ok := i.markers[repoID]
	if !ok {
		return 0, ErrRepoNotFound
	}

	// Group markers by file.
	byFile := make(map[string][]models.Marker)
	for _, m := range markers {
		byFile[m.File] = append(byFile[m.File], m)
	}

	// Build PageRank map and collect all file paths from the graph so that
	// files with zero markers (perfect score 10.0) are included in the average.
	var pageRanks map[string]float64
	allFilePaths := make(map[string]struct{})
	if engine, ok := i.engines[repoID]; ok {
		pageRanks = make(map[string]float64)
		for _, n := range engine.GetNodes() {
			if n.InternalType() == analysis.NodeTypeFile {
				if f := n.File(); f != nil {
					pageRanks[f.Path] = n.PageRank
					allFilePaths[f.Path] = struct{}{}
				}
			}
		}
	}

	// Score every file: marker-bearing files get actual deductions; others score 10.0.
	fileScores := make([]models.FileScore, 0, len(allFilePaths))
	for path := range allFilePaths {
		fileScores = append(fileScores, analysis.ComputeFileScore(path, byFile[path]))
	}
	// Fall back to marker-only files if graph has no file nodes (e.g. analysis not run).
	if len(fileScores) == 0 {
		for file, fm := range byFile {
			fileScores = append(fileScores, analysis.ComputeFileScore(file, fm))
		}
	}

	return analysis.ComputeRepoScore(fileScores, pageRanks), nil
}

// Run starts the default pipeline.

func (i *Instance) Run(ctx context.Context) error {
	_, err := i.Analyze(ctx, defaultAnalyzePath)
	return err
}

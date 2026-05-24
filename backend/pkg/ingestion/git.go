package ingestion

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/venkatvghub/argus/pkg/models"
)

// GitWalker provides methods to traverse a Git repository and extract metadata and AST insights.
type GitWalker struct {
	repoPath   string
	parser     *TreeSitterParser
	Workers    int // concurrent file processors; 0 → runtime.NumCPU()
	OnProgress func(filesProcessed int)
}

// NewGitWalker creates a new GitWalker for the repository at the given path.
func NewGitWalker(repoPath string, parser *TreeSitterParser) *GitWalker {
	return &GitWalker{
		repoPath: repoPath,
		parser:   parser,
	}
}

// fileMetrics holds pre-computed git history metrics for a single file.
type fileMetrics struct {
	churn                   int
	ownership               float64
	authorCount             int
	primaryAuthorLastCommit time.Time
}

// Walk traverses the repository's HEAD tree and returns metadata and biomarkers for each file.
// Files matching .gitignore patterns are skipped. AST parsing is parallelized across Workers goroutines.
func (w *GitWalker) Walk(ctx context.Context) ([]models.FileNode, []models.Symbol, error) {
	repo, err := git.PlainOpen(w.repoPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open repo: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get head: %w", err)
	}

	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get head commit: %w", err)
	}

	gitTree, err := headCommit.Tree()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get tree: %w", err)
	}

	ignoreMatcher := loadGitignore(repo)

	// Collect eligible files first (fast — only reads tree metadata, not content).
	var files []*object.File
	if err := gitTree.Files().ForEach(func(f *object.File) error {
		if ignoreMatcher != nil && ignoreMatcher.Match(splitPath(f.Name), false) {
			return nil
		}
		files = append(files, f)
		return nil
	}); err != nil {
		return nil, nil, err
	}

	// Single history walk for all file metrics — O(commits) instead of O(files × commits).
	metricsMap, err := buildFileMetricsMap(repo)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build metrics: %w", err)
	}

	workers := w.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	type fileResult struct {
		idx     int
		node    models.FileNode
		symbols []models.Symbol
		err     error
	}

	results := make([]fileResult, len(files))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var processed atomic.Int64

	for i, f := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, f *object.File) {
			defer wg.Done()
			defer func() { <-sem }()

			m := metricsMap[f.Name]
			node := models.FileNode{
				Path:                    f.Name,
				IsFile:                  true,
				Size:                    f.Size,
				Churn:                   m.churn,
				Ownership:               m.ownership,
				AuthorCount:             m.authorCount,
				PrimaryAuthorLastCommit: m.primaryAuthorLastCommit,
			}

			var symbols []models.Symbol
			if w.parser != nil {
				var parseErr error
				symbols, parseErr = w.analyzeFile(ctx, f)
				if parseErr != nil {
					results[idx] = fileResult{idx: idx, err: fmt.Errorf("analyze %s: %w", f.Name, parseErr)}
					return
				}
			}

			n := processed.Add(1)
			if w.OnProgress != nil {
				w.OnProgress(int(n))
			}

			results[idx] = fileResult{idx: idx, node: node, symbols: symbols}
		}(i, f)
	}
	wg.Wait()

	// Collect results in original order.
	nodes := make([]models.FileNode, 0, len(files))
	var allSymbols []models.Symbol
	for _, r := range results {
		if r.err != nil {
			return nil, nil, r.err
		}
		// Skip zero-value results (gitignore-filtered slot would not appear here, but guard anyway).
		if r.node.Path == "" {
			continue
		}
		nodes = append(nodes, r.node)
		allSymbols = append(allSymbols, r.symbols...)
	}

	return nodes, allSymbols, nil
}

func (w *GitWalker) analyzeFile(ctx context.Context, f *object.File) ([]models.Symbol, error) {
	content, err := f.Contents()
	if err != nil {
		return nil, err
	}

	tree, langName, err := w.parser.Parse(ctx, []byte(content), f.Name)
	if err != nil {
		if errors.Is(err, ErrUnsupportedLanguage) {
			return nil, nil // skip files with no registered grammar
		}
		return nil, err
	}

	symbols, err := w.parser.ExecuteBiomarkers(tree, langName, []byte(content))
	if err != nil {
		return nil, err
	}

	for i := range symbols {
		symbols[i].FilePath = f.Name
	}

	return symbols, nil
}

// buildFileMetricsMap walks the entire commit history once and accumulates per-file stats.
// This is O(commits × avg_files_changed) rather than O(files × commits).
func buildFileMetricsMap(repo *git.Repository) (map[string]fileMetrics, error) {
	cIter, err := repo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})
	if err != nil {
		return nil, err
	}

	type perFileAuthorStats struct {
		totalCommits int
		lastCommit   time.Time
	}

	fileAuthorStats := make(map[string]map[string]*perFileAuthorStats)
	fileRecentAuthors := make(map[string]map[string]bool)
	cutoff := time.Now().AddDate(0, 0, -90)

	_ = cIter.ForEach(func(c *object.Commit) error {
		email := c.Author.Email
		isRecent := c.Author.When.After(cutoff)

		stats, err := c.Stats()
		if err != nil {
			return nil // skip commits where diff fails (e.g. merge conflicts, binary)
		}

		for _, s := range stats {
			path := s.Name
			if _, ok := fileAuthorStats[path]; !ok {
				fileAuthorStats[path] = make(map[string]*perFileAuthorStats)
				fileRecentAuthors[path] = make(map[string]bool)
			}
			as, ok := fileAuthorStats[path][email]
			if !ok {
				as = &perFileAuthorStats{}
				fileAuthorStats[path][email] = as
			}
			as.totalCommits++
			if c.Author.When.After(as.lastCommit) {
				as.lastCommit = c.Author.When
			}
			if isRecent {
				fileRecentAuthors[path][email] = true
			}
		}
		return nil
	})

	result := make(map[string]fileMetrics, len(fileAuthorStats))
	for path, authorMap := range fileAuthorStats {
		totalCommits := 0
		for _, as := range authorMap {
			totalCommits += as.totalCommits
		}
		if totalCommits == 0 {
			continue
		}

		maxCommits := 0
		var primaryEmail string
		for email, as := range authorMap {
			if as.totalCommits > maxCommits {
				maxCommits = as.totalCommits
				primaryEmail = email
			} else if as.totalCommits == maxCommits && (primaryEmail == "" || email < primaryEmail) {
				primaryEmail = email
			}
		}

		var primaryLastCommit time.Time
		if primaryEmail != "" {
			primaryLastCommit = authorMap[primaryEmail].lastCommit
		}

		result[path] = fileMetrics{
			churn:                   totalCommits,
			ownership:               float64(maxCommits) / float64(totalCommits),
			authorCount:             len(fileRecentAuthors[path]),
			primaryAuthorLastCommit: primaryLastCommit,
		}
	}
	return result, nil
}

// loadGitignore reads .gitignore patterns from the repository worktree.
// Returns nil if patterns cannot be loaded (non-fatal).
func loadGitignore(repo *git.Repository) gitignore.Matcher {
	wt, err := repo.Worktree()
	if err != nil {
		return nil
	}
	patterns, err := gitignore.ReadPatterns(wt.Filesystem, nil)
	if err != nil || len(patterns) == 0 {
		return nil
	}
	return gitignore.NewMatcher(patterns)
}

// splitPath converts a slash-separated path string into a component slice for gitignore matching.
func splitPath(p string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			if i > start {
				parts = append(parts, p[start:i])
			}
			start = i + 1
		}
	}
	if start < len(p) {
		parts = append(parts, p[start:])
	}
	return parts
}

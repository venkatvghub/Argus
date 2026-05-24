package ingestion

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/venkatvghub/argus/pkg/models"
)

// commitRecordSep and fieldSep are the parsed byte values from git's %x1e/%x1f format escapes.
// We use git's %x<hex> notation in the format string (not raw bytes) to avoid NUL-terminaton
// issues when passing format strings as exec.Command arguments.
const (
	commitRecordSep = "\x1e" // git %x1e — record separator, safe in CLI args
	fieldSep        = "\x1f" // git %x1f — unit separator
)

// GitWalker provides methods to traverse a Git repository and extract metadata and AST insights.
type GitWalker struct {
	repoPath          string
	parser            *TreeSitterParser
	Workers           int // concurrent file processors; 0 → runtime.NumCPU()
	OnProgress        func(filesProcessed int)
	OnHistoryProgress func(commitsProcessed int)
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

	// Single native git log --numstat call — native C diff, much faster than go-git c.Stats().
	metricsMap, err := buildFileMetricsMap(w.repoPath, w.OnHistoryProgress)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build metrics: %w", err)
	}

	workers := w.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	type fileResult struct {
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
					results[idx] = fileResult{err: fmt.Errorf("analyze %s: %w", f.Name, parseErr)}
					return
				}
			}

			n := processed.Add(1)
			if w.OnProgress != nil {
				w.OnProgress(int(n))
			}

			results[idx] = fileResult{node: node, symbols: symbols}
		}(i, f)
	}
	wg.Wait()

	nodes := make([]models.FileNode, 0, len(files))
	var allSymbols []models.Symbol
	for _, r := range results {
		if r.err != nil {
			return nil, nil, r.err
		}
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

// buildFileMetricsMap uses a single `git log --numstat` call (native C diff engine) to accumulate
// per-file commit stats. This mirrors repowise's approach and is orders of magnitude faster than
// go-git's c.Stats() which computes in-memory diffs for every commit.
func buildFileMetricsMap(repoPath string, onProgress func(int)) (map[string]fileMetrics, error) {
	// %x1e = record separator, %x1f = field separator, %ae = author email, %ct = commit timestamp (unix)
	logFormat := "%x1e%ae%x1f%ct"
	cmd := exec.Command("git", "log",
		"--numstat",
		"--no-merges",
		"--format="+logFormat,
		"HEAD",
	)
	cmd.Dir = repoPath

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log --numstat: %w", err)
	}

	type perFileAuthorStats struct {
		totalCommits int
		lastCommit   time.Time
	}

	fileAuthorStats := make(map[string]map[string]*perFileAuthorStats)
	fileRecentAuthors := make(map[string]map[string]bool)
	cutoff := time.Now().AddDate(0, 0, -90)
	commitCount := 0

	// Output is split by the NUL record separator; first element is always empty.
	for _, record := range strings.Split(string(out), commitRecordSep) {
		if record == "" {
			continue
		}

		// First line: "email\x1ftimestamp", rest: numstat lines
		nl := strings.IndexByte(record, '\n')
		if nl < 0 {
			continue
		}
		header := record[:nl]
		body := record[nl+1:]

		parts := strings.SplitN(header, fieldSep, 2)
		if len(parts) < 2 {
			continue
		}
		email := parts[0]
		ts, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			continue
		}
		commitTime := time.Unix(ts, 0)
		isRecent := commitTime.After(cutoff)

		commitCount++
		if onProgress != nil && commitCount%100 == 0 {
			onProgress(commitCount)
		}

		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// numstat format: "added\tdeleted\tpath"
			cols := strings.SplitN(line, "\t", 3)
			if len(cols) != 3 || cols[0] == "-" { // "-" = binary file
				continue
			}
			path := normalizeRename(cols[2])

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
			if commitTime.After(as.lastCommit) {
				as.lastCommit = commitTime
			}
			if isRecent {
				fileRecentAuthors[path][email] = true
			}
		}
	}

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

// normalizeRename extracts the destination path from git's rename notation in numstat output.
// "{src => dst}/file.go" → "dst/file.go"
// "old/path.go => new/path.go" → "new/path.go"
func normalizeRename(path string) string {
	arrow := strings.Index(path, " => ")
	if arrow < 0 {
		return path
	}
	// Brace notation: prefix{old => new}suffix
	braceStart := strings.LastIndex(path[:arrow], "{")
	if braceStart >= 0 {
		braceEnd := strings.Index(path[arrow:], "}")
		if braceEnd >= 0 {
			prefix := path[:braceStart]
			newPart := path[arrow+4 : arrow+braceEnd]
			suffix := path[arrow+braceEnd+1:]
			return prefix + newPart + suffix
		}
	}
	// Simple: "old => new"
	return path[arrow+4:]
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

package ingestion

import (
	"bufio"
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
	repoPath               string
	parser                 *TreeSitterParser
	RecentAuthorCutoffDays int // lookback for recent author count; 0 → 90 days
	Workers                int // concurrent file processors; 0 → runtime.NumCPU()
	OnTotalFiles      func(total int)
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
	primaryAuthorEmail      string
	primaryAuthorName       string
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

	if w.OnTotalFiles != nil {
		w.OnTotalFiles(len(files))
	}

	// Single native git log --numstat call — native C diff, much faster than go-git c.Stats().
	metricsMap, err := buildFileMetricsMap(w.repoPath, w.recentAuthorCutoffDays(), w.OnHistoryProgress)
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
				PrimaryAuthor:           m.primaryAuthorName,
				PrimaryAuthorLastCommit: m.primaryAuthorLastCommit,
			}

			var symbols []models.Symbol
			if w.parser != nil {
				var parseErr error
				var langName string
				symbols, langName, parseErr = w.analyzeFile(ctx, f)
				if parseErr != nil {
					results[idx] = fileResult{err: fmt.Errorf("analyze %s: %w", f.Name, parseErr)}
					return
				}
				if langName != "" {
					node.Language = langName
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

func (w *GitWalker) analyzeFile(ctx context.Context, f *object.File) ([]models.Symbol, string, error) {
	content, err := f.Contents()
	if err != nil {
		return nil, "", err
	}

	tree, langName, err := w.parser.Parse(ctx, []byte(content), f.Name)
	if err != nil {
		if errors.Is(err, ErrUnsupportedLanguage) {
			return nil, "", nil // skip files with no registered grammar
		}
		return nil, "", err
	}

	symbols, err := w.parser.ExecuteBiomarkers(tree, langName, []byte(content))
	if err != nil {
		return nil, langName, err
	}

	for i := range symbols {
		symbols[i].FilePath = f.Name
	}

	return symbols, langName, nil
}

func (w *GitWalker) recentAuthorCutoffDays() int {
	if w.RecentAuthorCutoffDays > 0 {
		return w.RecentAuthorCutoffDays
	}
	return 90
}

// buildFileMetricsMap streams `git log --name-only` (no diff computation, ~25× faster than
// --numstat) to accumulate per-file churn/ownership stats in a single pass.
func buildFileMetricsMap(repoPath string, recentAuthorCutoffDays int, onProgress func(int)) (map[string]fileMetrics, error) {
	// --name-only: file names only, no diff content computed → fast
	// %x1e = record separator (safe in CLI args), %x1f = field separator
	// format: \x1e<email>\x1f<name>\x1f<unix-timestamp>
	cmd := exec.Command("git", "log",
		"--name-only",
		"--no-merges",
		"--format=%x1e%ae%x1f%an%x1f%ct",
		"HEAD",
	)
	cmd.Dir = repoPath

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("git log pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("git log start: %w", err)
	}

	type perFileAuthorStats struct {
		totalCommits int
		lastCommit   time.Time
		name         string
	}

	fileAuthorStats := make(map[string]map[string]*perFileAuthorStats)
	fileRecentAuthors := make(map[string]map[string]bool)
	cutoff := time.Now().AddDate(0, 0, -recentAuthorCutoffDays)

	var email, authorName string
	var commitTime time.Time
	var isRecent bool
	commitCount := 0

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 512*1024), 512*1024)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, commitRecordSep) {
			// Commit header: \x1eemail\x1fname\x1ftimestamp
			parts := strings.SplitN(line[len(commitRecordSep):], fieldSep, 3)
			if len(parts) < 3 {
				email = ""
				authorName = ""
				continue
			}
			email = parts[0]
			authorName = parts[1]
			ts, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
			if err != nil {
				email = ""
				authorName = ""
				continue
			}
			commitTime = time.Unix(ts, 0)
			isRecent = commitTime.After(cutoff)
			commitCount++
			if onProgress != nil && commitCount%100 == 0 {
				onProgress(commitCount)
			}
			continue
		}

		if line == "" || email == "" {
			continue
		}

		path := normalizeRename(line)
		if _, ok := fileAuthorStats[path]; !ok {
			fileAuthorStats[path] = make(map[string]*perFileAuthorStats)
			fileRecentAuthors[path] = make(map[string]bool)
		}
		as, ok := fileAuthorStats[path][email]
		if !ok {
			as = &perFileAuthorStats{name: authorName}
			fileAuthorStats[path][email] = as
		}
		if as.name == "" && authorName != "" {
			as.name = authorName
		}
		as.totalCommits++
		if commitTime.After(as.lastCommit) {
			as.lastCommit = commitTime
		}
		if isRecent {
			fileRecentAuthors[path][email] = true
		}
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("git log: %w", err)
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
		var primaryName string
		if primaryEmail != "" {
			primaryLastCommit = authorMap[primaryEmail].lastCommit
			primaryName = authorMap[primaryEmail].name
		}

		result[path] = fileMetrics{
			churn:                   totalCommits,
			ownership:               float64(maxCommits) / float64(totalCommits),
			authorCount:             len(fileRecentAuthors[path]),
			primaryAuthorEmail:      primaryEmail,
			primaryAuthorName:       primaryName,
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

// GitHEAD returns the full SHA of the HEAD commit for the git repo at repoPath.
func GitHEAD(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// GitChangedFiles returns paths changed between fromCommit and toCommit (exclusive..inclusive).
// Both commits must be full or abbreviated SHAs. Returns an empty slice if commits are equal.
func GitChangedFiles(repoPath, fromCommit, toCommit string) ([]string, error) {
	if fromCommit == toCommit {
		return nil, nil
	}
	out, err := exec.Command("git", "-C", repoPath, "diff", "--name-only", fromCommit+".."+toCommit).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only: %w", err)
	}
	var files []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

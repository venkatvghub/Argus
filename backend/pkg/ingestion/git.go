package ingestion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/venkatvghub/argus/pkg/models"
)

// GitWalker provides methods to traverse a Git repository and extract metadata and AST insights.
type GitWalker struct {
	repoPath string
	parser   *TreeSitterParser
}

// NewGitWalker creates a new GitWalker for the repository at the given path.
func NewGitWalker(repoPath string, parser *TreeSitterParser) *GitWalker {
	return &GitWalker{
		repoPath: repoPath,
		parser:   parser,
	}
}

// Walk traverses the repository's HEAD tree and returns metadata and biomarkers for each file.
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

	tree, err := headCommit.Tree()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get tree: %w", err)
	}

	var nodes []models.FileNode
	var allSymbols []models.Symbol

	err = tree.Files().ForEach(func(f *object.File) error {
		node := models.FileNode{
			Path:   f.Name,
			IsFile: true,
			Size:   f.Size,
		}

		// Calculate churn, ownership, and git org metrics
		churn, ownership, authorCount, primaryLastCommit, err := calculateMetrics(repo, f.Name)
		if err != nil {
			return fmt.Errorf("metrics for %s: %w", f.Name, err)
		}
		node.Churn = churn
		node.Ownership = ownership
		node.AuthorCount = authorCount
		node.PrimaryAuthorLastCommit = primaryLastCommit
		nodes = append(nodes, node)

		// AST Analysis if parser is available
		if w.parser != nil {
			symbols, err := w.analyzeFile(ctx, f)
			if err != nil {
				return fmt.Errorf("analyze %s: %w", f.Name, err)
			}
			allSymbols = append(allSymbols, symbols...)
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
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

	// Enrich symbols with file path
	for i := range symbols {
		symbols[i].FilePath = f.Name
	}

	return symbols, nil
}

// calculateMetrics computes churn, top-author ownership, distinct author count (last 90 days),
// and the last commit time of the primary author (highest total commit count).
func calculateMetrics(repo *git.Repository, filePath string) (churn int, ownership float64, authorCount int, primaryAuthorLastCommit time.Time, err error) {
	cIter, err := repo.Log(&git.LogOptions{
		FileName: &filePath,
	})
	if err != nil {
		return 0, 0, 0, time.Time{}, err
	}

	type authorStats struct {
		totalCommits int
		lastCommit   time.Time
	}

	allAuthorStats := make(map[string]*authorStats)
	recentAuthors := make(map[string]bool)
	cutoff := time.Now().AddDate(0, 0, -90)
	totalCommits := 0

	err = cIter.ForEach(func(c *object.Commit) error {
		email := c.Author.Email
		totalCommits++

		if _, ok := allAuthorStats[email]; !ok {
			allAuthorStats[email] = &authorStats{}
		}
		st := allAuthorStats[email]
		st.totalCommits++
		if c.Author.When.After(st.lastCommit) {
			st.lastCommit = c.Author.When
		}

		if c.Author.When.After(cutoff) {
			recentAuthors[email] = true
		}
		return nil
	})
	if err != nil {
		return 0, 0, 0, time.Time{}, err
	}

	if totalCommits == 0 {
		return 0, 0, 0, time.Time{}, nil
	}

	// Find primary author (highest total commit count)
	maxCommits := 0
	var primaryEmail string
	for email, st := range allAuthorStats {
		if st.totalCommits > maxCommits {
			maxCommits = st.totalCommits
			primaryEmail = email
		} else if st.totalCommits == maxCommits && (primaryEmail == "" || email < primaryEmail) {
			primaryEmail = email
		}
	}

	ownership = float64(maxCommits) / float64(totalCommits)
	authorCount = len(recentAuthors)

	if primaryEmail != "" {
		primaryAuthorLastCommit = allAuthorStats[primaryEmail].lastCommit
	}

	return totalCommits, ownership, authorCount, primaryAuthorLastCommit, nil
}

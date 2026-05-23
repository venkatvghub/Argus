package ingestion

import (
	"context"
	"fmt"

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

		// Calculate churn and ownership
		churn, ownership, err := calculateMetrics(repo, f.Name)
		if err != nil {
			return fmt.Errorf("metrics for %s: %w", f.Name, err)
		}
		node.Churn = churn
		node.Ownership = ownership
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

// calculateMetrics computes the churn count and top-author ownership for a file.
func calculateMetrics(repo *git.Repository, filePath string) (int, float64, error) {
	cIter, err := repo.Log(&git.LogOptions{
		FileName: &filePath,
	})
	if err != nil {
		return 0, 0, err
	}

	authorCommits := make(map[string]int)
	totalCommits := 0

	err = cIter.ForEach(func(c *object.Commit) error {
		authorCommits[c.Author.Email]++
		totalCommits++
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	if totalCommits == 0 {
		return 0, 0, nil
	}

	maxCommits := 0
	for _, count := range authorCommits {
		if count > maxCommits {
			maxCommits = count
		}
	}

	ownership := float64(maxCommits) / float64(totalCommits)
	return totalCommits, ownership, nil
}

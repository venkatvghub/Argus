package ingestion

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "argus-test-repo-*")
	require.NoError(t, err)

	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	w, err := repo.Worktree()
	require.NoError(t, err)

	// Create and commit a file
	filename := "file1.go"
	fullPath := filepath.Join(dir, filename)
	err = os.WriteFile(fullPath, []byte("package main\nfunc main() { go func() {}() }\n"), 0644)
	require.NoError(t, err)

	_, err = w.Add(filename)
	require.NoError(t, err)

	author1 := &object.Signature{
		Name:  "Author One",
		Email: "one@example.com",
		When:  time.Now(),
	}

	_, err = w.Commit("Initial commit", &git.CommitOptions{
		Author: author1,
	})
	require.NoError(t, err)

	return dir, func() { os.RemoveAll(dir) }
}

func TestGitWalker_Walk(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	parser, err := NewTreeSitterParser()
	require.NoError(t, err)

	walker := NewGitWalker(dir, parser)
	ctx := context.Background()
	nodes, symbols, err := walker.Walk(ctx)
	require.NoError(t, err)

	assert.Len(t, nodes, 1)
	node := nodes[0]
	assert.Equal(t, "file1.go", node.Path)
	assert.True(t, node.IsFile)
	assert.Equal(t, 1, node.Churn)
	assert.Equal(t, 1.0, node.Ownership)

	// Verify that the parser found the unsecured_goroutine biomarker
	assert.NotEmpty(t, symbols)
	assert.Equal(t, "unsecured_goroutine", symbols[0].Name)
	assert.Equal(t, "file1.go", symbols[0].FilePath)
}

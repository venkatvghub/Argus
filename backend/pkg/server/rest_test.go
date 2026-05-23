package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/argus"
	"github.com/venkatvghub/argus/pkg/config"
)

func setupTestArgus(t *testing.T) (*argus.Instance, func()) {
	tmpDir, err := os.MkdirTemp("", "argus-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		DBPath:   dbPath,
		LogLevel: "error",
		AppName:  "ArgusTest",
	}

	ctx := context.Background()
	argus, err := argus.New(ctx, cfg)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create argus instance: %v", err)
	}

	return argus, func() {
		argus.Close()
		os.RemoveAll(tmpDir)
	}
}

func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "argus-test-repo-*")
	require.NoError(t, err)

	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	w, err := repo.Worktree()
	require.NoError(t, err)

	// Create and commit a file
	filename := "main.go"
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

func TestRESTServer_ListRepos(t *testing.T) {
	argus, cleanup := setupTestArgus(t)
	defer cleanup()

	srv := NewRESTServer(argus)
	router := srv.Routes()

	req := httptest.NewRequest("GET", "/api/repos", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "[]")
}

func waitForRepoAnalyzed(t *testing.T, argus *argus.Instance, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		repos, err := argus.ListRepositories(context.Background())
		if err == nil && len(repos) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for analysis to complete")
}

func TestRESTServer_IndexRepo(t *testing.T) {
	argus, cleanup := setupTestArgus(t)
	defer cleanup()

	srv := NewRESTServer(argus)
	router := srv.Routes()

	tmpRepo, repoCleanup := setupTestRepo(t)
	defer repoCleanup()

	body, _ := json.Marshal(map[string]string{"path": tmpRepo})
	req := httptest.NewRequest("POST", "/api/repos/index", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusAccepted, rr.Code)

	// Poll until analysis completes before asserting repo presence
	waitForRepoAnalyzed(t, argus, 5*time.Second)

	req = httptest.NewRequest("GET", "/api/repos", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), filepath.Base(tmpRepo))
}

func TestRESTServer_GetRepoMarkers(t *testing.T) {
	argus, cleanup := setupTestArgus(t)
	defer cleanup()

	srv := NewRESTServer(argus)
	router := srv.Routes()

	tmpRepo, repoCleanup := setupTestRepo(t)
	defer repoCleanup()

	_, err := argus.Analyze(context.Background(), tmpRepo)
	assert.NoError(t, err)

	waitForRepoAnalyzed(t, argus, 5*time.Second)

	repos, _ := argus.ListRepositories(context.Background())
	assert.NotEmpty(t, repos)
	repoID := repos[0].ID

	req := httptest.NewRequest("GET", "/api/repos/"+repoID+"/markers", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "[")
}

func TestRESTServer_GetRepoSymbols(t *testing.T) {
	argus, cleanup := setupTestArgus(t)
	defer cleanup()

	srv := NewRESTServer(argus)
	router := srv.Routes()

	tmpRepo, repoCleanup := setupTestRepo(t)
	defer repoCleanup()

	_, err := argus.Analyze(context.Background(), tmpRepo)
	assert.NoError(t, err)

	waitForRepoAnalyzed(t, argus, 5*time.Second)

	repos, _ := argus.ListRepositories(context.Background())
	assert.NotEmpty(t, repos)
	repoID := repos[0].ID

	req := httptest.NewRequest("GET", "/api/repos/"+repoID+"/symbols", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestRESTServer_ExportCognee(t *testing.T) {
	argus, cleanup := setupTestArgus(t)
	defer cleanup()

	srv := NewRESTServer(argus)
	router := srv.Routes()

	req := httptest.NewRequest("GET", "/api/export/cognee", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "entities")
}

func TestRESTServer_InvalidBody(t *testing.T) {
	argus, cleanup := setupTestArgus(t)
	defer cleanup()

	srv := NewRESTServer(argus)
	router := srv.Routes()

	req := httptest.NewRequest("POST", "/api/repos/index", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

package persistence

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/models"
)

func TestDB(t *testing.T) {
	dataDir := t.TempDir()
	// New() now takes a full DB path, not just a directory.
	dbPath := dataDir + "/argus.db"
	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Verify table exists
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='repositories'").Scan(&name)
	assert.NoError(t, err)
	assert.Equal(t, "repositories", name)
}

func TestUpsertMarkers_PersistsSuggestion(t *testing.T) {
	dataDir := t.TempDir()
	db, err := New(dataDir + "/argus.db")
	require.NoError(t, err)
	defer db.Close()

	ctx := t.Context()
	repoID := "test-repo"
	markers := []models.Marker{
		{
			Type:       "token_bloat",
			Severity:   "warning",
			Message:    "high token density",
			File:       "main.go",
			Line:       10,
			Deduction:  0.5,
			Category:   models.ScoreCatEfficiency,
			Suggestion: "Split large functions into smaller helpers.",
		},
	}

	require.NoError(t, db.UpsertMarkers(ctx, repoID, markers))

	loaded, err := db.LoadAllMarkers(ctx)
	require.NoError(t, err)
	require.Len(t, loaded[repoID], 1)
	assert.Equal(t, "Split large functions into smaller helpers.", loaded[repoID][0].Suggestion)
}

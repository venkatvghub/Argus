package persistence

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/models"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ARGUS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ARGUS_TEST_DATABASE_URL not set — skipping persistence integration tests")
	}
	return dsn
}

func TestDB(t *testing.T) {
	db, err := New(testDSN(t))
	require.NoError(t, err)
	defer db.Close()

	// Verify connection is alive
	sqlDB, err := db.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping())
}

func TestUpsertMarkers_PersistsSuggestion(t *testing.T) {
	db, err := New(testDSN(t))
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	repoID := "test-repo-markers"
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
